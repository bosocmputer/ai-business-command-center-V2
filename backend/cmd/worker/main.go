package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/alert"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/auth"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/config"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/database"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/delivery"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/master"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/notification"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/quota"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/recipient"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/retention"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/schedule"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/secret"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/sml"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		logger.Error("invalid worker configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := database.OpenPool(ctx, cfg.DatabaseURL, cfg.DatabaseMaxConnections, cfg.DatabaseMinConnections)
	if err != nil {
		logger.Error("create database pool", "error", "database configuration rejected")
		os.Exit(1)
	}
	defer pool.Close()
	box, err := secret.NewBox(cfg.EncryptionMasterKey, cfg.EncryptionKeyID, rand.Reader)
	if err != nil {
		logger.Error("create secret box", "error", "encryption configuration rejected")
		os.Exit(1)
	}
	policy := sml.EndpointPolicy{AllowedPrefixes: cfg.SMLAllowedPrefixes, AllowedHosts: cfg.SMLAllowedHosts, AllowPublicEndpoints: cfg.SMLAllowPublicEndpoints, AllowedPorts: cfg.SMLAllowedPorts}
	connections := sml.NewConnectionService(database.NewSMLConnectionStore(pool), box, policy, nil, time.Now)
	// The HTTP transport is a hard ceiling. Report-level contexts impose the
	// lower 60s/120s/5m total budgets and are shared by current+comparison.
	reportClient := sml.NewClient(policy, 5*time.Minute, 32*1024*1024, 200_000)
	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("%s-%d", hostname, os.Getpid())
	reportStore := database.NewReportStore(pool).ConfigureGenerationCache(cfg.GenerationCacheEnabled)
	modeStore := database.NewReportModeStore(pool)
	// The legacy env allowlist only seeds the table so nothing changes on rollout.
	if err := modeStore.SeedFromTargets(ctx, cfg.HeavyChunkTenantReports, time.Now().UTC()); err != nil {
		logger.Error("seed report execution modes", "error", "database write failed")
		os.Exit(1)
	}
	reportWorker := worker.NewReportWorker(reportStore, connections, reportClient, workerID, time.Now).
		ConfigureSummaryQueries(cfg.SummaryQueryEnabled).
		ConfigureHeavyChunks(cfg.HeavyChunkEnabled, cfg.ScheduleChunkEnabled, cfg.HeavyChunkTenantReports).
		ConfigureExecutionModes(modeStore)
	schedulerID := workerID + "-scheduler"
	periodObserver := func(preset report.Preset, mode report.ParameterKind, result string) {
		logger.Info("schedule period resolved", "event", "schedule_period_resolution", "preset", preset, "mode", mode, "result", result, "schedulePeriodResolutionTotal", 1)
	}
	dueWorker := schedule.NewDueWorker(database.NewScheduleStore(pool).ConfigureSmartPeriods(cfg.SmartSchedulePeriodsEnabled, cfg.SmartSchedulePeriodTenantIDs, periodObserver), schedulerID, time.Now)
	sessionManager, err := auth.NewSessionManager(cfg.SessionHMACKey, rand.Reader, time.Now)
	if err != nil {
		logger.Error("create worker token manager", "error", "session configuration rejected")
		os.Exit(1)
	}
	notificationID := workerID + "-notification"
	observedFlexRenderer := func(input line.FlexInput) (json.RawMessage, error) {
		result, renderErr := line.RenderFlexWithStats(input)
		if renderErr != nil {
			logger.Warn("flex render failed",
				"event", "flex_rendered", "presentationVersion", line.FlexPresentationVersion, "result", "ERROR",
				"safeErrorCode", "FLEX_RENDER_FAILED", "reportCount", len(input.Reports),
				"flexRenderTotal", 1, "flexRenderDurationMs", float64(result.Duration.Microseconds())/1000,
			)
			return nil, renderErr
		}
		logger.Info("flex render completed",
			"event", "flex_rendered", "presentationVersion", result.PresentationVersion, "result", "SUCCESS",
			"reportCount", result.ReportCount, "flexRenderTotal", 1, "flexPayloadBytes", result.PayloadBytes,
			"flexZeroReportCount", result.ZeroReportCount, "flexRenderDurationMs", float64(result.Duration.Microseconds())/1000,
			"mixedPeriods", result.MixedPeriods,
			"flexMixedPeriodTotal", 1,
		)
		return result.Message, nil
	}
	notificationWorker := notification.NewWorker(
		database.NewNotificationStore(pool), observedFlexRenderer, sessionManager, rand.Reader,
		cfg.PublicBaseURL, notificationID, time.Now,
	)
	recipientService := recipient.NewService(database.NewRecipientStore(pool), box, sessionManager, rand.Reader, cfg.PublicBaseURL.String(), time.Now)
	deliveryID := workerID + "-delivery"
	deliveryWorker := delivery.NewWorker(
		database.NewDeliveryStore(pool), recipientService,
		line.NewMessagingClient(cfg.LineMessagingAccessToken, line.DefaultPushEndpoint, 30*time.Second),
		deliveryID, time.Now,
	)
	retentionID := workerID + "-retention"
	retentionWorker := retention.NewWorker(database.NewRetentionStore(pool), retention.ProductionPolicy(), time.Now)
	quotaWorker := quota.NewWorker(
		line.NewQuotaClient(
			cfg.LineMessagingAccessToken, line.DefaultQuotaEndpoint, line.DefaultQuotaConsumptionEndpoint, 10*time.Second,
		),
		database.NewQuotaStore(pool), time.Now,
	)

	logger.Info("report worker started", "workerId", workerID, "concurrency", cfg.ReportWorkerConcurrency, "summaryQueriesEnabled", cfg.SummaryQueryEnabled)
	var recoveryLoopAt atomic.Int64
	go reportRecoveryLoop(ctx, logger, reportStore, &recoveryLoopAt)
	go heartbeatLoopDynamic(ctx, logger, pool, workerID, "REPORT", hostname, func() map[string]any {
		metadata := map[string]any{"concurrency": cfg.ReportWorkerConcurrency, "summaryQueriesEnabled": cfg.SummaryQueryEnabled}
		if unix := recoveryLoopAt.Load(); unix > 0 {
			metadata["recoveryLoopAt"] = time.Unix(unix, 0).UTC().Format(time.RFC3339)
		}
		return metadata
	})
	go heartbeatLoop(ctx, logger, pool, schedulerID, "SCHEDULER", hostname, map[string]any{"concurrency": 1})
	go heartbeatLoop(ctx, logger, pool, notificationID, "DELIVERY", hostname, map[string]any{"stage": "prepare", "presentationVersion": line.FlexPresentationVersion})
	go heartbeatLoop(ctx, logger, pool, deliveryID, "DELIVERY", hostname, map[string]any{"stage": "send", "concurrency": cfg.DeliveryWorkerConcurrency})
	go heartbeatLoop(ctx, logger, pool, retentionID, "RETENTION", hostname, map[string]any{"snapshotDays": 90, "historyDays": 365})
	go dueScheduleLoop(ctx, logger, dueWorker)
	go notificationLoop(ctx, logger, notificationWorker)
	for lane := 0; lane < cfg.DeliveryWorkerConcurrency; lane++ {
		go deliveryLoop(ctx, logger, deliveryWorker, lane)
	}
	go retentionLoop(ctx, logger, retentionWorker)
	if cfg.MasterSyncEnabled {
		syncer := master.NewSyncer(database.NewMasterStore(pool), master.SMLSource{Connections: connections, Client: sml.NewClient(policy, 3*time.Minute, 32*1024*1024, 100_000)}, logger, time.Now)
		syncer.StartMinute, syncer.Window = cfg.MasterSyncStartMinute, time.Duration(cfg.MasterSyncWindowMinutes)*time.Minute
		go masterLoop(ctx, logger, syncer)
		logger.Info("master data copy started")
	}
	if cfg.AgentAlertsEnabled {
		agentStore := database.NewAgentStore(pool)
		// The daily check reads through the assistant's own service, so a rule sees what the owner would see.
		source := agent.NewService(agentStore, reportStore, sessionManager, rand.Reader, agent.Alias(sessionManager), time.Now, agent.Config{CallsPerHour: cfg.AgentCallsPerHour, RefreshesPerHour: cfg.AgentRefreshesPerHour})
		var sender alert.Sender
		if !cfg.AgentAlertDryRun {
			sender = &alert.WebhookSender{BaseURL: cfg.AgentAlertWebhookURL, Secret: cfg.AgentAlertWebhookSecret, Routes: cfg.AgentAlertWebhookRoutes}
		}
		evaluator := alert.NewEvaluator(agentStore, source, sender, cfg.AgentAlertDryRun, time.Now, logger)
		evaluator.StartMinute, evaluator.Window = cfg.AgentAlertStartMinute, time.Duration(cfg.AgentAlertWindowMinutes)*time.Minute
		go alertLoop(ctx, logger, evaluator)
		logger.Info("agent alerts started", "dryRun", cfg.AgentAlertDryRun, "routes", len(cfg.AgentAlertWebhookRoutes))
	}
	if cfg.LineMessagingAccessToken != "" {
		quotaID := workerID + "-quota"
		go heartbeatLoop(ctx, logger, pool, quotaID, "DELIVERY", hostname, map[string]any{"stage": "quota-sync"})
		go lineQuotaLoop(ctx, logger, quotaWorker)
	}
	for lane := 0; lane < cfg.ReportWorkerConcurrency; lane++ {
		go processLoop(ctx, logger, reportWorker, lane)
	}
	<-ctx.Done()
	logger.Info("report worker stopping", "workerId", workerID)
}

