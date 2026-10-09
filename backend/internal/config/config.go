package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const production = "production"

type LookupFunc func(string) (string, bool)

type Config struct {
	Environment                string
	HTTPAddr                   string
	DatabaseURL                string
	PublicBaseURL              *url.URL
	AdminUsername              string
	AdminPasswordHash          string
	SessionHMACKey             []byte
	EncryptionMasterKey        []byte
	EncryptionKeyID            string
	LineLoginChannelID         string
	LineMessagingAccessToken   string
	LineMessagingChannelSecret string
	// LineWebhookForwardURL, when set, is where a LINE webhook that passed the signature check is passed on to (the assistant).
	LineWebhookForwardURL        string
	SMLAllowedPrefixes           []netip.Prefix
	SMLAllowedHosts              []string
	SMLAllowPublicEndpoints      bool
	SMLAllowedPorts              []uint16
	DatabaseMaxConnections       int
	DatabaseMinConnections       int
	ReportWorkerConcurrency      int
	DeliveryWorkerConcurrency    int
	SnapshotFirstEnabled         bool
	SnapshotFirstTenantIDs       []uuid.UUID
	SmartSchedulePeriodsEnabled  bool
	SmartSchedulePeriodTenantIDs []uuid.UUID
	SummaryQueryEnabled          bool
	GenerationCacheEnabled       bool
	StaleRevalidationEnabled     bool
	HeavyChunkEnabled            bool
	AgentAPIEnabled              bool
	// Alerts the assistant sets up are checked once a day by the worker. They are off until an operator turns them on,
	// and then only record what they would have said (dry run) until sending is switched on too.
	AgentAlertsEnabled bool
	// The worker copies the shop's customers, suppliers and items from SML once a day for the assistant's search.
	MasterSyncEnabled bool
	// The assistant can ask the shop's system one narrow live question about a known customer or item. Off until switched on.
	AgentLiveLookupsEnabled bool
	// AgentCallsPerHour is how many counted calls one assistant token may make in a sliding hour (default 60). AgentRefreshesPerHour is
	// how many live fetches from a shop's SML the assistant may start per shop per hour (default 10), and AgentLookupsPerHour how many
	// live customer, supplier or item reads it may make per shop per hour (default 30). Raise them to lift the limit; a value that is
	// never reached (1,000,000) removes it. They protect a shop's own SML from being asked too often, and the model bill from a loop.
	AgentCallsPerHour     int
	AgentRefreshesPerHour int
	AgentLookupsPerHour   int
	// AgentMonthlyCallBudget caps the assistant calls a shop may make in a calendar month (0 = no cap). It is the cost brake.
	AgentMonthlyCallBudget  int
	MasterSyncStartMinute   int
	MasterSyncWindowMinutes int
	AgentAlertDryRun        bool
	AgentAlertWebhookURL    string
	AgentAlertWebhookSecret string
	AgentAlertWebhookRoutes []string
	// The daily check starts at this minute of the shop's day and gives up after the window (default 08:15 and four hours).
	AgentAlertStartMinute    int
	AgentAlertWindowMinutes  int
	HeavyChunkTenantReports  []string
	ScheduleChunkEnabled     bool
	WatchdogEnabled          bool
	SentinelRuntimeDirectory string
	BackupPolicy             string
}

