// Command onboarding-check tells whether a shop is ready: its configuration, that every approved report query runs on
// its SML, which document types it uses against the ones the reports count, and the gaps in its data that change what
// the numbers can say. It only reads, and it prints counts and aggregates, never names or per-customer figures.
//
//	ONBOARDING_TENANT_ID=<uuid> /app/onboarding-check            (Thai checklist)
//	ONBOARDING_TENANT_ID=<uuid> ONBOARDING_FORMAT=json /app/onboarding-check
//
// The exit code is 1 when something failed, so a script can stop.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/config"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/database"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/onboarding"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/secret"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/sml"
	"github.com/google/uuid"
)

const queryTimeout = 90 * time.Second

type smlRunner struct {
	client     *sml.Client
	connection sml.Connection
}

func (runner smlRunner) Query(ctx context.Context, statement string) ([]map[string]string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	rows, err := runner.client.Query(queryCtx, runner.connection, statement)
	if err != nil {
		var safe *sml.SafeError
		if errors.As(err, &safe) {
			return nil, errors.New(safe.Code)
		}
		return nil, errors.New("SML_QUERY_FAILED")
	}
	return rows, nil
}

func main() {
	if code, err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "onboarding-check:", err)
		os.Exit(2)
	} else if code != 0 {
		os.Exit(code)
	}
}

func run() (int, error) {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return 0, errors.New("invalid configuration")
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(os.Getenv("ONBOARDING_TENANT_ID")))
	if err != nil {
		return 0, errors.New("ONBOARDING_TENANT_ID must be a UUID")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := database.OpenPool(ctx, cfg.DatabaseURL, cfg.DatabaseMaxConnections, cfg.DatabaseMinConnections)
	if err != nil {
		return 0, errors.New("database is unavailable")
	}
	defer pool.Close()
	store := database.NewOnboardingStore(pool)
	checker := onboarding.NewChecker(store, nil, time.Now)

	facts, factsErr := store.Facts(ctx, tenantID, time.Now())
	if factsErr == nil && facts.Found && facts.SMLReadiness == "READY" {
		box, boxErr := secret.NewBox(cfg.EncryptionMasterKey, cfg.EncryptionKeyID, rand.Reader)
		if boxErr != nil {
			return 0, errors.New("encryption configuration is invalid")
		}
		policy := sml.EndpointPolicy{AllowedPrefixes: cfg.SMLAllowedPrefixes, AllowedHosts: cfg.SMLAllowedHosts, AllowPublicEndpoints: cfg.SMLAllowPublicEndpoints, AllowedPorts: cfg.SMLAllowedPorts}
		connection, openErr := sml.NewConnectionService(database.NewSMLConnectionStore(pool), box, policy, nil, time.Now).Open(ctx, tenantID)
		if openErr != nil {
			return 0, errors.New("tenant SML connection could not be opened")
		}
		checker.SQL = smlRunner{client: sml.NewClient(policy, queryTimeout, 4*1024*1024, 500), connection: connection}
	}
	result := checker.Run(ctx, tenantID)

	if strings.EqualFold(os.Getenv("ONBOARDING_FORMAT"), "json") {
		encoded, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(encoded))
	} else {
		printResult(result)
	}
	if !result.Ready() {
		return 1, nil
	}
	return 0, nil
}

func printResult(result onboarding.Result) {
	icon := map[onboarding.Status]string{onboarding.Pass: "[ผ่าน]", onboarding.Warn: "[ระวัง]", onboarding.Fail: "[ไม่ผ่าน]", onboarding.Info: "[ข้อมูล]"}
	fmt.Printf("ตรวจความพร้อมร้าน %s เมื่อ %s\n", result.TenantID, result.GeneratedAt.Format("2006-01-02 15:04"))
	area := ""
	for _, item := range result.Items {
		if item.Area != area {
			area = item.Area
			fmt.Printf("\n== %s\n", area)
		}
		fmt.Printf("%-9s %s\n", icon[item.Status], item.Title)
		if item.Detail != "" {
			fmt.Printf("          %s\n", item.Detail)
		}
	}
	counts := result.Counts()
	fmt.Printf("\nสรุป: ผ่าน %d · ระวัง %d · ไม่ผ่าน %d · ข้อมูล %d\n", counts[onboarding.Pass], counts[onboarding.Warn], counts[onboarding.Fail], counts[onboarding.Info])
	if result.Ready() {
		fmt.Println("ไม่มีข้อที่ไม่ผ่าน (อ่านข้อที่ระวังก่อนเปิดใช้งาน)")
	}
}
