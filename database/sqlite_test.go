package database

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSQLiteInitializesFreshDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	if got := db.Driver(); got != "sqlite" {
		t.Fatalf("Driver() = %q, want %q", got, "sqlite")
	}
}

func TestSQLiteAPIKeyLookupAndCount(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	key := "sk-test-lookup-1234567890"
	id, err := db.InsertAPIKey(ctx, "lookup", key)
	if err != nil {
		t.Fatalf("InsertAPIKey 返回错误: %v", err)
	}
	count, err := db.CountAPIKeys(ctx)
	if err != nil {
		t.Fatalf("CountAPIKeys 返回错误: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountAPIKeys = %d, want 1", count)
	}
	row, err := db.GetAPIKeyByValue(ctx, key)
	if err != nil {
		t.Fatalf("GetAPIKeyByValue 返回错误: %v", err)
	}
	if row.ID != id || row.Name != "lookup" || row.Key != key {
		t.Fatalf("API key row = %#v, want id=%d name=lookup key=%s", row, id, key)
	}
}

func TestSQLiteAPIKeyQuotaAndExpiration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	key := "sk-test-limited-1234567890"
	expiresAt := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	id, err := db.InsertAPIKeyWithOptions(ctx, APIKeyInput{
		Name:       "limited",
		Key:        key,
		QuotaLimit: 0.01,
		ExpiresAt:  sql.NullTime{Time: expiresAt, Valid: true},
	})
	if err != nil {
		t.Fatalf("InsertAPIKeyWithOptions 返回错误: %v", err)
	}

	row, err := db.GetAPIKeyByValue(ctx, key)
	if err != nil {
		t.Fatalf("GetAPIKeyByValue 返回错误: %v", err)
	}
	if row.ID != id || row.QuotaLimit != 0.01 || !row.ExpiresAt.Valid {
		t.Fatalf("API key row = %#v, want quota and expiration", row)
	}
	if !row.ExpiresAt.Time.Equal(expiresAt) {
		t.Fatalf("ExpiresAt = %s, want %s", row.ExpiresAt.Time, expiresAt)
	}

	if err := db.InsertUsageLog(ctx, &UsageLogInput{
		APIKeyID:     id,
		Endpoint:     "/v1/responses",
		Model:        "gpt-5.4",
		StatusCode:   200,
		InputTokens:  1000,
		OutputTokens: 0,
	}); err != nil {
		t.Fatalf("InsertUsageLog 返回错误: %v", err)
	}
	db.flushLogs()

	row, err = db.GetAPIKeyByValue(ctx, key)
	if err != nil {
		t.Fatalf("GetAPIKeyByValue after usage 返回错误: %v", err)
	}
	if row.QuotaUsed != 0.0025 {
		t.Fatalf("QuotaUsed = %.12f, want %.12f", row.QuotaUsed, 0.0025)
	}
}

func TestSQLiteUpdateAPIKeyPatchesSelectedFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	key := "sk-test-patch-1234567890"
	expiresAt := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	id, err := db.InsertAPIKeyWithOptions(ctx, APIKeyInput{
		Name:            "patch",
		Key:             key,
		QuotaLimit:      1,
		ExpiresAt:       sql.NullTime{Time: expiresAt, Valid: true},
		AllowedGroupIDs: []int64{1, 2},
	})
	if err != nil {
		t.Fatalf("InsertAPIKeyWithOptions 返回错误: %v", err)
	}

	if err := db.UpdateAPIKey(ctx, id, APIKeyUpdate{Name: "patched", NameSet: true}); err != nil {
		t.Fatalf("UpdateAPIKey name 返回错误: %v", err)
	}
	row, err := db.GetAPIKeyByValue(ctx, key)
	if err != nil {
		t.Fatalf("GetAPIKeyByValue 返回错误: %v", err)
	}
	if row.Name != "patched" || row.QuotaLimit != 1 || !row.ExpiresAt.Valid || len(row.AllowedGroupIDs) != 2 {
		t.Fatalf("row = %#v, want only name patched", row)
	}

	if err := db.UpdateAPIKey(ctx, id, APIKeyUpdate{
		QuotaLimitSet:      true,
		QuotaLimit:         0,
		ExpiresAtSet:       true,
		ExpiresAt:          sql.NullTime{},
		AllowedGroupIDsSet: true,
		AllowedGroupIDs:    []int64{3},
	}); err != nil {
		t.Fatalf("UpdateAPIKey limits 返回错误: %v", err)
	}
	row, err = db.GetAPIKeyByValue(ctx, key)
	if err != nil {
		t.Fatalf("GetAPIKeyByValue after patch 返回错误: %v", err)
	}
	if row.Name != "patched" || row.QuotaLimit != 0 || row.ExpiresAt.Valid || len(row.AllowedGroupIDs) != 1 || row.AllowedGroupIDs[0] != 3 {
		t.Fatalf("row = %#v, want limits/groups patched", row)
	}
}

func TestSQLiteMigratesLegacyAPIKeysColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
	}
	if _, err := raw.Exec(`CREATE TABLE api_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		key TEXT UNIQUE NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("create legacy api_keys: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO api_keys (name, key) VALUES ('legacy', 'sk-legacy-1234567890')`); err != nil {
		t.Fatalf("insert legacy api key: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close legacy sqlite: %v", err)
	}

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite legacy) 返回错误: %v", err)
	}
	defer db.Close()

	row, err := db.GetAPIKeyByValue(context.Background(), "sk-legacy-1234567890")
	if err != nil {
		t.Fatalf("GetAPIKeyByValue legacy 返回错误: %v", err)
	}
	if row.Name != "legacy" || row.QuotaLimit != 0 || row.QuotaUsed != 0 || row.ExpiresAt.Valid || len(row.AllowedGroupIDs) != 0 {
		t.Fatalf("legacy row = %#v, want migrated defaults", row)
	}
}

func TestSQLiteAccountsEnabledDefaultsAndCanToggle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	id, err := db.InsertAccount(ctx, "test", "rt", "")
	if err != nil {
		t.Fatalf("InsertAccount 返回错误: %v", err)
	}

	rows, err := db.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive 返回错误: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListActive 返回 %d 条，want 1", len(rows))
	}
	if !rows[0].Enabled {
		t.Fatal("new account Enabled = false, want true")
	}

	if err := db.SetAccountEnabled(ctx, id, false); err != nil {
		t.Fatalf("SetAccountEnabled 返回错误: %v", err)
	}
	rows, err = db.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive 返回错误: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListActive 返回 %d 条，want 1", len(rows))
	}
	if rows[0].Enabled {
		t.Fatal("disabled account Enabled = true, want false")
	}

	if err := db.SetAccountEnabled(ctx, id+1, false); err != sql.ErrNoRows {
		t.Fatalf("SetAccountEnabled missing account error = %v, want sql.ErrNoRows", err)
	}
}

func TestSQLiteUsageLogsHasAPIKeyColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	columns, err := db.sqliteTableColumns(context.Background(), "usage_logs")
	if err != nil {
		t.Fatalf("sqliteTableColumns 返回错误: %v", err)
	}

	for _, name := range []string{"api_key_id", "api_key_name", "api_key_masked", "image_count", "image_width", "image_height", "image_bytes", "image_format", "image_size", "effective_model", "account_billed", "user_billed", "is_retry_attempt", "attempt_index", "upstream_error_kind", "error_message"} {
		if _, ok := columns[name]; !ok {
			t.Fatalf("usage_logs 缺少列 %q", name)
		}
	}
}

func TestSystemSettingsPersistsMaintenanceFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New sqlite 返回错误: %v", err)
	}
	defer db.Close()

	settings := &SystemSettings{
		SiteName:                         DefaultSiteName,
		MaxConcurrency:                   2,
		TestModel:                        "gpt-5.4",
		TestConcurrency:                  50,
		PgMaxConns:                       50,
		RedisPoolSize:                    30,
		ClientCompatMode:                 "preserve",
		CodexMinCLIVersion:               "0.118.0",
		UsageLogMode:                     UsageLogModeFull,
		UsageLogBatchSize:                200,
		UsageLogFlushIntervalSeconds:     5,
		StreamFlushPolicy:                "immediate",
		StreamFlushIntervalMS:            20,
		IPQPSLimit:                       3,
		IPRPMLimit:                       7,
		FilterLocalFallbackResponse:      true,
		DisableFastServiceTier:           true,
		ImageGenerationToolMode:          "force_on",
		DownstreamUsageMultiplier:        3.5,
		ProtocolMessageUsageBlastEnabled: true,
		APIMaintenanceConfig:             `{"enabled":true,"message":"维护中"}`,
	}

	if err := db.UpdateSystemSettings(context.Background(), settings); err != nil {
		t.Fatalf("UpdateSystemSettings 返回错误: %v", err)
	}

	got, err := db.GetSystemSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSystemSettings 返回错误: %v", err)
	}
	if got == nil {
		t.Fatal("settings is nil")
	}
	if !got.FilterLocalFallbackResponse {
		t.Fatal("FilterLocalFallbackResponse = false, want true")
	}
	if !got.DisableFastServiceTier {
		t.Fatal("DisableFastServiceTier = false, want true")
	}
	if got.ImageGenerationToolMode != "force_on" {
		t.Fatalf("ImageGenerationToolMode = %q, want force_on", got.ImageGenerationToolMode)
	}
	if got.DownstreamUsageMultiplier != 3.5 {
		t.Fatalf("DownstreamUsageMultiplier = %f, want 3.5", got.DownstreamUsageMultiplier)
	}
	if !got.ProtocolMessageUsageBlastEnabled {
		t.Fatal("ProtocolMessageUsageBlastEnabled = false, want true")
	}
	if got.APIMaintenanceConfig != `{"enabled":true,"message":"维护中"}` {
		t.Fatalf("APIMaintenanceConfig = %q", got.APIMaintenanceConfig)
	}
	if got.IPQPSLimit != 3 {
		t.Fatalf("IPQPSLimit = %d, want 3", got.IPQPSLimit)
	}
	if got.IPRPMLimit != 7 {
		t.Fatalf("IPRPMLimit = %d, want 7", got.IPRPMLimit)
	}
}

func TestSQLiteMigratesLegacyIPLimitToIPQPSLimit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy sqlite 返回错误: %v", err)
	}
	_, err = raw.Exec(`
		CREATE TABLE system_settings (
			id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			site_name TEXT DEFAULT 'CodexProxy',
			site_logo TEXT DEFAULT '',
			max_concurrency INTEGER DEFAULT 2,
			global_rpm INTEGER DEFAULT 0,
			ip_concurrency_limit INTEGER DEFAULT 6,
			ip_rpm_limit INTEGER DEFAULT 0,
			test_model TEXT DEFAULT 'gpt-5.4',
			test_concurrency INTEGER DEFAULT 50,
			proxy_url TEXT DEFAULT '',
			pg_max_conns INTEGER DEFAULT 50,
			redis_pool_size INTEGER DEFAULT 30,
			auto_clean_unauthorized INTEGER DEFAULT 0,
			auto_clean_rate_limited INTEGER DEFAULT 0
		);
		INSERT INTO system_settings (id, ip_concurrency_limit, ip_rpm_limit) VALUES (1, 6, 9);
	`)
	if closeErr := raw.Close(); closeErr != nil {
		t.Fatalf("close legacy sqlite 返回错误: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("seed legacy sqlite 返回错误: %v", err)
	}

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	got, err := db.GetSystemSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSystemSettings 返回错误: %v", err)
	}
	if got.IPQPSLimit != 6 {
		t.Fatalf("IPQPSLimit = %d, want migrated value 6", got.IPQPSLimit)
	}
	var migrated int
	if err := db.conn.QueryRowContext(context.Background(), `SELECT ip_qps_limit FROM system_settings WHERE id = 1`).Scan(&migrated); err != nil {
		t.Fatalf("query migrated ip_qps_limit 返回错误: %v", err)
	}
	if migrated != 6 {
		t.Fatalf("ip_qps_limit = %d, want migrated value 6", migrated)
	}
	columns, err := db.sqliteTableColumns(context.Background(), "system_settings")
	if err != nil {
		t.Fatalf("sqliteTableColumns 返回错误: %v", err)
	}
	if _, ok := columns["ip_concurrency_limit"]; ok {
		t.Fatal("legacy ip_concurrency_limit column still exists after migration")
	}
}

func TestSQLiteMigratesLegacyIPBlacklistToIPBansAndDropsColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy sqlite 返回错误: %v", err)
	}
	_, err = raw.Exec(`
		CREATE TABLE system_settings (
			id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			site_name TEXT DEFAULT 'CodexProxy',
			max_concurrency INTEGER DEFAULT 2,
			global_rpm INTEGER DEFAULT 0,
			ip_qps_limit INTEGER DEFAULT 0,
			ip_rpm_limit INTEGER DEFAULT 0,
			ip_blacklist TEXT DEFAULT '',
			test_model TEXT DEFAULT 'gpt-5.4',
			test_concurrency INTEGER DEFAULT 50
		);
		INSERT INTO system_settings (id, ip_blacklist) VALUES (1, '203.0.113.20
198.51.100.0/24');
	`)
	if closeErr := raw.Close(); closeErr != nil {
		t.Fatalf("close legacy sqlite 返回错误: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("seed legacy sqlite 返回错误: %v", err)
	}

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	bans, err := db.ListIPBans(context.Background(), true)
	if err != nil {
		t.Fatalf("ListIPBans 返回错误: %v", err)
	}
	if len(bans) != 1 {
		t.Fatalf("len(bans) = %d, want 1: %#v", len(bans), bans)
	}
	if bans[0].IP != "203.0.113.20" {
		t.Fatalf("migrated bans = %#v", bans)
	}
	for _, ban := range bans {
		if ban.Source != "manual" || ban.Reason != "manual" || !ban.Enabled {
			t.Fatalf("ban metadata = %#v, want manual enabled", ban)
		}
	}
	columns, err := db.sqliteTableColumns(context.Background(), "system_settings")
	if err != nil {
		t.Fatalf("sqliteTableColumns 返回错误: %v", err)
	}
	if _, ok := columns["ip_blacklist"]; ok {
		t.Fatal("legacy ip_blacklist column still exists after migration")
	}
}

func TestSQLiteDropsLegacyAPIKeyDisabledMessageColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy sqlite 返回错误: %v", err)
	}
	_, err = raw.Exec(`
		CREATE TABLE system_settings (
			id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			api_key_disabled_message TEXT DEFAULT ''
		);
		INSERT INTO system_settings (id, api_key_disabled_message) VALUES (1, '旧提示');
	`)
	if closeErr := raw.Close(); closeErr != nil {
		t.Fatalf("close legacy sqlite 返回错误: %v", closeErr)
	}
	if err != nil {
		t.Fatalf("seed legacy sqlite 返回错误: %v", err)
	}

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	columns, err := db.sqliteTableColumns(context.Background(), "system_settings")
	if err != nil {
		t.Fatalf("sqliteTableColumns 返回错误: %v", err)
	}
	if _, ok := columns["api_key_disabled_message"]; ok {
		t.Fatal("legacy api_key_disabled_message column still exists after migration")
	}
}

