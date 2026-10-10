package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadRequiresProductionConfiguration(t *testing.T) {
	_, err := Load(func(key string) (string, bool) {
		values := map[string]string{"APP_ENV": "production"}
		value, ok := values[key]
		return value, ok
	})

	if err == nil {
		t.Fatal("expected missing production configuration to fail")
	}
	for _, name := range []string{
		"DATABASE_URL",
		"PUBLIC_BASE_URL",
		"ADMIN_PASSWORD_HASH",
		"SESSION_HMAC_KEY",
		"ENCRYPTION_MASTER_KEY",
		"ENCRYPTION_KEY_ID",
		"SML_ALLOWED_CIDRS",
		"LINE_LOGIN_CHANNEL_ID",
		"LINE_MESSAGING_CHANNEL_ACCESS_TOKEN",
	} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not mention %s", err, name)
		}
	}
}

func TestLoadAcceptsSafeProductionConfiguration(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	values := map[string]string{
		"APP_ENV":                             "production",
		"HTTP_ADDR":                           ":8080",
		"DATABASE_URL":                        "postgres://nextstep@example.internal/nextstep?sslmode=verify-full",
		"PUBLIC_BASE_URL":                     "https://dashboard.nextstep-soft.com",
		"ADMIN_USERNAME":                      "superadmin",
		"ADMIN_PASSWORD_HASH":                 "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"SESSION_HMAC_KEY":                    secret,
		"ENCRYPTION_MASTER_KEY":               secret,
		"ENCRYPTION_KEY_ID":                   "key-2026-01",
		"SML_ALLOWED_CIDRS":                   "10.0.0.0/8,192.168.0.0/16",
		"SML_ALLOWED_HOSTS":                   "sml-shop.example.com",
		"SML_ALLOW_PUBLIC_ENDPOINTS":          "true",
		"SML_ALLOWED_PORTS":                   "80,443,8080,8092",
		"LINE_LOGIN_CHANNEL_ID":               "2010662588",
		"LINE_MESSAGING_CHANNEL_ACCESS_TOKEN": strings.Repeat("x", 64),
		"DATABASE_MAX_CONNECTIONS":            "24",
		"DATABASE_MIN_CONNECTIONS":            "3",
		"SNAPSHOT_FIRST_ENABLED":              "true",
		"SNAPSHOT_FIRST_TENANT_IDS":           "a904bc92-a89b-463b-bc2a-565f09cbef44",
		"SMART_SCHEDULE_PERIODS_ENABLED":      "true",
		"SMART_SCHEDULE_PERIOD_TENANT_IDS":    "a904bc92-a89b-463b-bc2a-565f09cbef44",
		"SUMMARY_QUERY_ENABLED":               "true",
		"GENERATION_CACHE_ENABLED":            "true",
		"STALE_REVALIDATION_ENABLED":          "true",
		"HEAVY_CHUNK_ENABLED":                 "true",
		"HEAVY_CHUNK_TENANT_REPORTS":          "11111111-1111-1111-1111-111111111111/stock_balance",
		"SCHEDULE_CHUNK_ENABLED":              "false",
	}

	cfg, err := Load(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.PublicBaseURL.String() != values["PUBLIC_BASE_URL"] {
		t.Fatalf("PublicBaseURL = %q", cfg.PublicBaseURL.String())
	}
	if len(cfg.EncryptionMasterKey) != 32 {
		t.Fatalf("EncryptionMasterKey length = %d", len(cfg.EncryptionMasterKey))
	}
	if cfg.ReportWorkerConcurrency != 4 {
		t.Fatalf("ReportWorkerConcurrency = %d", cfg.ReportWorkerConcurrency)
	}
	if cfg.BackupPolicy != "PRE_MIGRATION_ONLY" {
		t.Fatalf("BackupPolicy = %q", cfg.BackupPolicy)
	}
	if cfg.DeliveryWorkerConcurrency != 4 {
		t.Fatalf("DeliveryWorkerConcurrency = %d", cfg.DeliveryWorkerConcurrency)
	}
	if cfg.DatabaseMaxConnections != 24 || cfg.DatabaseMinConnections != 3 {
		t.Fatalf("database pool = %d/%d, want 3/24", cfg.DatabaseMinConnections, cfg.DatabaseMaxConnections)
	}
	if cfg.LineLoginChannelID != values["LINE_LOGIN_CHANNEL_ID"] {
		t.Fatalf("LineLoginChannelID = %q", cfg.LineLoginChannelID)
	}
	if cfg.LineMessagingAccessToken != values["LINE_MESSAGING_CHANNEL_ACCESS_TOKEN"] {
		t.Fatal("LineMessagingAccessToken was not loaded")
	}
	if len(cfg.SMLAllowedHosts) != 1 || cfg.SMLAllowedHosts[0] != "sml-shop.example.com" {
		t.Fatalf("SMLAllowedHosts = %#v", cfg.SMLAllowedHosts)
	}
	if !cfg.SMLAllowPublicEndpoints {
		t.Fatal("SMLAllowPublicEndpoints was not loaded")
	}
	if got := cfg.SMLAllowedPorts; len(got) != 4 || got[0] != 80 || got[1] != 443 || got[2] != 8080 || got[3] != 8092 {
		t.Fatalf("SMLAllowedPorts = %#v", got)
	}
	if !cfg.SnapshotFirstEnabled || len(cfg.SnapshotFirstTenantIDs) != 1 {
		t.Fatalf("snapshot first config = enabled:%v tenants:%v", cfg.SnapshotFirstEnabled, cfg.SnapshotFirstTenantIDs)
	}
	if !cfg.SmartSchedulePeriodsEnabled || len(cfg.SmartSchedulePeriodTenantIDs) != 1 {
		t.Fatalf("smart schedule config = enabled:%v tenants:%v", cfg.SmartSchedulePeriodsEnabled, cfg.SmartSchedulePeriodTenantIDs)
	}
	if !cfg.SummaryQueryEnabled || !cfg.GenerationCacheEnabled || !cfg.StaleRevalidationEnabled || !cfg.HeavyChunkEnabled || len(cfg.HeavyChunkTenantReports) != 1 || cfg.ScheduleChunkEnabled {
		t.Fatalf("dashboard feature flags were not loaded: %+v", cfg)
	}
}

func TestLoadEnablesBoundedSummaryQueriesByDefault(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	values := map[string]string{
		"DATABASE_URL":          "postgres://nextstep@localhost/nextstep?sslmode=disable",
		"PUBLIC_BASE_URL":       "http://localhost:6324",
		"ADMIN_PASSWORD_HASH":   "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"SESSION_HMAC_KEY":      secret,
		"ENCRYPTION_MASTER_KEY": secret,
		"ENCRYPTION_KEY_ID":     "key-2026-01",
		"SML_ALLOWED_CIDRS":     "10.0.0.0/8",
	}

	cfg, err := Load(func(key string) (string, bool) { value, ok := values[key]; return value, ok })
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.SummaryQueryEnabled {
		t.Fatal("SummaryQueryEnabled = false, want safe bounded summaries enabled for every tenant by default")
	}

	values["SUMMARY_QUERY_ENABLED"] = "false"
	cfg, err = Load(func(key string) (string, bool) { value, ok := values[key]; return value, ok })
	if err != nil {
		t.Fatalf("Load() with emergency kill switch error = %v", err)
	}
	if cfg.SummaryQueryEnabled {
		t.Fatal("SummaryQueryEnabled = true after explicit false, want emergency kill switch to remain available")
	}
}

func TestParseAllowedPortsAllowsWildcardForAnyPublicEndpointPort(t *testing.T) {
	ports, err := parseAllowedPorts("*")
	if err != nil {
		t.Fatalf("parseAllowedPorts(*) error = %v", err)
	}
	if len(ports) != 0 {
		t.Fatalf("parseAllowedPorts(*) = %#v, want no port restriction", ports)
	}
}

func TestParseAllowedPortsRejectsWildcardMixedWithExplicitPorts(t *testing.T) {
	if _, err := parseAllowedPorts("*,8080"); err == nil {
		t.Fatal("parseAllowedPorts(*,8080) accepted an ambiguous wildcard configuration")
	}
}

func TestLoadRejectsDatabaseMinimumAboveMaximum(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	values := map[string]string{
		"DATABASE_URL":             "postgres://nextstep@localhost/nextstep?sslmode=disable",
		"PUBLIC_BASE_URL":          "http://localhost:6324",
		"ADMIN_PASSWORD_HASH":      "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"SESSION_HMAC_KEY":         secret,
		"ENCRYPTION_MASTER_KEY":    secret,
		"ENCRYPTION_KEY_ID":        "key-2026-01",
		"SML_ALLOWED_CIDRS":        "10.0.0.0/8",
		"DATABASE_MAX_CONNECTIONS": "4",
		"DATABASE_MIN_CONNECTIONS": "5",
	}

	_, err := Load(func(key string) (string, bool) { value, ok := values[key]; return value, ok })
	if err == nil || !strings.Contains(err.Error(), "DATABASE_MIN_CONNECTIONS") {
		t.Fatalf("Load() error = %v, want minimum/maximum validation", err)
	}
}

func TestLoadRejectsUnsafeFeatureFlagCombinations(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	base := map[string]string{
		"DATABASE_URL":          "postgres://nextstep@localhost/nextstep?sslmode=disable",
		"PUBLIC_BASE_URL":       "http://localhost:6324",
		"ADMIN_PASSWORD_HASH":   "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"SESSION_HMAC_KEY":      secret,
		"ENCRYPTION_MASTER_KEY": secret,
		"ENCRYPTION_KEY_ID":     "key-2026-01",
		"SML_ALLOWED_CIDRS":     "10.0.0.0/8",
	}
	tests := []struct {
		name    string
		flags   map[string]string
		message string
	}{
		{name: "generation without summary", flags: map[string]string{"SUMMARY_QUERY_ENABLED": "false", "GENERATION_CACHE_ENABLED": "true"}, message: "GENERATION_CACHE_ENABLED requires SUMMARY_QUERY_ENABLED"},
		{name: "revalidation without generation", flags: map[string]string{"SUMMARY_QUERY_ENABLED": "true", "STALE_REVALIDATION_ENABLED": "true"}, message: "STALE_REVALIDATION_ENABLED requires GENERATION_CACHE_ENABLED"},
		{name: "schedule chunk without heavy chunk", flags: map[string]string{"SCHEDULE_CHUNK_ENABLED": "true"}, message: "SCHEDULE_CHUNK_ENABLED requires HEAVY_CHUNK_ENABLED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := make(map[string]string, len(base)+len(test.flags))
			for key, value := range base {
				values[key] = value
			}
			for key, value := range test.flags {
				values[key] = value
			}
			_, err := Load(func(key string) (string, bool) { value, ok := values[key]; return value, ok })
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Load() error = %v, want %q", err, test.message)
			}
		})
	}

	// The mode table decides what is chunked, so the master switch no longer
	// needs an env allowlist; the list only seeds the table.
	values := make(map[string]string, len(base)+1)
	for key, value := range base {
		values[key] = value
	}
	values["HEAVY_CHUNK_ENABLED"] = "true"
	cfg, err := Load(func(key string) (string, bool) { value, ok := values[key]; return value, ok })
	if err != nil || !cfg.HeavyChunkEnabled || len(cfg.HeavyChunkTenantReports) != 0 {
		t.Fatalf("HEAVY_CHUNK_ENABLED without an allowlist: cfg=%+v err=%v", cfg.HeavyChunkEnabled, err)
	}
}