func Load(lookup LookupFunc) (Config, error) {
	environment := valueOrDefault(lookup, "APP_ENV", "development")
	httpAddr := valueOrDefault(lookup, "HTTP_ADDR", ":8080")
	adminUsername := valueOrDefault(lookup, "ADMIN_USERNAME", "superadmin")

	requiredNames := []string{
		"DATABASE_URL",
		"PUBLIC_BASE_URL",
		"ADMIN_PASSWORD_HASH",
		"SESSION_HMAC_KEY",
		"ENCRYPTION_MASTER_KEY",
		"ENCRYPTION_KEY_ID",
		"SML_ALLOWED_CIDRS",
	}
	if environment == production {
		requiredNames = append(requiredNames, "LINE_LOGIN_CHANNEL_ID", "LINE_MESSAGING_CHANNEL_ACCESS_TOKEN")
	}
	values := make(map[string]string, len(requiredNames))
	missing := make([]string, 0, len(requiredNames))
	for _, name := range requiredNames {
		value, ok := lookup(name)
		value = strings.TrimSpace(value)
		if !ok || value == "" {
			missing = append(missing, name)
			continue
		}
		values[name] = value
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return Config{}, fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}

	publicBaseURL, err := url.Parse(values["PUBLIC_BASE_URL"])
	if err != nil || publicBaseURL.Host == "" {
		return Config{}, errors.New("PUBLIC_BASE_URL must be an absolute URL")
	}
	if environment == production && publicBaseURL.Scheme != "https" {
		return Config{}, errors.New("PUBLIC_BASE_URL must use HTTPS in production")
	}

	if err := validateDatabaseURL(values["DATABASE_URL"], environment); err != nil {
		return Config{}, err
	}
	if !strings.HasPrefix(values["ADMIN_PASSWORD_HASH"], "$argon2id$") {
		return Config{}, errors.New("ADMIN_PASSWORD_HASH must be an Argon2id encoded hash")
	}
	if len(values["ENCRYPTION_KEY_ID"]) > 64 || strings.ContainsAny(values["ENCRYPTION_KEY_ID"], " \t\r\n") {
		return Config{}, errors.New("ENCRYPTION_KEY_ID must be a compact identifier of at most 64 characters")
	}
	lineLoginChannelID := strings.TrimSpace(values["LINE_LOGIN_CHANNEL_ID"])
	if lineLoginChannelID == "" {
		lineLoginChannelID, _ = lookup("LINE_LOGIN_CHANNEL_ID")
		lineLoginChannelID = strings.TrimSpace(lineLoginChannelID)
	}
	if lineLoginChannelID != "" && (len(lineLoginChannelID) > 32 || strings.Trim(lineLoginChannelID, "0123456789") != "") {
		return Config{}, errors.New("LINE_LOGIN_CHANNEL_ID must contain 1 to 32 digits")
	}
	lineMessagingAccessToken := strings.TrimSpace(values["LINE_MESSAGING_CHANNEL_ACCESS_TOKEN"])
	if lineMessagingAccessToken == "" {
		lineMessagingAccessToken, _ = lookup("LINE_MESSAGING_CHANNEL_ACCESS_TOKEN")
		lineMessagingAccessToken = strings.TrimSpace(lineMessagingAccessToken)
	}
	if lineMessagingAccessToken != "" && (len(lineMessagingAccessToken) < 32 || len(lineMessagingAccessToken) > 4096 || strings.ContainsAny(lineMessagingAccessToken, " \t\r\n")) {
		return Config{}, errors.New("LINE_MESSAGING_CHANNEL_ACCESS_TOKEN must be a compact token containing 32 to 4096 characters")
	}
	lineMessagingChannelSecret, _ := lookup("LINE_MESSAGING_CHANNEL_SECRET")
	lineMessagingChannelSecret = strings.TrimSpace(lineMessagingChannelSecret)
	if lineMessagingChannelSecret != "" && !isLowerHex(lineMessagingChannelSecret, 32) {
		return Config{}, errors.New("LINE_MESSAGING_CHANNEL_SECRET must be the 32-character hex channel secret")
	}
	lineWebhookForwardURL, _ := lookup("LINE_WEBHOOK_FORWARD_URL")
	lineWebhookForwardURL = strings.TrimSpace(lineWebhookForwardURL)
	if lineWebhookForwardURL != "" {
		parsed, parseErr := url.Parse(lineWebhookForwardURL)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return Config{}, errors.New("LINE_WEBHOOK_FORWARD_URL must be an http or https URL with a host and no credentials")
		}
	}
	allowedPrefixes, err := parseAllowedPrefixes(values["SML_ALLOWED_CIDRS"])
	if err != nil {
		return Config{}, err
	}
	allowedHosts, err := parseAllowedHosts(valueOrDefault(lookup, "SML_ALLOWED_HOSTS", ""))
	if err != nil {
		return Config{}, err
	}
	allowPublicEndpoints, err := boolValue(lookup, "SML_ALLOW_PUBLIC_ENDPOINTS", false)
	if err != nil {
		return Config{}, err
	}
	allowedPorts, err := parseAllowedPorts(valueOrDefault(lookup, "SML_ALLOWED_PORTS", "*"))
	if err != nil {
		return Config{}, err
	}
	workerConcurrency, err := intValue(lookup, "REPORT_WORKER_CONCURRENCY", 4, 1, 16)
	if err != nil {
		return Config{}, err
	}
	deliveryConcurrency, err := intValue(lookup, "DELIVERY_WORKER_CONCURRENCY", 4, 1, 16)
	if err != nil {
		return Config{}, err
	}
	databaseMaxConnections, err := intValue(lookup, "DATABASE_MAX_CONNECTIONS", 20, 2, 200)
	if err != nil {
		return Config{}, err
	}
	databaseMinConnections, err := intValue(lookup, "DATABASE_MIN_CONNECTIONS", 2, 0, 50)
	if err != nil {
		return Config{}, err
	}
	if databaseMinConnections > databaseMaxConnections {
		return Config{}, errors.New("DATABASE_MIN_CONNECTIONS must not exceed DATABASE_MAX_CONNECTIONS")
	}
	snapshotFirstEnabled, err := boolValue(lookup, "SNAPSHOT_FIRST_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	snapshotFirstTenantIDs, err := parseUUIDList("SNAPSHOT_FIRST_TENANT_IDS", valueOrDefault(lookup, "SNAPSHOT_FIRST_TENANT_IDS", ""))
	if err != nil {
		return Config{}, err
	}
	smartSchedulePeriodsEnabled, err := boolValue(lookup, "SMART_SCHEDULE_PERIODS_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	smartSchedulePeriodTenantIDs, err := parseUUIDList("SMART_SCHEDULE_PERIOD_TENANT_IDS", valueOrDefault(lookup, "SMART_SCHEDULE_PERIOD_TENANT_IDS", ""))
	if err != nil {
		return Config{}, err
	}
	// Bounded aggregate queries are the safe production default for every
	// tenant. Operators can still set the flag to false as an emergency kill
	// switch, but doing so intentionally falls back to the heavier detail plan.
	summaryQueryEnabled, err := boolValue(lookup, "SUMMARY_QUERY_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	generationCacheEnabled, err := boolValue(lookup, "GENERATION_CACHE_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	staleRevalidationEnabled, err := boolValue(lookup, "STALE_REVALIDATION_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	// The assistant surface is off until an operator turns it on.
	agentAPIEnabled, err := boolValue(lookup, "AGENT_API_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	agentMonthlyCallBudget, err := intValue(lookup, "AGENT_MONTHLY_CALL_BUDGET", 0, 0, 1_000_000)
	if err != nil {
		return Config{}, err
	}
	agentCallsPerHour, err := intValue(lookup, "AGENT_CALLS_PER_HOUR", 60, 1, 1_000_000)
	if err != nil {
		return Config{}, err
	}
	agentRefreshesPerHour, err := intValue(lookup, "AGENT_REFRESHES_PER_HOUR", 10, 1, 1_000_000)
	if err != nil {
		return Config{}, err
	}
	agentLookupsPerHour, err := intValue(lookup, "AGENT_LOOKUPS_PER_HOUR", 30, 1, 1_000_000)
	if err != nil {
		return Config{}, err
	}
	liveLookupsEnabled, err := boolValue(lookup, "AGENT_LIVE_LOOKUPS_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	masterSyncEnabled, err := boolValue(lookup, "MASTER_SYNC_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	agentAlertsEnabled, err := boolValue(lookup, "AGENT_ALERTS_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	agentAlertDryRun, err := boolValue(lookup, "AGENT_ALERT_DRY_RUN", true)
	if err != nil {
		return Config{}, err
	}
	agentAlertWebhookURL := strings.TrimRight(strings.TrimSpace(valueOrDefault(lookup, "AGENT_ALERT_WEBHOOK_URL", "")), "/")
	agentAlertWebhookSecret := strings.TrimSpace(valueOrDefault(lookup, "AGENT_ALERT_WEBHOOK_SECRET", ""))
	agentAlertWebhookRoutes := make([]string, 0)
	for _, route := range strings.Split(valueOrDefault(lookup, "AGENT_ALERT_WEBHOOK_ROUTES", ""), ",") {
		if route = strings.TrimSpace(route); route != "" {
			agentAlertWebhookRoutes = append(agentAlertWebhookRoutes, route)
		}
	}
	masterSyncStartMinute, err := intValue(lookup, "MASTER_SYNC_START_MINUTE", 6*60+30, 0, 23*60+59)
	if err != nil {
		return Config{}, err
	}
	masterSyncWindowMinutes, err := intValue(lookup, "MASTER_SYNC_WINDOW_MINUTES", 240, 5, 24*60)
	if err != nil {
		return Config{}, err
	}
	agentAlertStartMinute, err := intValue(lookup, "AGENT_ALERT_START_MINUTE", 8*60+15, 0, 23*60+59)
	if err != nil {
		return Config{}, err
	}
	agentAlertWindowMinutes, err := intValue(lookup, "AGENT_ALERT_WINDOW_MINUTES", 240, 5, 24*60)
	if err != nil {
		return Config{}, err
	}
	if agentAlertsEnabled && !agentAlertDryRun {
		if _, parseErr := url.Parse(agentAlertWebhookURL); parseErr != nil || !strings.HasPrefix(agentAlertWebhookURL, "http") {
			return Config{}, fmt.Errorf("AGENT_ALERT_WEBHOOK_URL must be an http(s) URL when alerts are sent")
		}
		if len(agentAlertWebhookSecret) < 32 {
			return Config{}, fmt.Errorf("AGENT_ALERT_WEBHOOK_SECRET must be at least 32 characters when alerts are sent")
		}
		if len(agentAlertWebhookRoutes) == 0 {
			return Config{}, fmt.Errorf("AGENT_ALERT_WEBHOOK_ROUTES must name at least one route when alerts are sent")
		}
	}
	heavyChunkEnabled, err := boolValue(lookup, "HEAVY_CHUNK_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	heavyChunkTenantReports, err := parseTenantReportList("HEAVY_CHUNK_TENANT_REPORTS", valueOrDefault(lookup, "HEAVY_CHUNK_TENANT_REPORTS", ""))
	if err != nil {
		return Config{}, err
	}
	scheduleChunkEnabled, err := boolValue(lookup, "SCHEDULE_CHUNK_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	if generationCacheEnabled && !summaryQueryEnabled {
		return Config{}, errors.New("GENERATION_CACHE_ENABLED requires SUMMARY_QUERY_ENABLED")
	}
	if staleRevalidationEnabled && !generationCacheEnabled {
		return Config{}, errors.New("STALE_REVALIDATION_ENABLED requires GENERATION_CACHE_ENABLED")
	}
	if scheduleChunkEnabled && !heavyChunkEnabled {
		return Config{}, errors.New("SCHEDULE_CHUNK_ENABLED requires HEAVY_CHUNK_ENABLED")
	}
	// HEAVY_CHUNK_TENANT_REPORTS only seeds the report_execution_modes table now;
	// the table decides which reports are chunked, so an empty list is valid.
	watchdogEnabled, err := boolValue(lookup, "WATCHDOG_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	sentinelRuntimeDirectory := valueOrDefault(lookup, "SENTINEL_RUNTIME_DIR", "/run/nextstep-dashboard")
	if !filepath.IsAbs(sentinelRuntimeDirectory) {
		return Config{}, errors.New("SENTINEL_RUNTIME_DIR must be an absolute path")
	}
	backupPolicy := valueOrDefault(lookup, "BACKUP_POLICY", "PRE_MIGRATION_ONLY")
	if backupPolicy != "PRE_MIGRATION_ONLY" && backupPolicy != "LOCAL_DAILY" && backupPolicy != "LOCAL_AND_OFFSITE" {
		return Config{}, errors.New("BACKUP_POLICY must be PRE_MIGRATION_ONLY, LOCAL_DAILY, or LOCAL_AND_OFFSITE")
	}

	sessionKey, err := decodeKey("SESSION_HMAC_KEY", values["SESSION_HMAC_KEY"], 32, false)
	if err != nil {
		return Config{}, err
	}
	encryptionKey, err := decodeKey("ENCRYPTION_MASTER_KEY", values["ENCRYPTION_MASTER_KEY"], 32, true)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Environment:                  environment,
		HTTPAddr:                     httpAddr,
		DatabaseURL:                  values["DATABASE_URL"],
		PublicBaseURL:                publicBaseURL,
		AdminUsername:                adminUsername,
		AdminPasswordHash:            values["ADMIN_PASSWORD_HASH"],
		SessionHMACKey:               sessionKey,
		EncryptionMasterKey:          encryptionKey,
		EncryptionKeyID:              values["ENCRYPTION_KEY_ID"],
		LineLoginChannelID:           lineLoginChannelID,
		LineMessagingAccessToken:     lineMessagingAccessToken,
		LineMessagingChannelSecret:   lineMessagingChannelSecret,
		LineWebhookForwardURL:        lineWebhookForwardURL,
		SMLAllowedPrefixes:           allowedPrefixes,
		SMLAllowedHosts:              allowedHosts,
		SMLAllowPublicEndpoints:      allowPublicEndpoints,
		SMLAllowedPorts:              allowedPorts,
		DatabaseMaxConnections:       databaseMaxConnections,
		DatabaseMinConnections:       databaseMinConnections,
		ReportWorkerConcurrency:      workerConcurrency,
		DeliveryWorkerConcurrency:    deliveryConcurrency,
		SnapshotFirstEnabled:         snapshotFirstEnabled,
		SnapshotFirstTenantIDs:       snapshotFirstTenantIDs,
		SmartSchedulePeriodsEnabled:  smartSchedulePeriodsEnabled,
		SmartSchedulePeriodTenantIDs: smartSchedulePeriodTenantIDs,
		SummaryQueryEnabled:          summaryQueryEnabled,
		GenerationCacheEnabled:       generationCacheEnabled,
		StaleRevalidationEnabled:     staleRevalidationEnabled,
		HeavyChunkEnabled:            heavyChunkEnabled,
		AgentAPIEnabled:              agentAPIEnabled,
		AgentAlertsEnabled:           agentAlertsEnabled,
		MasterSyncEnabled:            masterSyncEnabled,
		AgentLiveLookupsEnabled:      liveLookupsEnabled,
		AgentCallsPerHour:            agentCallsPerHour,
		AgentRefreshesPerHour:        agentRefreshesPerHour,
		AgentLookupsPerHour:          agentLookupsPerHour,
		AgentMonthlyCallBudget:       agentMonthlyCallBudget,
		MasterSyncStartMinute:        masterSyncStartMinute,
		MasterSyncWindowMinutes:      masterSyncWindowMinutes,
		AgentAlertDryRun:             agentAlertDryRun,
		AgentAlertWebhookURL:         agentAlertWebhookURL,
		AgentAlertWebhookSecret:      agentAlertWebhookSecret,
		AgentAlertWebhookRoutes:      agentAlertWebhookRoutes,
		AgentAlertStartMinute:        agentAlertStartMinute,
		AgentAlertWindowMinutes:      agentAlertWindowMinutes,
		HeavyChunkTenantReports:      heavyChunkTenantReports,
		ScheduleChunkEnabled:         scheduleChunkEnabled,
		WatchdogEnabled:              watchdogEnabled,
		SentinelRuntimeDirectory:     sentinelRuntimeDirectory,
		BackupPolicy:                 backupPolicy,
	}, nil
}

func parseAllowedPorts(raw string) ([]uint16, error) {
	raw = strings.TrimSpace(raw)
	if raw == "*" {
		return []uint16{}, nil
	}
	if strings.Contains(raw, "*") {
		return nil, errors.New("SML_ALLOWED_PORTS must be * or contain comma-separated ports between 1 and 65535")
	}
	parts := strings.Split(raw, ",")
	ports := make([]uint16, 0, len(parts))
	seen := make(map[uint16]struct{}, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseUint(strings.TrimSpace(part), 10, 16)
		if err != nil || value == 0 {
			return nil, errors.New("SML_ALLOWED_PORTS must be * or contain comma-separated ports between 1 and 65535")
		}
		port := uint16(value)
		if _, duplicate := seen[port]; duplicate {
			return nil, errors.New("SML_ALLOWED_PORTS must not contain duplicate ports")
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	return ports, nil
}

func parseTenantReportList(name, raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	allowed := map[string]struct{}{"stock_balance": {}, "ar_customer_movement": {}}
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		fields := strings.Split(strings.TrimSpace(part), "/")
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s must contain comma-separated tenant-uuid/report-key pairs", name)
		}
		tenantID, err := uuid.Parse(fields[0])
		if err != nil {
			return nil, fmt.Errorf("%s contains an invalid tenant UUID", name)
		}
		reportKey := strings.TrimSpace(fields[1])
		if _, ok := allowed[reportKey]; !ok {
			return nil, fmt.Errorf("%s supports only stock_balance and ar_customer_movement", name)
		}
		canonical := tenantID.String() + "/" + reportKey
		if _, exists := seen[canonical]; !exists {
			seen[canonical] = struct{}{}
			result = append(result, canonical)
		}
	}
	return result, nil
}

func boolValue(lookup LookupFunc, name string, fallback bool) (bool, error) {
	raw := valueOrDefault(lookup, name, strconv.FormatBool(fallback))
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return value, nil
}

func parseUUIDList(name, raw string) ([]uuid.UUID, error) {
	if strings.TrimSpace(raw) == "" {
		return []uuid.UUID{}, nil
	}
	result := make([]uuid.UUID, 0)
	seen := make(map[uuid.UUID]struct{})
	for _, part := range strings.Split(raw, ",") {
		id, err := uuid.Parse(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("%s must contain comma-separated UUIDs", name)
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	return result, nil
}

func parseAllowedHosts(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	seen := make(map[string]struct{})
	hosts := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(part), "."))
		if !validAllowedHostname(host) {
			return nil, errors.New("SML_ALLOWED_HOSTS must contain exact ASCII hostnames")
		}
		if _, exists := seen[host]; !exists {
			seen[host] = struct{}{}
			hosts = append(hosts, host)
		}
	}
	slices.Sort(hosts)
	return hosts, nil
}

func validAllowedHostname(host string) bool {
	if len(host) < 1 || len(host) > 253 || !strings.Contains(host, ".") || strings.Contains(host, "..") {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

func intValue(lookup LookupFunc, name string, fallback, minimum, maximum int) (int, error) {
	raw := valueOrDefault(lookup, name, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minimum, maximum)
	}
	return value, nil
}

func parseAllowedPrefixes(raw string) ([]netip.Prefix, error) {
	parts := strings.Split(raw, ",")
	prefixes := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil {
			return nil, errors.New("SML_ALLOWED_CIDRS must contain valid CIDR prefixes")
		}
		prefix = prefix.Masked()
		if (prefix.Addr().Is4() && prefix.Bits() < 8) || (prefix.Addr().Is6() && prefix.Bits() < 32) {
			return nil, errors.New("SML_ALLOWED_CIDRS contains an excessively broad prefix")
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

func valueOrDefault(lookup LookupFunc, name, fallback string) string {
	value, ok := lookup(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func validateDatabaseURL(rawURL, environment string) error {
	databaseURL, err := url.Parse(rawURL)
	if err != nil || databaseURL.Host == "" {
		return errors.New("DATABASE_URL must be an absolute PostgreSQL URL")
	}
	if databaseURL.Scheme != "postgres" && databaseURL.Scheme != "postgresql" {
		return errors.New("DATABASE_URL must use postgres or postgresql scheme")
	}
	if environment != production {
		return nil
	}
	hostname := strings.ToLower(databaseURL.Hostname())
	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		return errors.New("DATABASE_URL must not use a loopback host in production")
	}
	sslMode := strings.ToLower(databaseURL.Query().Get("sslmode"))
	if sslMode == "" || sslMode == "disable" || sslMode == "allow" || sslMode == "prefer" {
		return errors.New("DATABASE_URL must require verified TLS in production")
	}
	return nil
}

func decodeKey(name, encoded string, minimumLength int, exact bool) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%s must be valid base64", name)
	}
	if exact && len(decoded) != minimumLength {
		return nil, fmt.Errorf("%s must decode to exactly %d bytes", name, minimumLength)
	}
	if !exact && len(decoded) < minimumLength {
		return nil, fmt.Errorf("%s must decode to at least %d bytes", name, minimumLength)
	}
	return decoded, nil
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