// masterLoop looks for shops whose master data is due every five minutes. A copy is made once a day, so most passes do nothing.
func masterLoop(ctx context.Context, logger *slog.Logger, syncer *master.Syncer) {
	timer := time.NewTimer(45 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("master copy panicked", "safeErrorCode", "MASTER_COPY_PANIC")
				}
			}()
			syncer.RunOnce(ctx)
		}()
		timer.Reset(5 * time.Minute)
	}
}

// alertLoop looks for alerts to check and to deliver every five minutes. A rule is checked once a day, so most passes do nothing.
func alertLoop(ctx context.Context, logger *slog.Logger, evaluator *alert.Evaluator) {
	if !sleepContext(ctx, 30*time.Second) {
		return
	}
	for {
		evaluator.RunOnce(ctx)
		if !sleepContext(ctx, 5*time.Minute) {
			return
		}
	}
}

func lineQuotaLoop(ctx context.Context, logger *slog.Logger, quotaWorker *quota.Worker) {
	delay := time.Duration(0)
	for ctx.Err() == nil {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		status, err := quotaWorker.Process(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				logger.Warn("LINE quota sync failed", "error", err)
			}
			delay = time.Minute
			continue
		}
		logger.Info("LINE quota synced", "state", status.State, "providerLimit", status.ProviderLimit, "providerConsumed", status.ProviderConsumed)
		delay = 5 * time.Minute
	}
}