func TestLoadRejectsUnsafeProductionValuesWithoutEchoingSecrets(t *testing.T) {
	weak := base64.StdEncoding.EncodeToString([]byte("too-short"))
	values := map[string]string{
		"APP_ENV":                             "production",
		"DATABASE_URL":                        "postgres://user:top-secret@localhost/db?sslmode=disable",
		"PUBLIC_BASE_URL":                     "http://dashboard.nextstep-soft.com",
		"ADMIN_PASSWORD_HASH":                 "top-secret-password",
		"SESSION_HMAC_KEY":                    weak,
		"ENCRYPTION_MASTER_KEY":               weak,
		"ENCRYPTION_KEY_ID":                   "key-2026-01",
		"SML_ALLOWED_CIDRS":                   "0.0.0.0/0",
		"LINE_LOGIN_CHANNEL_ID":               "not-a-channel-id",
		"LINE_MESSAGING_CHANNEL_ACCESS_TOKEN": "short-secret",
	}

	_, err := Load(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err == nil {
		t.Fatal("expected unsafe configuration to fail")
	}
	for _, secret := range []string{"top-secret", "top-secret-password", weak} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("configuration error leaked secret %q: %v", secret, err)
		}
	}
}

func TestLoadValidatesOptionalLineChannelSecret(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	base := map[string]string{
		"APP_ENV":                             "production",
		"DATABASE_URL":                        "postgres://nextstep@example.internal/nextstep?sslmode=verify-full",
		"PUBLIC_BASE_URL":                     "https://dashboard.nextstep-soft.com",
		"ADMIN_PASSWORD_HASH":                 "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"SESSION_HMAC_KEY":                    secret,
		"ENCRYPTION_MASTER_KEY":               secret,
		"ENCRYPTION_KEY_ID":                   "key-2026-01",
		"SML_ALLOWED_CIDRS":                   "10.0.0.0/8",
		"LINE_LOGIN_CHANNEL_ID":               "2010662588",
		"LINE_MESSAGING_CHANNEL_ACCESS_TOKEN": strings.Repeat("x", 64),
	}
	load := func(channelSecret string) (Config, error) {
		return Load(func(key string) (string, bool) {
			if key == "LINE_MESSAGING_CHANNEL_SECRET" {
				return channelSecret, channelSecret != ""
			}
			value, ok := base[key]
			return value, ok
		})
	}

	cfg, err := load("")
	if err != nil || cfg.LineMessagingChannelSecret != "" {
		t.Fatalf("absent secret must stay optional: %q, %v", cfg.LineMessagingChannelSecret, err)
	}
	forward := func(value string) (Config, error) {
		return Load(func(key string) (string, bool) {
			if key == "LINE_WEBHOOK_FORWARD_URL" {
				return value, value != ""
			}
			result, ok := base[key]
			return result, ok
		})
	}
	if cfg, err := forward(""); err != nil || cfg.LineWebhookForwardURL != "" {
		t.Fatalf("the pass-on address is optional: %q %v", cfg.LineWebhookForwardURL, err)
	}
	if cfg, err := forward("http://assistant:8646/line/webhook"); err != nil || cfg.LineWebhookForwardURL != "http://assistant:8646/line/webhook" {
		t.Fatalf("a plain internal address must be accepted: %q %v", cfg.LineWebhookForwardURL, err)
	}
	for _, bad := range []string{"assistant:8646/line/webhook", "ftp://assistant/x", "http://user:pass@assistant/x", "http:///x"} {
		if _, err := forward(bad); err == nil {
			t.Fatalf("pass-on address %q must be refused", bad)
		}
	}
	gate := func(value, secret string) (Config, error) {
		return Load(func(key string) (string, bool) {
			switch key {
			case "LINE_FRONT_GATE":
				return value, value != ""
			case "LINE_MESSAGING_CHANNEL_SECRET":
				return secret, secret != ""
			}
			result, ok := base[key]
			return result, ok
		})
	}
	if cfg, err := gate("", ""); err != nil || cfg.LineFrontGate {
		t.Fatalf("the front gate is off by default: %v %v", cfg.LineFrontGate, err)
	}
	if cfg, err := gate("true", "0123456789abcdef0123456789abcdef"); err != nil || !cfg.LineFrontGate {
		t.Fatalf("the front gate must turn on with a channel secret: %v %v", cfg.LineFrontGate, err)
	}
	if _, err := gate("true", ""); err == nil {
		t.Fatal("the front gate cannot run without the channel secret")
	}
	if _, err := gate("maybe", "0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("an unreadable gate switch must be refused")
	}
	cfg, err = load("0123456789abcdef0123456789abcdef")
	if err != nil || cfg.LineMessagingChannelSecret != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("valid secret rejected: %v", err)
	}
	for _, invalid := range []string{"short-secret-value", "0123456789ABCDEF0123456789ABCDEF", strings.Repeat("g", 32)} {
		_, err := load(invalid)
		if err == nil {
			t.Fatalf("invalid secret %q accepted", invalid)
		}
		if strings.Contains(err.Error(), invalid) {
			t.Fatalf("error echoes the secret: %v", err)
		}
	}
}