func TestCreateIPBanRejectsCIDR(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	if _, err := db.CreateIPBan(context.Background(), IPBanInput{IP: "198.51.100.0/24", Reason: IPBanReasonManual, Source: IPBanSourceManual, Enabled: true}); err == nil {
		t.Fatal("CreateIPBan(CIDR) error = nil, want error")
	}

	bans, err := db.ListIPBans(context.Background(), true)
	if err != nil {
		t.Fatalf("ListIPBans 返回错误: %v", err)
	}
	if len(bans) != 0 {
		t.Fatalf("len(bans) = %d, want 0: %#v", len(bans), bans)
	}
}

func TestCreateIPBanUpsertsAndIncrementsHitCount(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	ip := "203.0.113.88"
	if _, err := db.CreateIPBan(ctx, IPBanInput{
		IP:      ip,
		Reason:  IPBanReasonManual,
		Source:  IPBanSourceManual,
		Enabled: true,
	}); err != nil {
		t.Fatalf("CreateIPBan manual 返回错误: %v", err)
	}
	if _, err := db.RecordAutoIPBan(ctx, ip, IPBanReasonQPS, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("RecordAutoIPBan qps 返回错误: %v", err)
	}
	row, err := db.RecordAutoIPBan(ctx, ip, IPBanReasonRPM, time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatalf("RecordAutoIPBan rpm 返回错误: %v", err)
	}

	if row.HitCount != 3 {
		t.Fatalf("hit_count = %d, want 3", row.HitCount)
	}
	if row.Reason != IPBanReasonRPM || row.Source != IPBanSourceAuto {
		t.Fatalf("reason/source = %s/%s, want rpm_limit/auto", row.Reason, row.Source)
	}
	if row.UnbannedAt.Valid {
		t.Fatalf("unbanned_at valid = true, want false")
	}
	if !row.ExpiresAt.Valid || !row.LastTriggeredAt.Valid || !row.Enabled {
		t.Fatalf("row state = %#v, want enabled with expires_at and last_triggered_at", row)
	}

	bans, err := db.ListIPBans(ctx, true)
	if err != nil {
		t.Fatalf("ListIPBans 返回错误: %v", err)
	}
	if len(bans) != 1 || bans[0].IP != ip {
		t.Fatalf("bans = %#v, want single upserted row", bans)
	}
}

func TestUsageLogModeErrorsSkipsSuccessfulLogs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()
	db.SetUsageLogConfig(UsageLogModeErrors, 10, 5)

	ctx := context.Background()
	if err := db.InsertUsageLog(ctx, &UsageLogInput{
		AccountID:  1,
		Endpoint:   "/v1/responses",
		Model:      "gpt-5.4",
		StatusCode: 200,
	}); err != nil {
		t.Fatalf("InsertUsageLog success 返回错误: %v", err)
	}
	if err := db.InsertUsageLog(ctx, &UsageLogInput{
		AccountID:    1,
		Endpoint:     "/v1/responses",
		Model:        "gpt-5.4",
		StatusCode:   500,
		ErrorMessage: "upstream failed",
	}); err != nil {
		t.Fatalf("InsertUsageLog error 返回错误: %v", err)
	}
	db.flushLogs()

	logs, err := db.ListRecentUsageLogs(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentUsageLogs 返回错误: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}
	if logs[0].StatusCode != 500 {
		t.Fatalf("StatusCode = %d, want 500", logs[0].StatusCode)
	}
}

func TestUsageErrorSummaryAndFilters(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	for _, usageLog := range []*UsageLogInput{
		{
			AccountID:         1,
			Endpoint:          "/v1/responses",
			InboundEndpoint:   "/v1/responses",
			UpstreamEndpoint:  "/backend-api/codex/responses",
			Model:             "gpt-5.4",
			StatusCode:        500,
			DurationMs:        1200,
			IsRetryAttempt:    true,
			AttemptIndex:      1,
			UpstreamErrorKind: "upstream_timeout",
			ErrorMessage:      "upstream timeout",
		},
		{
			AccountID:         2,
			Endpoint:          "/v1/messages",
			InboundEndpoint:   "/v1/messages",
			Model:             "claude-sonnet-4.5",
			StatusCode:        401,
			DurationMs:        80,
			UpstreamErrorKind: "unauthorized",
			ErrorMessage:      "invalid access token",
		},
		{
			AccountID:    3,
			Endpoint:     "/v1/responses",
			Model:        "gpt-5.4",
			StatusCode:   499,
			DurationMs:   30,
			ErrorMessage: "client canceled",
		},
		{
			AccountID:  4,
			Endpoint:   "/v1/responses",
			Model:      "gpt-5.4",
			StatusCode: 200,
			DurationMs: 90,
		},
	} {
		if err := db.InsertUsageLog(ctx, usageLog); err != nil {
			t.Fatalf("InsertUsageLog 返回错误: %v", err)
		}
	}
	db.flushLogs()

	now := time.Now()
	filter := UsageLogFilter{
		Start:           now.Add(-1 * time.Hour),
		End:             now.Add(1 * time.Hour),
		Page:            1,
		PageSize:        10,
		ErrorOnly:       true,
		IncludeCanceled: true,
	}
	page, err := db.ListUsageLogsByTimeRangePaged(ctx, filter)
	if err != nil {
		t.Fatalf("ListUsageLogsByTimeRangePaged 返回错误: %v", err)
	}
	if page.Total != 3 {
		t.Fatalf("page.Total = %d, want 3", page.Total)
	}

	foundRetry := false
	for _, usageLog := range page.Logs {
		if usageLog.UpstreamErrorKind == "upstream_timeout" {
			foundRetry = true
			if !usageLog.IsRetryAttempt {
				t.Fatal("IsRetryAttempt = false, want true")
			}
			if usageLog.AttemptIndex != 1 {
				t.Fatalf("AttemptIndex = %d, want 1", usageLog.AttemptIndex)
			}
		}
	}
	if !foundRetry {
		t.Fatal("未找到 upstream_timeout 错误日志")
	}

	summary, err := db.GetUsageErrorSummary(ctx, filter)
	if err != nil {
		t.Fatalf("GetUsageErrorSummary 返回错误: %v", err)
	}
	if summary.TotalErrors != 3 {
		t.Fatalf("TotalErrors = %d, want 3", summary.TotalErrors)
	}
	if summary.Status5xx != 1 || summary.Unauthorized != 1 || summary.Canceled != 1 || summary.Timeouts != 1 || summary.RetryAttempts != 1 {
		t.Fatalf("summary = %+v, want one 5xx/401/499/timeout/retry", summary)
	}

	charts, err := db.GetChartAggregation(ctx, filter.Start, filter.End, 5)
	if err != nil {
		t.Fatalf("GetChartAggregation 返回错误: %v", err)
	}
	var chart4xx, chart5xx int64
	for _, point := range charts.Timeline {
		chart4xx += point.Errors4xx
		chart5xx += point.Errors5xx
	}
	if chart4xx != 1 || chart5xx != 1 {
		t.Fatalf("chart errors = 4xx:%d 5xx:%d, want 1/1", chart4xx, chart5xx)
	}

	filter.StatusFamily = "5xx"
	page, err = db.ListUsageLogsByTimeRangePaged(ctx, filter)
	if err != nil {
		t.Fatalf("ListUsageLogsByTimeRangePaged status family 返回错误: %v", err)
	}
	if page.Total != 1 || len(page.Logs) != 1 || page.Logs[0].StatusCode != 500 {
		t.Fatalf("5xx page = total %d len %d first %+v", page.Total, len(page.Logs), page.Logs)
	}
}