func retentionLoop(ctx context.Context, logger *slog.Logger, retentionWorker *retention.Worker) {
	delay := time.Duration(0)
	for ctx.Err() == nil {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		counts, err := retentionWorker.Process(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				logger.Error("retention worker error", "error", err)
			}
			delay = time.Minute
			continue
		}
		logger.Info("retention batch completed", "reportRows", counts.ReportRows, "reportRuns", counts.ReportRuns, "dashboardRefreshes", counts.DashboardRefreshes, "dashboardGenerations", counts.DashboardGenerations, "auditLogs", counts.AuditLogs, "viewEvents", counts.ViewEvents, "agentCalls", counts.AgentCalls, "agentAlertEvents", counts.AgentAlertEvents, "evidenceCases", counts.EvidenceCases, "deliveries", counts.Deliveries, "operationalIncidents", counts.OperationalIncidents, "maintenanceWindows", counts.MaintenanceWindows)
		delay = time.Hour
	}
}

func deliveryLoop(ctx context.Context, logger *slog.Logger, deliveryWorker *delivery.Worker, lane int) {
	runLane(ctx, logger, "delivery worker error",
		func(err error) bool { return errors.Is(err, delivery.ErrNoDeliveryReady) },
		deliveryWorker.ProcessOne, newErrorBackoff(errorBackoffInitial, errorBackoffMax), idleDelay, "lane", lane)
}

func notificationLoop(ctx context.Context, logger *slog.Logger, notificationWorker *notification.Worker) {
	runLane(ctx, logger, "notification worker error",
		func(err error) bool { return errors.Is(err, notification.ErrNoExecutionReady) },
		notificationWorker.ProcessOne, newErrorBackoff(errorBackoffInitial, errorBackoffMax), idleDelay)
}

func dueScheduleLoop(ctx context.Context, logger *slog.Logger, dueWorker *schedule.DueWorker) {
	step := func(ctx context.Context) error {
		execution, err := dueWorker.ProcessOne(ctx)
		if err == nil && execution.Status == schedule.ExecutionFailed {
			logger.Warn("due schedule paused by readiness gate", "scheduleId", execution.ScheduleID, "safeErrorCode", execution.SafeErrorCode)
		}
		return err
	}
	runLane(ctx, logger, "schedule worker error",
		func(err error) bool { return errors.Is(err, schedule.ErrNoDueSchedule) },
		step, newErrorBackoff(errorBackoffInitial, errorBackoffMax), idleDelay)
}

func processLoop(ctx context.Context, logger *slog.Logger, reportWorker *worker.ReportWorker, lane int) {
	runLane(ctx, logger, "report worker lane error",
		func(err error) bool { return errors.Is(err, report.ErrNoQueuedRun) },
		reportWorker.ProcessOne, newErrorBackoff(errorBackoffInitial, errorBackoffMax), idleDelay, "lane", lane)
}

func heartbeatLoop(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, workerID, workerType, hostname string, metadata map[string]any) {
	heartbeatLoopDynamic(ctx, logger, pool, workerID, workerType, hostname, func() map[string]any { return metadata })
}

func heartbeatLoopDynamic(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, workerID, workerType, hostname string, metadata func() map[string]any) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		if err := database.RecordWorkerHeartbeat(ctx, pool, workerID, workerType, hostname, metadata(), time.Now().UTC()); err != nil && ctx.Err() == nil {
			logger.Error("worker heartbeat failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func reportRecoveryLoop(ctx context.Context, logger *slog.Logger, store *database.ReportStore, lastSuccess *atomic.Int64) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		now := time.Now().UTC()
		recovered, err := store.RecoverExpiredLeases(ctx, now)
		if err != nil {
			if ctx.Err() == nil {
				logger.Error("report lease recovery failed", "safeErrorCode", "REPORT_LEASE_RECOVERY_FAILED")
			}
		} else {
			lastSuccess.Store(now.Unix())
			if recovered.RequeuedClaimed > 0 || recovered.FailedRunning > 0 {
				logger.Warn("report leases recovered",
					"requeuedClaimed", recovered.RequeuedClaimed,
					"failedRunning", recovered.FailedRunning,
					"cancelledSiblings", recovered.CancelledSiblings,
				)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