func alertTestValues(extra map[string]string) func(string) (string, bool) {
	secret := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	values := map[string]string{
		"DATABASE_URL":          "postgres://nextstep@localhost/nextstep?sslmode=disable",
		"PUBLIC_BASE_URL":       "http://localhost:6324",
		"ADMIN_PASSWORD_HASH":   "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"SESSION_HMAC_KEY":      secret,
		"ENCRYPTION_MASTER_KEY": secret,
		"ENCRYPTION_KEY_ID":     "key-2026-01",
		"SML_ALLOWED_CIDRS":     "10.0.0.0/8",
	}
	for key, value := range extra {
		values[key] = value
	}
	return func(key string) (string, bool) { value, ok := values[key]; return value, ok }
}

func TestAlertsAreOffByDefaultAndRecordOnlyUntilSendingIsSwitchedOn(t *testing.T) {
	cfg, err := Load(alertTestValues(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentAlertsEnabled || !cfg.AgentAlertDryRun {
		t.Fatalf("alerts must be off and, if turned on, a dry run: %+v", cfg)
	}
	if cfg.AgentAlertStartMinute != 8*60+15 || cfg.AgentAlertWindowMinutes != 240 {
		t.Fatalf("the check starts at 08:15 and runs four hours by default: %d %d", cfg.AgentAlertStartMinute, cfg.AgentAlertWindowMinutes)
	}
	if _, err := Load(alertTestValues(map[string]string{"AGENT_ALERT_START_MINUTE": "1500"})); err == nil {
		t.Fatal("a start minute past the end of the day must be refused")
	}
	if _, err := Load(alertTestValues(map[string]string{"AGENT_ALERT_WINDOW_MINUTES": "1"})); err == nil {
		t.Fatal("a window shorter than one pass must be refused")
	}
	cfg, err = Load(alertTestValues(map[string]string{"AGENT_ALERTS_ENABLED": "true"}))
	if err != nil || !cfg.AgentAlertsEnabled || !cfg.AgentAlertDryRun {
		t.Fatalf("turning alerts on alone must stay a dry run, with no webhook needed: %+v %v", cfg, err)
	}
}

func TestSendingAlertsNeedsAWebhookASecretAndRoutes(t *testing.T) {
	good := map[string]string{
		"AGENT_ALERTS_ENABLED": "true", "AGENT_ALERT_DRY_RUN": "false",
		"AGENT_ALERT_WEBHOOK_URL":    " http://assistant:8644/ ",
		"AGENT_ALERT_WEBHOOK_SECRET": strings.Repeat("s", 32),
		"AGENT_ALERT_WEBHOOK_ROUTES": " alert-1, alert-2 ,,",
	}
	cfg, err := Load(alertTestValues(good))
	if err != nil || cfg.AgentAlertDryRun || cfg.AgentAlertWebhookURL != "http://assistant:8644" || len(cfg.AgentAlertWebhookRoutes) != 2 || cfg.AgentAlertWebhookRoutes[1] != "alert-2" {
		t.Fatalf("good config: %+v %v", cfg, err)
	}
	for name, change := range map[string]map[string]string{
		"no url":       {"AGENT_ALERT_WEBHOOK_URL": ""},
		"not http":     {"AGENT_ALERT_WEBHOOK_URL": "assistant:8644"},
		"short secret": {"AGENT_ALERT_WEBHOOK_SECRET": "too-short"},
		"no secret":    {"AGENT_ALERT_WEBHOOK_SECRET": ""},
		"no routes":    {"AGENT_ALERT_WEBHOOK_ROUTES": " , "},
		"bad dry-run":  {"AGENT_ALERT_DRY_RUN": "maybe"},
		"bad enabled":  {"AGENT_ALERTS_ENABLED": "yes please"},
	} {
		values := map[string]string{}
		for key, value := range good {
			values[key] = value
		}
		for key, value := range change {
			values[key] = value
		}
		_, err := Load(alertTestValues(values))
		if err == nil {
			t.Errorf("%s must be refused", name)
		} else if strings.Contains(err.Error(), strings.Repeat("s", 32)) {
			t.Errorf("%s: the error must not echo the secret: %v", name, err)
		}
	}
	// A dry run needs none of it, so a half-finished setup cannot send by accident.
	dry := map[string]string{"AGENT_ALERTS_ENABLED": "true", "AGENT_ALERT_DRY_RUN": "true", "AGENT_ALERT_WEBHOOK_SECRET": "short"}
	if _, err := Load(alertTestValues(dry)); err != nil {
		t.Fatalf("dry run with leftovers: %v", err)
	}
}

func TestMasterDataCopyIsOffUntilSwitchedOn(t *testing.T) {
	cfg, err := Load(alertTestValues(nil))
	if err != nil || cfg.MasterSyncEnabled {
		t.Fatalf("default: %v %v", cfg.MasterSyncEnabled, err)
	}
	if cfg.AgentLiveLookupsEnabled {
		t.Fatal("live lookups are off by default")
	}
	if cfg.MasterSyncStartMinute != 6*60+30 || cfg.MasterSyncWindowMinutes != 240 {
		t.Fatalf("the copy starts at 06:30 and runs four hours by default: %d %d", cfg.MasterSyncStartMinute, cfg.MasterSyncWindowMinutes)
	}
	if cfg, err = Load(alertTestValues(map[string]string{"MASTER_SYNC_ENABLED": "true"})); err != nil || !cfg.MasterSyncEnabled {
		t.Fatalf("on: %v %v", cfg.MasterSyncEnabled, err)
	}
	if _, err = Load(alertTestValues(map[string]string{"MASTER_SYNC_START_MINUTE": "1500"})); err == nil {
		t.Fatal("a start minute past the end of the day must be refused")
	}
}

func TestMonthlyCallBudgetIsOffByDefaultAndBounded(t *testing.T) {
	cfg, err := Load(alertTestValues(nil))
	if err != nil || cfg.AgentMonthlyCallBudget != 0 {
		t.Fatalf("default: %d %v", cfg.AgentMonthlyCallBudget, err)
	}
	if cfg, err = Load(alertTestValues(map[string]string{"AGENT_MONTHLY_CALL_BUDGET": "3000"})); err != nil || cfg.AgentMonthlyCallBudget != 3000 {
		t.Fatalf("set: %d %v", cfg.AgentMonthlyCallBudget, err)
	}
	if _, err = Load(alertTestValues(map[string]string{"AGENT_MONTHLY_CALL_BUDGET": "-1"})); err == nil {
		t.Fatal("a negative budget must be refused")
	}
}

func TestAgentHourlyLimitsDefaultAndCanBeRaisedButNotZeroedByAccident(t *testing.T) {
	cfg, err := Load(alertTestValues(nil))
	if err != nil || cfg.AgentCallsPerHour != 60 || cfg.AgentRefreshesPerHour != 10 || cfg.AgentLookupsPerHour != 30 {
		t.Fatalf("defaults: %d %d %d %v", cfg.AgentCallsPerHour, cfg.AgentRefreshesPerHour, cfg.AgentLookupsPerHour, err)
	}
	cfg, err = Load(alertTestValues(map[string]string{"AGENT_CALLS_PER_HOUR": "1000000", "AGENT_REFRESHES_PER_HOUR": "500", "AGENT_LOOKUPS_PER_HOUR": "9999"}))
	if err != nil || cfg.AgentCallsPerHour != 1_000_000 || cfg.AgentRefreshesPerHour != 500 || cfg.AgentLookupsPerHour != 9999 {
		t.Fatalf("raised: %d %d %d %v", cfg.AgentCallsPerHour, cfg.AgentRefreshesPerHour, cfg.AgentLookupsPerHour, err)
	}
	for _, name := range []string{"AGENT_CALLS_PER_HOUR", "AGENT_REFRESHES_PER_HOUR", "AGENT_LOOKUPS_PER_HOUR"} {
		for _, bad := range []string{"0", "-1", "many"} {
			if _, err := Load(alertTestValues(map[string]string{name: bad})); err == nil {
				t.Fatalf("%s=%s was accepted", name, bad)
			}
		}
	}
}