func TestGetChartAggregationReturnsUTCBuckets(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.conn.ExecContext(ctx, `
		INSERT INTO usage_logs (
			endpoint, model, status_code, duration_ms,
			input_tokens, output_tokens, reasoning_tokens, cached_tokens, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, "/v1/responses", "gpt-5.4", 200, 100, 1000, 20, 5, 300, "2026-05-22 13:08:00")
	if err != nil {
		t.Fatalf("insert usage log: %v", err)
	}

	beijing := time.FixedZone("CST", 8*60*60)
	start := time.Date(2026, 5, 22, 21, 0, 0, 0, beijing)
	end := time.Date(2026, 5, 22, 21, 30, 0, 0, beijing)
	charts, err := db.GetChartAggregation(ctx, start, end, 30)
	if err != nil {
		t.Fatalf("GetChartAggregation 返回错误: %v", err)
	}
	if len(charts.Timeline) != 1 {
		t.Fatalf("timeline len = %d, want 1", len(charts.Timeline))
	}
	if got, want := charts.Timeline[0].Bucket, "2026-05-22T13:00:00Z"; got != want {
		t.Fatalf("bucket = %q, want %q", got, want)
	}
}

func TestUsageLogModeOffSkipsAllLogs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()
	db.SetUsageLogConfig(UsageLogModeOff, 10, 5)

	ctx := context.Background()
	if err := db.InsertUsageLog(ctx, &UsageLogInput{
		AccountID:  1,
		Endpoint:   "/v1/responses",
		Model:      "gpt-5.4",
		StatusCode: 500,
	}); err != nil {
		t.Fatalf("InsertUsageLog 返回错误: %v", err)
	}
	db.flushLogs()

	logs, err := db.ListRecentUsageLogs(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentUsageLogs 返回错误: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("len(logs) = %d, want 0", len(logs))
	}
}

func TestSQLiteModelCooldownPersistence(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	resetAt := time.Now().Add(15 * time.Minute).Truncate(time.Second)
	if err := db.SetModelCooldown(ctx, 42, "gpt-5.4", "model_capacity", resetAt); err != nil {
		t.Fatalf("SetModelCooldown 返回错误: %v", err)
	}

	rows, err := db.ListActiveModelCooldowns(ctx)
	if err != nil {
		t.Fatalf("ListActiveModelCooldowns 返回错误: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListActiveModelCooldowns 返回 %d 条，want 1", len(rows))
	}
	if rows[0].AccountID != 42 || rows[0].Model != "gpt-5.4" || rows[0].Reason != "model_capacity" {
		t.Fatalf("cooldown row = %#v", rows[0])
	}

	if err := db.ClearModelCooldown(ctx, 42, "gpt-5.4"); err != nil {
		t.Fatalf("ClearModelCooldown 返回错误: %v", err)
	}
	rows, err = db.ListActiveModelCooldowns(ctx)
	if err != nil {
		t.Fatalf("ListActiveModelCooldowns 返回错误: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("ListActiveModelCooldowns 返回 %d 条，want 0", len(rows))
	}
}

func TestAccountRequestCountsSeparateRetryAttempts(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	logs := []*UsageLogInput{
		{AccountID: 7, Endpoint: "/v1/responses", Model: "gpt-5.4", StatusCode: 200},
		{AccountID: 7, Endpoint: "/v1/responses", Model: "gpt-5.4", StatusCode: 429, IsRetryAttempt: true, AttemptIndex: 1, UpstreamErrorKind: "model_capacity"},
		{AccountID: 7, Endpoint: "/v1/responses", Model: "gpt-5.4", StatusCode: 500, IsRetryAttempt: false, AttemptIndex: 2, UpstreamErrorKind: "server"},
	}
	for _, usageLog := range logs {
		if err := db.InsertUsageLog(ctx, usageLog); err != nil {
			t.Fatalf("InsertUsageLog 返回错误: %v", err)
		}
	}
	db.flushLogs()

	counts, err := db.GetAccountRequestCounts(ctx)
	if err != nil {
		t.Fatalf("GetAccountRequestCounts 返回错误: %v", err)
	}
	got := counts[7]
	if got == nil {
		t.Fatal("account 7 counts missing")
	}
	if got.SuccessCount != 1 || got.ErrorCount != 1 || got.RetryErrorCount != 1 || got.RateLimitAttemptCount != 1 {
		t.Fatalf("counts = %#v, want success=1 error=1 retry=1 rateLimit=1", got)
	}
}

func TestSQLiteUsageStatsBaselineHasBillingColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	columns, err := db.sqliteTableColumns(context.Background(), "usage_stats_baseline")
	if err != nil {
		t.Fatalf("sqliteTableColumns 返回错误: %v", err)
	}

	for _, name := range []string{"account_billed", "user_billed", "cache_hit_requests", "first_token_ms_sum", "first_token_samples"} {
		if _, ok := columns[name]; !ok {
			t.Fatalf("usage_stats_baseline 缺少列 %q", name)
		}
	}
}

func TestDeleteAccountGroupDoesNotBroadenScopedAPIKey(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	groupA, err := db.CreateAccountGroup(ctx, "Group A", "", "#2563eb", 0)
	if err != nil {
		t.Fatalf("CreateAccountGroup A 返回错误: %v", err)
	}
	groupB, err := db.CreateAccountGroup(ctx, "Group B", "", "#16a34a", 1)
	if err != nil {
		t.Fatalf("CreateAccountGroup B 返回错误: %v", err)
	}

	keyOnlyA, err := db.InsertAPIKeyWithOptions(ctx, APIKeyInput{
		Name:            "Only A",
		Key:             "sk-only-a-1234567890",
		AllowedGroupIDs: []int64{groupA},
	})
	if err != nil {
		t.Fatalf("InsertAPIKeyWithOptions only-a 返回错误: %v", err)
	}
	keyAB, err := db.InsertAPIKeyWithOptions(ctx, APIKeyInput{
		Name:            "A and B",
		Key:             "sk-a-b-1234567890",
		AllowedGroupIDs: []int64{groupA, groupB},
	})
	if err != nil {
		t.Fatalf("InsertAPIKeyWithOptions a-b 返回错误: %v", err)
	}

	if err := db.DeleteAccountGroup(ctx, groupA, true); err != nil {
		t.Fatalf("DeleteAccountGroup 返回错误: %v", err)
	}

	rows, err := db.ListAPIKeys(ctx)
	if err != nil {
		t.Fatalf("ListAPIKeys 返回错误: %v", err)
	}

	got := make(map[int64][]int64)
	for _, row := range rows {
		got[row.ID] = row.AllowedGroupIDs
	}

	if actual := got[keyOnlyA]; len(actual) != 1 || actual[0] != groupA {
		t.Fatalf("keyOnlyA allowed groups = %v, want stale [%d] to preserve deny-all semantics", actual, groupA)
	}
	if actual := got[keyAB]; len(actual) != 1 || actual[0] != groupB {
		t.Fatalf("keyAB allowed groups = %v, want [%d]", actual, groupB)
	}
}

func TestUsageLogsPersistEffectiveModel(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.InsertUsageLog(ctx, &UsageLogInput{
		AccountID:        1,
		Endpoint:         "/v1/messages",
		InboundEndpoint:  "/v1/messages",
		UpstreamEndpoint: "/v1/responses",
		Model:            "claude-haiku-4-5-20251001",
		EffectiveModel:   "gpt-5.4",
		StatusCode:       200,
		ReasoningEffort:  "high",
	}); err != nil {
		t.Fatalf("InsertUsageLog 返回错误: %v", err)
	}
	db.flushLogs()

	logs, err := db.ListRecentUsageLogs(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentUsageLogs 返回错误: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}
	if logs[0].Model != "claude-haiku-4-5-20251001" {
		t.Fatalf("Model = %q, want claude-haiku-4-5-20251001", logs[0].Model)
	}
	if logs[0].EffectiveModel != "gpt-5.4" {
		t.Fatalf("EffectiveModel = %q, want gpt-5.4", logs[0].EffectiveModel)
	}
	if logs[0].ReasoningEffort != "high" {
		t.Fatalf("ReasoningEffort = %q, want high", logs[0].ReasoningEffort)
	}
}

func TestUsageLogsReturnClientIP(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.InsertUsageLog(ctx, &UsageLogInput{
		AccountID:        1,
		Endpoint:         "/v1/responses",
		InboundEndpoint:  "/v1/responses",
		UpstreamEndpoint: "/v1/responses",
		Model:            "gpt-5.4",
		StatusCode:       200,
		ClientIP:         "203.0.113.24",
	}); err != nil {
		t.Fatalf("InsertUsageLog 返回错误: %v", err)
	}
	db.flushLogs()

	recentLogs, err := db.ListRecentUsageLogs(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentUsageLogs 返回错误: %v", err)
	}
	if len(recentLogs) != 1 {
		t.Fatalf("len(recentLogs) = %d, want 1", len(recentLogs))
	}
	if recentLogs[0].ClientIP != "203.0.113.24" {
		t.Fatalf("recent ClientIP = %q, want 203.0.113.24", recentLogs[0].ClientIP)
	}

	page, err := db.ListUsageLogsByTimeRangePaged(ctx, UsageLogFilter{
		Start:    time.Now().Add(-1 * time.Hour),
		End:      time.Now().Add(1 * time.Hour),
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListUsageLogsByTimeRangePaged 返回错误: %v", err)
	}
	if len(page.Logs) != 1 {
		t.Fatalf("len(page.Logs) = %d, want 1", len(page.Logs))
	}
	if page.Logs[0].ClientIP != "203.0.113.24" {
		t.Fatalf("paged ClientIP = %q, want 203.0.113.24", page.Logs[0].ClientIP)
	}
}

func TestUsageLogsPersistImageMetadata(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.InsertUsageLog(ctx, &UsageLogInput{
		AccountID:        1,
		Endpoint:         "/v1/images/generations",
		InboundEndpoint:  "/v1/images/generations",
		UpstreamEndpoint: "/v1/responses",
		Model:            "gpt-image-2-4k",
		StatusCode:       200,
		DurationMs:       1200,
		ImageCount:       1,
		ImageWidth:       3840,
		ImageHeight:      2160,
		ImageBytes:       2457600,
		ImageFormat:      "png",
		ImageSize:        "3840x2160",
	}); err != nil {
		t.Fatalf("InsertUsageLog 返回错误: %v", err)
	}
	db.flushLogs()

	logs, err := db.ListRecentUsageLogs(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentUsageLogs 返回错误: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}
	got := logs[0]
	if got.ImageCount != 1 || got.ImageWidth != 3840 || got.ImageHeight != 2160 || got.ImageBytes != 2457600 || got.ImageFormat != "png" || got.ImageSize != "3840x2160" {
		t.Fatalf("image metadata = %#v", got)
	}
}

func TestUsageLogsReturnBillingFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.InsertUsageLog(ctx, &UsageLogInput{
		AccountID:        1,
		Endpoint:         "/v1/responses",
		InboundEndpoint:  "/v1/responses",
		UpstreamEndpoint: "/v1/responses",
		Model:            "gpt-5.5",
		StatusCode:       200,
		InputTokens:      476,
		OutputTokens:     252,
		TotalTokens:      728,
		ServiceTier:      "default",
	}); err != nil {
		t.Fatalf("InsertUsageLog 返回错误: %v", err)
	}
	db.flushLogs()

	logs, err := db.ListRecentUsageLogs(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentUsageLogs 返回错误: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}

	got := logs[0]
	want := calculateCost(476, 252, 0, "gpt-5.5", "default")
	if got.AccountBilled != want || got.UserBilled != want {
		t.Fatalf("billing = account %.12f user %.12f, want %.12f", got.AccountBilled, got.UserBilled, want)
	}
	if got.InputCost <= 0 || got.OutputCost <= 0 || got.TotalCost != want {
		t.Fatalf("billing breakdown = input %.12f output %.12f total %.12f, want total %.12f", got.InputCost, got.OutputCost, got.TotalCost, want)
	}
}

func TestUsageLogsBillFastByActualServiceTier(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.InsertUsageLog(ctx, &UsageLogInput{
		AccountID:          1,
		Endpoint:           "/v1/responses",
		Model:              "gpt-5.4",
		StatusCode:         200,
		InputTokens:        1000,
		OutputTokens:       500,
		CachedTokens:       200,
		ServiceTier:        "fast",
		BillingServiceTier: "default",
	}); err != nil {
		t.Fatalf("InsertUsageLog 返回错误: %v", err)
	}
	if err := db.InsertUsageLog(ctx, &UsageLogInput{
		AccountID:          1,
		Endpoint:           "/v1/responses",
		Model:              "gpt-5.4",
		StatusCode:         200,
		InputTokens:        1000,
		OutputTokens:       500,
		CachedTokens:       200,
		ServiceTier:        "fast",
		BillingServiceTier: "priority",
	}); err != nil {
		t.Fatalf("InsertUsageLog 返回错误: %v", err)
	}
	db.flushLogs()

	logs, err := db.ListRecentUsageLogs(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentUsageLogs 返回错误: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("len(logs) = %d, want 2", len(logs))
	}

	wantPriority := calculateCost(1000, 500, 200, "gpt-5.4", "priority")
	wantDefault := calculateCost(1000, 500, 200, "gpt-5.4", "default")
	seenPriority := false
	seenDefault := false
	for _, log := range logs {
		if log.ServiceTier != "fast" {
			t.Fatalf("log tier = %q, want fast", log.ServiceTier)
		}
		switch log.AccountBilled {
		case wantPriority:
			seenPriority = true
		case wantDefault:
			seenDefault = true
		default:
			t.Fatalf("unexpected billed amount %.12f, want %.12f or %.12f", log.AccountBilled, wantPriority, wantDefault)
		}
	}
	if !seenPriority || !seenDefault {
		t.Fatalf("billing tiers seen priority=%v default=%v, want both", seenPriority, seenDefault)
	}
}

func TestUsageLogsReturnErrorMessage(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.InsertUsageLog(ctx, &UsageLogInput{
		AccountID:    1,
		Endpoint:     "/v1/responses",
		Model:        "gpt-5.4",
		StatusCode:   429,
		ErrorMessage: "rate_limit_exceeded · Too many requests",
	}); err != nil {
		t.Fatalf("InsertUsageLog 返回错误: %v", err)
	}
	db.flushLogs()

	logs, err := db.ListRecentUsageLogs(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentUsageLogs 返回错误: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}
	if got := logs[0].ErrorMessage; got != "rate_limit_exceeded · Too many requests" {
		t.Fatalf("ErrorMessage = %q", got)
	}
}

func TestUsageStatsIncludeBillingTotals(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	for _, usageLog := range []*UsageLogInput{
		{
			AccountID:    1,
			Endpoint:     "/v1/responses",
			Model:        "gpt-5.5",
			StatusCode:   200,
			InputTokens:  1000,
			OutputTokens: 500,
			TotalTokens:  1500,
		},
		{
			AccountID:    1,
			Endpoint:     "/v1/responses",
			Model:        "gpt-5.5",
			StatusCode:   499,
			InputTokens:  1000,
			OutputTokens: 500,
			TotalTokens:  1500,
		},
	} {
		if err := db.InsertUsageLog(ctx, usageLog); err != nil {
			t.Fatalf("InsertUsageLog 返回错误: %v", err)
		}
	}
	db.flushLogs()

	stats, err := db.GetUsageStats(ctx, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("GetUsageStats 返回错误: %v", err)
	}

	want := calculateCost(1000, 500, 0, "gpt-5.5", "")
	if stats.TotalAccountBilled != want || stats.TotalUserBilled != want {
		t.Fatalf("total billing = account %.12f user %.12f, want %.12f", stats.TotalAccountBilled, stats.TotalUserBilled, want)
	}
	if stats.TodayAccountBilled != want || stats.TodayUserBilled != want {
		t.Fatalf("today billing = account %.12f user %.12f, want %.12f", stats.TodayAccountBilled, stats.TodayUserBilled, want)
	}
	if stats.AvgAccountBilled != want || stats.AvgUserBilled != want {
		t.Fatalf("avg billing = account %.12f user %.12f, want %.12f", stats.AvgAccountBilled, stats.AvgUserBilled, want)
	}
	if len(stats.ModelStats) != 1 {
		t.Fatalf("ModelStats len = %d, want 1: %+v", len(stats.ModelStats), stats.ModelStats)
	}
	modelStats := stats.ModelStats[0]
	if modelStats.Model != "gpt-5.5" || modelStats.Requests != 1 || modelStats.Tokens != 1500 {
		t.Fatalf("ModelStats[0] = %+v, want gpt-5.5 requests=1 tokens=1500", modelStats)
	}
	if modelStats.AccountBilled != want || modelStats.UserBilled != want {
		t.Fatalf("model billing = account %.12f user %.12f, want %.12f", modelStats.AccountBilled, modelStats.UserBilled, want)
	}
}

func TestUsageStatsIncludeCodex2APIBreakdowns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	logs := []*UsageLogInput{
		{
			AccountID:       1,
			Endpoint:        "/v1/responses",
			InboundEndpoint: "/v1/responses",
			Model:           "gpt-5.5",
			StatusCode:      200,
			InputTokens:     1000,
			OutputTokens:    500,
			TotalTokens:     1500,
			Stream:          true,
			ServiceTier:     "fast",
			CachedTokens:    128,
			FirstTokenMs:    820,
			ReasoningTokens: 32,
			APIKeyID:        7,
			APIKeyName:      "Claude Code",
			APIKeyMasked:    "sk-...1111",
		},
		{
			AccountID:       1,
			Endpoint:        "/v1/images/generations",
			InboundEndpoint: "/v1/images/generations",
			Model:           "gpt-image-2",
			StatusCode:      200,
			ImageCount:      1,
			APIKeyID:        7,
			APIKeyName:      "Claude Code",
			APIKeyMasked:    "sk-...1111",
		},
		{
			AccountID:      2,
			Endpoint:       "/v1/chat/completions",
			Model:          "gpt-5.4",
			StatusCode:     500,
			InputTokens:    100,
			OutputTokens:   20,
			TotalTokens:    120,
			APIKeyID:       8,
			APIKeyName:     "Cherry Studio",
			APIKeyMasked:   "sk-...2222",
			IsRetryAttempt: true,
			AttemptIndex:   1,
		},
		{
			AccountID:       3,
			Endpoint:        "/v1/responses",
			InboundEndpoint: "/v1/responses",
			Model:           "gpt-5.4",
			StatusCode:      499,
			Stream:          true,
			APIKeyID:        9,
			APIKeyName:      "Canceled",
		},
	}
	for _, usageLog := range logs {
		if err := db.InsertUsageLog(ctx, usageLog); err != nil {
			t.Fatalf("InsertUsageLog 返回错误: %v", err)
		}
	}
	db.flushLogs()

	stats, err := db.GetUsageStats(ctx, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("GetUsageStats 返回错误: %v", err)
	}
	if stats.TotalRequests != 3 {
		t.Fatalf("TotalRequests = %d, want 3", stats.TotalRequests)
	}
	if stats.TodayCachedTokens != 128 {
		t.Fatalf("TodayCachedTokens = %d, want 128", stats.TodayCachedTokens)
	}
	if stats.TodayCacheRate < 33.3 || stats.TodayCacheRate > 33.4 {
		t.Fatalf("TodayCacheRate = %.4f, want about 33.33", stats.TodayCacheRate)
	}
	if stats.TotalCacheRate < 33.3 || stats.TotalCacheRate > 33.4 {
		t.Fatalf("TotalCacheRate = %.4f, want about 33.33", stats.TotalCacheRate)
	}
	if stats.AvgFirstTokenMs != 820 {
		t.Fatalf("AvgFirstTokenMs = %.2f, want 820", stats.AvgFirstTokenMs)
	}
	features := stats.FeatureStats
	if features.StreamRequests != 1 || features.SyncRequests != 2 || features.FastRequests != 1 ||
		features.CacheHitRequests != 1 || features.ReasoningRequests != 1 || features.ImageRequests != 1 ||
		features.RetryRequests != 1 || features.ErrorRequests != 1 {
		t.Fatalf("FeatureStats = %+v, want stream/sync/fast/cache/reasoning/image/retry/error = 1/2/1/1/1/1/1/1", features)
	}

	endpoints := make(map[string]UsageEndpointStat)
	for _, item := range stats.EndpointStats {
		endpoints[item.Endpoint] = item
	}
	if endpoints["/v1/responses"].Requests != 1 || endpoints["/v1/images/generations"].Requests != 1 || endpoints["/v1/chat/completions"].ErrorCount != 1 {
		t.Fatalf("EndpointStats = %+v", stats.EndpointStats)
	}

	apiKeys := make(map[int64]UsageAPIKeyStat)
	for _, item := range stats.APIKeyStats {
		apiKeys[item.APIKeyID] = item
	}
	if apiKeys[7].Requests != 2 || apiKeys[7].Label != "Claude Code" {
		t.Fatalf("APIKeyStats[7] = %+v, want Claude Code requests=2", apiKeys[7])
	}
	if apiKeys[8].Requests != 1 || apiKeys[8].ErrorCount != 1 {
		t.Fatalf("APIKeyStats[8] = %+v, want requests=1 errors=1", apiKeys[8])
	}
}

func TestUsageStatsBaselinePreservesCacheRateAndFirstTokenAfterClear(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	for _, usageLog := range []*UsageLogInput{
		{
			AccountID:    1,
			Endpoint:     "/v1/responses",
			Model:        "gpt-5.5",
			StatusCode:   200,
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			CachedTokens: 32,
			FirstTokenMs: 600,
		},
		{
			AccountID:    1,
			Endpoint:     "/v1/responses",
			Model:        "gpt-5.5",
			StatusCode:   200,
			InputTokens:  80,
			OutputTokens: 20,
			TotalTokens:  100,
			FirstTokenMs: 300,
		},
	} {
		if err := db.InsertUsageLog(ctx, usageLog); err != nil {
			t.Fatalf("InsertUsageLog 返回错误: %v", err)
		}
	}
	db.flushLogs()

	if err := db.ClearUsageLogs(ctx); err != nil {
		t.Fatalf("ClearUsageLogs 返回错误: %v", err)
	}

	stats, err := db.GetUsageStats(ctx, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("GetUsageStats 返回错误: %v", err)
	}
	if stats.TotalRequests != 2 {
		t.Fatalf("TotalRequests = %d, want 2", stats.TotalRequests)
	}
	if stats.TotalCacheRate < 49.9 || stats.TotalCacheRate > 50.1 {
		t.Fatalf("TotalCacheRate = %.4f, want about 50.00", stats.TotalCacheRate)
	}
	if stats.AvgFirstTokenMs < 449.9 || stats.AvgFirstTokenMs > 450.1 {
		t.Fatalf("AvgFirstTokenMs = %.4f, want about 450.00", stats.AvgFirstTokenMs)
	}
}

func TestSoftDeleteAccountMarksDeletedStatus(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	id, err := db.InsertAccount(ctx, "delete-me", "rt-delete-me", "")
	if err != nil {
		t.Fatalf("InsertAccount 返回错误: %v", err)
	}
	if err := db.SoftDeleteAccount(ctx, id); err != nil {
		t.Fatalf("SoftDeleteAccount 返回错误: %v", err)
	}

	active, err := db.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive 返回错误: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("ListActive 返回 %d 条，want 0", len(active))
	}
	if _, err := db.GetAccountByID(ctx, id); err == nil {
		t.Fatal("GetAccountByID 应该排除已删除账号")
	}

	var status string
	var errorMessage string
	var deletedAt sql.NullString
	if err := db.conn.QueryRowContext(ctx, `SELECT status, error_message, deleted_at FROM accounts WHERE id = $1`, id).Scan(&status, &errorMessage, &deletedAt); err != nil {
		t.Fatalf("查询账号状态返回错误: %v", err)
	}
	if status != "deleted" {
		t.Fatalf("status = %q, want deleted", status)
	}
	if errorMessage != "" {
		t.Fatalf("error_message = %q, want empty", errorMessage)
	}
	if !deletedAt.Valid || deletedAt.String == "" {
		t.Fatal("deleted_at 未写入")
	}
}

func TestSQLiteMigratesLegacyDeletedAccounts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	ctx := context.Background()

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	id, err := db.InsertAccount(ctx, "legacy-delete", "rt-legacy-delete", "")
	if err != nil {
		t.Fatalf("InsertAccount 返回错误: %v", err)
	}
	if err := db.SetError(ctx, id, "deleted"); err != nil {
		t.Fatalf("SetError 返回错误: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close 返回错误: %v", err)
	}

	db, err = New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	var status string
	var errorMessage string
	var deletedAt sql.NullString
	if err := db.conn.QueryRowContext(ctx, `SELECT status, error_message, deleted_at FROM accounts WHERE id = $1`, id).Scan(&status, &errorMessage, &deletedAt); err != nil {
		t.Fatalf("查询迁移后账号返回错误: %v", err)
	}
	if status != "deleted" {
		t.Fatalf("status = %q, want deleted", status)
	}
	if errorMessage != "" {
		t.Fatalf("error_message = %q, want empty", errorMessage)
	}
	if !deletedAt.Valid || deletedAt.String == "" {
		t.Fatal("deleted_at 未迁移")
	}
}

func TestListActiveIncludesErrorAccounts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	id, err := db.InsertAccount(ctx, "error-account", "rt-error", "")
	if err != nil {
		t.Fatalf("InsertAccount 返回错误: %v", err)
	}
	if err := db.SetError(ctx, id, "batch test failed"); err != nil {
		t.Fatalf("SetError 返回错误: %v", err)
	}

	rows, err := db.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive 返回错误: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListActive 返回 %d 条，want 1", len(rows))
	}
	if rows[0].Status != "error" {
		t.Fatalf("status = %q, want error", rows[0].Status)
	}
	if rows[0].ErrorMessage != "batch test failed" {
		t.Fatalf("error_message = %q, want batch test failed", rows[0].ErrorMessage)
	}
}

func TestUsageLogsFilterByAPIKeyID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	targetAPIKeyID := int64(7)

	logs := []*UsageLogInput{
		{
			AccountID:    1,
			Endpoint:     "/v1/chat/completions",
			Model:        "gpt-5.4",
			StatusCode:   200,
			DurationMs:   120,
			APIKeyID:     targetAPIKeyID,
			APIKeyName:   "Team A",
			APIKeyMasked: "sk-a****...****1111",
		},
		{
			AccountID:    1,
			Endpoint:     "/v1/responses",
			Model:        "gpt-5.4",
			StatusCode:   200,
			DurationMs:   220,
			APIKeyID:     targetAPIKeyID,
			APIKeyName:   "Team A",
			APIKeyMasked: "sk-a****...****1111",
		},
		{
			AccountID:    2,
			Endpoint:     "/v1/responses",
			Model:        "gpt-5.4-mini",
			StatusCode:   200,
			DurationMs:   320,
			APIKeyID:     8,
			APIKeyName:   "Team B",
			APIKeyMasked: "sk-b****...****2222",
		},
	}

	for _, usageLog := range logs {
		if err := db.InsertUsageLog(ctx, usageLog); err != nil {
			t.Fatalf("InsertUsageLog 返回错误: %v", err)
		}
	}
	db.flushLogs()

	recentLogs, err := db.ListRecentUsageLogs(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentUsageLogs 返回错误: %v", err)
	}
	if len(recentLogs) != len(logs) {
		t.Fatalf("recentLogs 长度 = %d, want %d", len(recentLogs), len(logs))
	}

	foundSnapshot := false
	for _, usageLog := range recentLogs {
		if usageLog.APIKeyID == targetAPIKeyID {
			foundSnapshot = true
			if usageLog.APIKeyName != "Team A" {
				t.Fatalf("APIKeyName = %q, want %q", usageLog.APIKeyName, "Team A")
			}
			if usageLog.APIKeyMasked != "sk-a****...****1111" {
				t.Fatalf("APIKeyMasked = %q, want %q", usageLog.APIKeyMasked, "sk-a****...****1111")
			}
		}
	}
	if !foundSnapshot {
		t.Fatal("未找到带 API 密钥快照的最近日志")
	}

	page, err := db.ListUsageLogsByTimeRangePaged(ctx, UsageLogFilter{
		Start:    now.Add(-1 * time.Hour),
		End:      now.Add(1 * time.Hour),
		Page:     1,
		PageSize: 10,
		APIKeyID: &targetAPIKeyID,
	})
	if err != nil {
		t.Fatalf("ListUsageLogsByTimeRangePaged 返回错误: %v", err)
	}

	if page.Total != 2 {
		t.Fatalf("page.Total = %d, want %d", page.Total, 2)
	}
	if len(page.Logs) != 2 {
		t.Fatalf("len(page.Logs) = %d, want %d", len(page.Logs), 2)
	}
	for _, usageLog := range page.Logs {
		if usageLog.APIKeyID != targetAPIKeyID {
			t.Fatalf("APIKeyID = %d, want %d", usageLog.APIKeyID, targetAPIKeyID)
		}
		if usageLog.APIKeyName != "Team A" {
			t.Fatalf("APIKeyName = %q, want %q", usageLog.APIKeyName, "Team A")
		}
	}
}

func TestSQLiteGetIPUsageStatsAggregatesRecentTraffic(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now()
	rows := []struct {
		ip     string
		tokens int
		cost   float64
		status int
		at     time.Time
	}{
		{ip: "203.0.113.8", tokens: 100, cost: 0.01, status: 200, at: now.Add(-5 * time.Second)},
		{ip: "203.0.113.8", tokens: 200, cost: 0.02, status: 200, at: now.Add(-30 * time.Second)},
		{ip: "198.51.100.2", tokens: 50, cost: 0.005, status: 200, at: now.Add(-20 * time.Second)},
		{ip: "203.0.113.9", tokens: 300, cost: 0.03, status: 200, at: now.Add(-10 * time.Minute)},
		{ip: "", tokens: 20, cost: 0.002, status: 200, at: now.Add(-15 * time.Second)},
		{ip: "203.0.113.10", tokens: 20, cost: 0.002, status: 499, at: now.Add(-15 * time.Second)},
	}
	for _, row := range rows {
		_, err := db.conn.ExecContext(ctx, `
			INSERT INTO usage_logs (client_ip, total_tokens, user_billed, status_code, created_at)
			VALUES ($1, $2, $3, $4, $5)
		`, row.ip, row.tokens, row.cost, row.status, sqliteTimeParam(row.at))
		if err != nil {
			t.Fatalf("insert usage log 返回错误: %v", err)
		}
	}

	stats, err := db.GetIPUsageStats(ctx, 10, now.Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("GetIPUsageStats 返回错误: %v", err)
	}
	if len(stats) != 3 {
		t.Fatalf("len(stats) = %d, want 3 (%#v)", len(stats), stats)
	}
	if stats[0].IP != "203.0.113.8" {
		t.Fatalf("top IP = %q, want 203.0.113.8", stats[0].IP)
	}
	if stats[0].Requests != 2 {
		t.Fatalf("requests = %d, want 2", stats[0].Requests)
	}
	if stats[0].QPS <= 0 || stats[0].RPM != 2 || stats[0].TPM != 300 {
		t.Fatalf("top stats = %#v, want qps>0 rpm=2 tpm=300", stats[0])
	}
	if stats[0].Tokens != 300 {
		t.Fatalf("tokens = %d, want 300", stats[0].Tokens)
	}
	if diff := math.Abs(stats[0].Cost - 0.03); diff > 0.000001 {
		t.Fatalf("cost = %.6f, want 0.03", stats[0].Cost)
	}

	widerStats, err := db.GetIPUsageStats(ctx, 10, now.Add(-15*time.Minute))
	if err != nil {
		t.Fatalf("GetIPUsageStats wide 返回错误: %v", err)
	}
	if len(widerStats) != 4 {
		t.Fatalf("len(widerStats) = %d, want 4 (%#v)", len(widerStats), widerStats)
	}
	foundOldWindowIP := false
	for _, stat := range widerStats {
		if stat.IP == "203.0.113.9" {
			foundOldWindowIP = true
			if stat.Requests != 1 || stat.Tokens != 300 {
				t.Fatalf("old window stat = %#v, want requests=1 tokens=300", stat)
			}
			if stat.RPM != 0 || stat.TPM != 0 {
				t.Fatalf("old window rpm/tpm = %#v, want rpm=0 tpm=0", stat)
			}
		}
	}
	if !foundOldWindowIP {
		t.Fatal("wide window missing 203.0.113.9")
	}

	total, err := db.CountIPUsageStats(ctx)
	if err != nil {
		t.Fatalf("CountIPUsageStats 返回错误: %v", err)
	}
	if total != 4 {
		t.Fatalf("CountIPUsageStats = %d, want 4", total)
	}

	limited, err := db.GetIPUsageStats(ctx, 2, now.Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("GetIPUsageStats limited 返回错误: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("len(limited) = %d, want 2 (%#v)", len(limited), limited)
	}

	page, err := db.GetIPUsageStatsPage(ctx, IPUsageStatsQuery{
		Page:        1,
		PageSize:    2,
		WindowStart: now.Add(-5 * time.Minute),
		SortBy:      "tokens",
		SortOrder:   "asc",
	})
	if err != nil {
		t.Fatalf("GetIPUsageStatsPage 返回错误: %v", err)
	}
	if page.Total != 3 || page.Page != 1 || page.PageSize != 2 {
		t.Fatalf("page meta = total %d page %d size %d, want 3/1/2", page.Total, page.Page, page.PageSize)
	}
	if len(page.Stats) != 2 {
		t.Fatalf("len(page.Stats) = %d, want 2 (%#v)", len(page.Stats), page.Stats)
	}
	if page.Stats[0].IP != "203.0.113.10" || page.Stats[1].IP != "198.51.100.2" {
		t.Fatalf("token asc order = %#v, want 203.0.113.10 then 198.51.100.2", page.Stats)
	}

	secondPage, err := db.GetIPUsageStatsPage(ctx, IPUsageStatsQuery{
		Page:        2,
		PageSize:    2,
		WindowStart: now.Add(-5 * time.Minute),
		SortBy:      "tokens",
		SortOrder:   "asc",
	})
	if err != nil {
		t.Fatalf("GetIPUsageStatsPage second 返回错误: %v", err)
	}
	if len(secondPage.Stats) != 1 || secondPage.Stats[0].IP != "203.0.113.8" {
		t.Fatalf("second page = %#v, want only 203.0.113.8", secondPage.Stats)
	}
}

func TestSQLiteUsageLogsTimeRangeUsesUTCStorage(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "codex2api.db")

	db, err := New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("New(sqlite) 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	createdUTC := time.Date(2026, 4, 23, 20, 6, 0, 0, time.UTC)
	if _, err := db.conn.ExecContext(ctx, `
		INSERT INTO usage_logs (
			account_id, endpoint, inbound_endpoint, upstream_endpoint, model,
			status_code, total_tokens, input_tokens, output_tokens, created_at
		)
		VALUES (1, '/v1/images/generations', '/v1/images/generations', '/v1/responses', 'gpt-image-2',
			200, 1790, 34, 1756, $1)
	`, sqliteTimeParam(createdUTC)); err != nil {
		t.Fatalf("insert usage log 返回错误: %v", err)
	}

	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	localCreated := createdUTC.In(shanghai)
	page, err := db.ListUsageLogsByTimeRangePaged(ctx, UsageLogFilter{
		Start:    localCreated.Add(-1 * time.Hour),
		End:      localCreated.Add(1 * time.Hour),
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListUsageLogsByTimeRangePaged 返回错误: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("page.Total = %d, want %d", page.Total, 1)
	}
	if len(page.Logs) != 1 {
		t.Fatalf("len(page.Logs) = %d, want %d", len(page.Logs), 1)
	}
	if got := page.Logs[0].InboundEndpoint; got != "/v1/images/generations" {
		t.Fatalf("InboundEndpoint = %q, want /v1/images/generations", got)
	}
	if got := page.Logs[0].Model; got != "gpt-image-2" {
		t.Fatalf("Model = %q, want gpt-image-2", got)
	}
}
