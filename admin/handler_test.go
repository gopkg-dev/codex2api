package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/cache"
	"github.com/codex2api/database"
	"github.com/codex2api/proxy"
	"github.com/gin-gonic/gin"
)

func TestRefreshAccountRejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{
		refreshAccount: func(context.Context, int64) error {
			t.Fatal("refresh should not be called for invalid id")
			return nil
		},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/bad-id/refresh", nil)

	handler.RefreshAccount(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["error"]; got != "无效的账号 ID" {
		t.Fatalf("error = %q, want %q", got, "无效的账号 ID")
	}
}

func TestRefreshAccountRunsSingleRefresh(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var called bool
	var gotID int64
	handler := &Handler{
		refreshAccount: func(_ context.Context, id int64) error {
			called = true
			gotID = id
			return nil
		},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "42"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/42/refresh", nil)

	handler.RefreshAccount(ctx)

	if !called {
		t.Fatal("expected refresh to be called")
	}
	if gotID != 42 {
		t.Fatalf("refresh id = %d, want %d", gotID, 42)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["message"]; got != "账号刷新成功" {
		t.Fatalf("message = %q, want %q", got, "账号刷新成功")
	}
}

func TestRefreshAccountReturnsNotFoundForMissingAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{
		refreshAccount: func(context.Context, int64) error {
			return errors.New("账号 7 不存在")
		},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "7"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/7/refresh", nil)

	handler.RefreshAccount(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}

	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["error"]; got != "账号 7 不存在" {
		t.Fatalf("error = %q, want %q", got, "账号 7 不存在")
	}
}

func TestRefreshAccountReturnsRefreshFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{
		refreshAccount: func(context.Context, int64) error {
			return errors.New("upstream unavailable")
		},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "9"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/9/refresh", nil)

	handler.RefreshAccount(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}

	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["error"]; got != "刷新失败: upstream unavailable" {
		t.Fatalf("error = %q, want %q", got, "刷新失败: upstream unavailable")
	}
}

func TestCreateAPIKeyPersistsQuotaAndExpiration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	db, err := database.New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("database.New 返回错误: %v", err)
	}
	defer db.Close()

	handler := &Handler{db: db}
	body := `{"name":"Client A","key":"sk-test-client-a-1234567890","quota_limit":0.25,"expires_in_days":7}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/keys", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.CreateAPIKey(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload createAPIKeyResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ID <= 0 || payload.QuotaLimit != 0.25 || payload.ExpiresAt == nil {
		t.Fatalf("payload = %#v, want quota and expiration", payload)
	}

	row, err := db.GetAPIKeyByValue(context.Background(), "sk-test-client-a-1234567890")
	if err != nil {
		t.Fatalf("GetAPIKeyByValue 返回错误: %v", err)
	}
	if row.QuotaLimit != 0.25 || !row.ExpiresAt.Valid {
		t.Fatalf("row = %#v, want quota and expiration", row)
	}
}

func TestUpdateAPIKeyPreservesOmittedFieldsAndUpdatesLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	db, err := database.New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("database.New 返回错误: %v", err)
	}
	defer db.Close()

	expiresAt := sql.NullTime{Time: time.Now().AddDate(0, 0, 3), Valid: true}
	id, err := db.InsertAPIKeyWithOptions(context.Background(), database.APIKeyInput{
		Name:       "Client A",
		Key:        "sk-test-update-client-1234567890",
		QuotaLimit: 0.25,
		ExpiresAt:  expiresAt,
	})
	if err != nil {
		t.Fatalf("InsertAPIKeyWithOptions 返回错误: %v", err)
	}

	handler := &Handler{db: db}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", id)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/api/admin/keys/1", strings.NewReader(`{"name":"Client B"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAPIKey(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	row, err := db.GetAPIKeyByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetAPIKeyByID 返回错误: %v", err)
	}
	if row.Name != "Client B" || row.QuotaLimit != 0.25 || !row.ExpiresAt.Valid {
		t.Fatalf("row = %#v, want renamed with quota/expiration preserved", row)
	}

	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", id)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/api/admin/keys/1", strings.NewReader(`{"quota_limit":0,"expires_at":null,"disabled":true}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAPIKey(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	row, err = db.GetAPIKeyByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetAPIKeyByID 返回错误: %v", err)
	}
	if row.Name != "Client B" || row.QuotaLimit != 0 || row.ExpiresAt.Valid || !row.Disabled {
		t.Fatalf("row = %#v, want quota/expiration cleared with name preserved and disabled", row)
	}
}

func TestUpdateAPIKeyRefreshesRuntimeStoreAndCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	db, err := database.New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("database.New 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	groupID, err := db.CreateAccountGroup(ctx, "Team", "", "#2563eb", 0)
	if err != nil {
		t.Fatalf("CreateAccountGroup 返回错误: %v", err)
	}
	key := "sk-test-runtime-refresh-1234567890"
	keyID, err := db.InsertAPIKey(ctx, "Client A", key)
	if err != nil {
		t.Fatalf("InsertAPIKey 返回错误: %v", err)
	}
	store := auth.NewStore(nil, nil, nil)
	tc := cache.NewMemory(1)
	handler := &Handler{db: db, store: store, cache: tc}
	payload, err := json.Marshal(map[string]interface{}{
		"id":         keyID,
		"name":       "Client A",
		"created_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal runtime cache: %v", err)
	}
	if err := tc.SetRuntime(ctx, adminAPIKeyCacheNamespace, key, payload, time.Minute); err != nil {
		t.Fatalf("SetRuntime api key: %v", err)
	}

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", keyID)}}
	ginCtx.Request = httptest.NewRequest(http.MethodPatch, "/api/admin/keys/1", strings.NewReader(fmt.Sprintf(`{"allowed_group_ids":[%d]}`, groupID)))
	ginCtx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAPIKey(ginCtx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := store.GetAPIKeyAllowedGroups(keyID); len(got) != 1 || got[0] != groupID {
		t.Fatalf("runtime store allowed groups = %v, want [%d]", got, groupID)
	}
	if _, ok, err := tc.GetRuntime(ctx, adminAPIKeyCacheNamespace, key); err != nil || ok {
		t.Fatalf("runtime api key cache after update ok=%v err=%v, want miss", ok, err)
	}
	if _, ok, err := tc.GetRuntime(ctx, adminAPIKeyCountNamespace, "all"); err != nil || ok {
		t.Fatalf("runtime api key count cache after update ok=%v err=%v, want miss", ok, err)
	}
}

func TestUpdateSettingsPersistsMaintenanceRuntimeConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	db, err := database.New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("database.New 返回错误: %v", err)
	}
	defer db.Close()

	tc := cache.NewMemory(1)
	defer tc.Close()
	settings := &database.SystemSettings{
		SiteName:                         database.DefaultSiteName,
		MaxConcurrency:                   2,
		TestModel:                        "gpt-5.4",
		TestConcurrency:                  50,
		BackgroundRefreshIntervalMinutes: 2,
		UsageProbeMaxAgeMinutes:          10,
		RecoveryProbeIntervalMinutes:     30,
		PgMaxConns:                       50,
		RedisPoolSize:                    30,
		ClientCompatMode:                 proxy.ClientCompatModePreserve,
		CodexMinCLIVersion:               "0.118.0",
		UsageLogMode:                     database.UsageLogModeFull,
		UsageLogBatchSize:                200,
		UsageLogFlushIntervalSeconds:     5,
		StreamFlushPolicy:                proxy.StreamFlushPolicyImmediate,
		StreamFlushIntervalMS:            20,
		IPQPSLimit:                       2,
		IPRPMLimit:                       20,
		IPAutoBanDurationMinutes:         30,
		IPAutoBanOnQPS:                   true,
		IPAutoBanOnRPM:                   true,
		FilterLocalFallbackResponse:      true,
		APIMaintenanceConfig:             proxy.EncodeAPIMaintenanceConfig(proxy.DefaultAPIMaintenanceConfig()),
	}
	if err := db.UpdateSystemSettings(context.Background(), settings); err != nil {
		t.Fatalf("UpdateSystemSettings 返回错误: %v", err)
	}

	store := auth.NewStore(db, tc, settings)
	handler := NewHandler(store, db, tc, proxy.NewRateLimiter(0), "")
	body := `{
		"ip_qps_limit": 4,
		"ip_rpm_limit": 9,
		"ip_auto_ban_enabled": true,
		"ip_auto_ban_duration_minutes": 45,
		"filter_local_fallback_response": false,
		"disable_fast_service_tier": true,
		"image_generation_tool_mode": "force_off",
		"api_maintenance_message": "维护中",
		"api_maintenance_sse_randomize": true,
		"api_maintenance_image_b64_json": "abc",
		"api_maintenance_routes_json": "{\"/v1/responses\":{\"enabled\":true}}"
	}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/admin/settings", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload settingsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.FilterLocalFallbackResponse {
		t.Fatal("FilterLocalFallbackResponse = true, want false")
	}
	if !payload.DisableFastServiceTier {
		t.Fatal("DisableFastServiceTier = false, want true")
	}
	if payload.ImageGenerationToolMode != "force_off" {
		t.Fatalf("ImageGenerationToolMode = %q, want force_off", payload.ImageGenerationToolMode)
	}
	if strings.Contains(recorder.Body.String(), `"api_key_disabled_message"`) {
		t.Fatalf("response still exposes api_key_disabled_message: %s", recorder.Body.String())
	}
	if payload.APIMaintenanceMessage != "维护中" || !payload.APIMaintenanceSSERandomize {
		t.Fatalf("maintenance response = %#v", payload)
	}
	if payload.IPQPSLimit != 4 {
		t.Fatalf("IPQPSLimit = %d, want 4", payload.IPQPSLimit)
	}
	if payload.IPRPMLimit != 9 {
		t.Fatalf("IPRPMLimit = %d, want 9", payload.IPRPMLimit)
	}
	if !payload.IPAutoBanEnabled || payload.IPAutoBanDurationMinutes != 45 {
		t.Fatalf("IP auto ban payload = enabled=%v duration=%d", payload.IPAutoBanEnabled, payload.IPAutoBanDurationMinutes)
	}

	gotSettings, err := db.GetSystemSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSystemSettings 返回错误: %v", err)
	}
	if gotSettings.FilterLocalFallbackResponse {
		t.Fatal("persisted FilterLocalFallbackResponse = true, want false")
	}
	if !gotSettings.DisableFastServiceTier {
		t.Fatal("persisted DisableFastServiceTier = false, want true")
	}
	if gotSettings.ImageGenerationToolMode != "force_off" {
		t.Fatalf("persisted ImageGenerationToolMode = %q, want force_off", gotSettings.ImageGenerationToolMode)
	}
	if gotSettings.IPQPSLimit != 4 {
		t.Fatalf("persisted IPQPSLimit = %d, want 4", gotSettings.IPQPSLimit)
	}
	if gotSettings.IPRPMLimit != 9 {
		t.Fatalf("persisted IPRPMLimit = %d, want 9", gotSettings.IPRPMLimit)
	}
	if !gotSettings.IPAutoBanEnabled || gotSettings.IPAutoBanDurationMinutes != 45 {
		t.Fatalf("persisted auto ban = enabled=%v duration=%d", gotSettings.IPAutoBanEnabled, gotSettings.IPAutoBanDurationMinutes)
	}
	if got := handler.rateLimiter.GetIPQPSLimit(); got != 4 {
		t.Fatalf("runtime IPQPSLimit = %d, want 4", got)
	}
	if got := handler.rateLimiter.GetIPRPMLimit(); got != 9 {
		t.Fatalf("runtime IPRPMLimit = %d, want 9", got)
	}
	runtimeCfg := proxy.CurrentRuntimeSettings()
	if runtimeCfg.FilterLocalFallbackResponse {
		t.Fatal("runtime FilterLocalFallbackResponse = true, want false")
	}
	if !runtimeCfg.DisableFastServiceTier {
		t.Fatal("runtime DisableFastServiceTier = false, want true")
	}
	if runtimeCfg.ImageGenerationToolMode != proxy.ImageGenerationToolModeForceOff {
		t.Fatalf("runtime ImageGenerationToolMode = %q, want %q", runtimeCfg.ImageGenerationToolMode, proxy.ImageGenerationToolModeForceOff)
	}
	route := runtimeCfg.APIMaintenance.Routes["/v1/responses"]
	if route.Enabled == nil || !*route.Enabled || runtimeCfg.APIMaintenance.Message != "维护中" {
		t.Fatalf("runtime maintenance = %#v", runtimeCfg.APIMaintenance)
	}
}

func TestCreateIPBansBatchAddsValidIPsAndRefreshesRuntime(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	db, err := database.New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("database.New 返回错误: %v", err)
	}
	defer db.Close()

	tc := cache.NewMemory(4)
	settings := &database.SystemSettings{MaxConcurrency: 2, TestModel: "gpt-5.4", TestConcurrency: 50}
	store := auth.NewStore(db, tc, settings)
	rateLimiter := proxy.NewRateLimiter(0)
	handler := NewHandler(store, db, tc, rateLimiter, "secret")

	body := `{
		"ips": ["203.0.113.50, 203.0.113.51", "203.0.113.50", "198.51.100.0/24"],
		"reason": "manual",
		"source": "manual",
		"expires_in_minutes": 30
	}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/ip-bans/batch", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.CreateIPBansBatch(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload createIPBansBatchResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Created != 2 || payload.ErrorCount != 1 {
		t.Fatalf("batch response = created %d errors %d, want 2/1: %#v", payload.Created, payload.ErrorCount, payload)
	}

	router := gin.New()
	router.Use(rateLimiter.Middleware())
	router.POST("/v1/responses", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	banned := httptest.NewRecorder()
	bannedReq := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4"}`))
	bannedReq.RemoteAddr = "203.0.113.50:1234"
	router.ServeHTTP(banned, bannedReq)
	if banned.Code != http.StatusOK {
		t.Fatalf("banned status = %d, want 200; body=%s", banned.Code, banned.Body.String())
	}
	if body := banned.Body.String(); !strings.Contains(body, "触发风控已被锁定") {
		t.Fatalf("banned body = %s, want protocol ban message", body)
	}
}

func TestListIPBansPaginates(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	db, err := database.New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("database.New 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	for _, ip := range []string{"203.0.113.10", "203.0.113.11", "203.0.113.12"} {
		if _, err := db.CreateIPBan(ctx, database.IPBanInput{
			IP:      ip,
			Reason:  database.IPBanReasonManual,
			Source:  database.IPBanSourceManual,
			Enabled: true,
		}); err != nil {
			t.Fatalf("CreateIPBan(%s) 返回错误: %v", ip, err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	handler := &Handler{db: db}
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodGet, "/api/admin/ip-bans?include_inactive=1&page=2&page_size=2", nil)

	handler.ListIPBans(ginCtx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload listIPBansResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Total != 3 || payload.Page != 2 || payload.PageSize != 2 {
		t.Fatalf("pagination = total %d page %d page_size %d, want 3/2/2", payload.Total, payload.Page, payload.PageSize)
	}
	if len(payload.Bans) != 1 || payload.Bans[0].IP != "203.0.113.10" {
		t.Fatalf("bans = %#v, want second page with oldest 203.0.113.10", payload.Bans)
	}
}

func TestGetPublicIPBansReturnsTopTwentyAndSupportsIPQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	db, err := database.New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("database.New 返回错误: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	for i := 0; i < 25; i++ {
		ip := fmt.Sprintf("203.0.113.%d", i+1)
		if _, err := db.CreateIPBan(ctx, database.IPBanInput{
			IP:      ip,
			Reason:  database.IPBanReasonManual,
			Source:  database.IPBanSourceManual,
			Enabled: true,
		}); err != nil {
			t.Fatalf("CreateIPBan(%s) 返回错误: %v", ip, err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	handler := &Handler{db: db}
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodGet, "/api/public/ip-bans", nil)

	handler.GetPublicIPBans(ginCtx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload listIPBansResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Total != 25 || payload.Page != 1 || payload.PageSize != 20 || len(payload.Bans) != 20 {
		t.Fatalf("pagination = total %d page %d page_size %d len %d, want 25/1/20/20", payload.Total, payload.Page, payload.PageSize, len(payload.Bans))
	}
	if payload.Bans[0].IP != "203.0.113.25" || payload.Bans[19].IP != "203.0.113.6" {
		t.Fatalf("public bans order = first %s last %s, want newest first 25..6", payload.Bans[0].IP, payload.Bans[19].IP)
	}

	queryRecorder := httptest.NewRecorder()
	queryCtx, _ := gin.CreateTestContext(queryRecorder)
	queryCtx.Request = httptest.NewRequest(http.MethodGet, "/api/public/ip-bans?ip=203.0.113.25", nil)

	handler.GetPublicIPBans(queryCtx)

	if queryRecorder.Code != http.StatusOK {
		t.Fatalf("query status = %d, want %d; body=%s", queryRecorder.Code, http.StatusOK, queryRecorder.Body.String())
	}
	var queryPayload listIPBansResponse
	if err := json.Unmarshal(queryRecorder.Body.Bytes(), &queryPayload); err != nil {
		t.Fatalf("decode query response: %v", err)
	}
	if queryPayload.Total != 1 || len(queryPayload.Bans) != 1 || queryPayload.Bans[0].IP != "203.0.113.25" {
		t.Fatalf("query payload = %#v, want single queried IP", queryPayload)
	}
}

func TestGetAccountAuthJSONRejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/admin/accounts/bad-id/auth-json", nil)

	handler.GetAccountAuthJSON(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	assertErrorMessage(t, recorder, "无效的账号 ID")
}

func TestGetAccountAuthJSONReturnsCodexAuthFile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	accountID := insertTestAccount(t, db)
	if err := db.UpdateCredentials(context.Background(), accountID, map[string]interface{}{
		"id_token":     "id_test",
		"access_token": "access_test",
		"account_id":   "account_test",
	}); err != nil {
		t.Fatalf("seed credentials: %v", err)
	}
	handler := &Handler{db: db}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", accountID)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/accounts/%d/auth-json", accountID), nil)

	handler.GetAccountAuthJSON(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Disposition"); got != `attachment; filename="auth.json"` {
		t.Fatalf("Content-Disposition = %q, want auth.json attachment", got)
	}

	var payload struct {
		AuthMode     string  `json:"auth_mode"`
		OpenAIAPIKey *string `json:"OPENAI_API_KEY"`
		Tokens       struct {
			IDToken      string `json:"id_token"`
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			AccountID    string `json:"account_id"`
		} `json:"tokens"`
		LastRefresh string `json:"last_refresh"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.AuthMode != "chatgpt" {
		t.Fatalf("auth_mode = %q, want chatgpt", payload.AuthMode)
	}
	if payload.OpenAIAPIKey != nil {
		t.Fatalf("OPENAI_API_KEY = %q, want null", *payload.OpenAIAPIKey)
	}
	if payload.Tokens.IDToken != "id_test" || payload.Tokens.AccessToken != "access_test" || payload.Tokens.RefreshToken != "rt_test" || payload.Tokens.AccountID != "account_test" {
		t.Fatalf("tokens = %+v, want seeded credentials", payload.Tokens)
	}
	if payload.LastRefresh == "" {
		t.Fatal("last_refresh is empty")
	}
}

func TestGetAccountAuthJSONRejectsIncompleteTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	accountID := insertTestAccount(t, db)
	handler := &Handler{db: db}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", accountID)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/accounts/%d/auth-json", accountID), nil)

	handler.GetAccountAuthJSON(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	assertErrorMessage(t, recorder, "账号缺少 access_token 或 id_token，请先刷新账号后再生成 auth.json")
}

func TestGetUsageLogsRejectsInvalidAPIKeyID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/admin/usage/logs?start=2026-01-01T00:00:00Z&end=2026-01-02T00:00:00Z&page=1&api_key_id=bad", nil)

	handler.GetUsageLogs(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["error"]; got != "api_key_id 参数无效，需要正整数" {
		t.Fatalf("error = %q, want %q", got, "api_key_id 参数无效，需要正整数")
	}
}

func TestUpdateAccountSchedulerRejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/api/admin/accounts/bad-id/scheduler", http.NoBody)

	handler.UpdateAccountScheduler(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	assertErrorMessage(t, recorder, "无效的账号 ID")
}

func TestUpdateAccountSchedulerRejectsInvalidBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	handler := &Handler{db: db}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "1"}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/api/admin/accounts/1/scheduler", strings.NewReader(`{"score_bias_override":"abc"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAccountScheduler(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	assertErrorMessage(t, recorder, "score_bias_override 必须是整数或 null")
}

func TestUpdateAccountSchedulerRejectsInvalidAllowedAPIKeyIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	accountID := insertTestAccount(t, db)
	handler := &Handler{db: db}

	testCases := []struct {
		name    string
		body    string
		message string
	}{
		{
			name:    "invalid type",
			body:    `{"allowed_api_key_ids":"abc"}`,
			message: "allowed_api_key_ids 必须是整数数组或 null",
		},
		{
			name:    "non positive",
			body:    `{"allowed_api_key_ids":[0]}`,
			message: "allowed_api_key_ids 中的值必须是正整数",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", accountID)}}
			ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/accounts/%d/scheduler", accountID), strings.NewReader(tc.body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			handler.UpdateAccountScheduler(ctx)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			assertErrorMessage(t, recorder, tc.message)
		})
	}
}

func TestUpdateAccountSchedulerRejectsOutOfRangeValues(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	accountID := insertTestAccount(t, db)
	handler := &Handler{db: db}

	testCases := []struct {
		name    string
		body    string
		message string
	}{
		{
			name:    "score bias out of range",
			body:    `{"score_bias_override":201}`,
			message: "score_bias_override 超出范围，必须在 -200..200 之间",
		},
		{
			name:    "base concurrency out of range",
			body:    `{"base_concurrency_override":0}`,
			message: "base_concurrency_override 超出范围，必须在 1..50 之间",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", accountID)}}
			ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/accounts/%d/scheduler", accountID), strings.NewReader(tc.body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			handler.UpdateAccountScheduler(ctx)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			assertErrorMessage(t, recorder, tc.message)
		})
	}
}

func TestUpdateAccountSchedulerPersistsOverrides(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	accountID := insertTestAccount(t, db)
	handler := &Handler{db: db}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", accountID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/accounts/%d/scheduler", accountID), strings.NewReader(`{"score_bias_override":88,"base_concurrency_override":7}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAccountScheduler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if !rows[0].ScoreBiasOverride.Valid || rows[0].ScoreBiasOverride.Int64 != 88 {
		t.Fatalf("score_bias_override = %+v, want 88", rows[0].ScoreBiasOverride)
	}
	if !rows[0].BaseConcurrencyOverride.Valid || rows[0].BaseConcurrencyOverride.Int64 != 7 {
		t.Fatalf("base_concurrency_override = %+v, want 7", rows[0].BaseConcurrencyOverride)
	}
}

func TestUpdateAccountSchedulerPersistsAllowedAPIKeyIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	accountID := insertTestAccount(t, db)
	keyID1 := insertTestAPIKey(t, db, "Team A")
	keyID2 := insertTestAPIKey(t, db, "Team B")
	handler := &Handler{db: db}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", accountID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/accounts/%d/scheduler", accountID), strings.NewReader(fmt.Sprintf(`{"score_bias_override":88,"base_concurrency_override":7,"allowed_api_key_ids":[%d,%d,%d]}`, keyID2, keyID1, keyID2)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAccountScheduler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if got := rows[0].GetCredentialInt64Slice("allowed_api_key_ids"); len(got) != 2 || got[0] != keyID1 || got[1] != keyID2 {
		t.Fatalf("allowed_api_key_ids = %v, want [%d %d]", got, keyID1, keyID2)
	}
}

func TestUpdateAccountSchedulerResetsToAutoOnNull(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	accountID := insertTestAccount(t, db)
	ctx := context.Background()
	if err := db.UpdateAccountSchedulerConfig(ctx, accountID, database.OptionalNullInt64{Set: true, Value: sql.NullInt64{Int64: 20, Valid: true}}, database.OptionalNullInt64{Set: true, Value: sql.NullInt64{Int64: 4, Valid: true}}, database.OptionalInt64Slice{}); err != nil {
		t.Fatalf("seed scheduler config: %v", err)
	}

	handler := &Handler{db: db}
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", accountID)}}
	ginCtx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/accounts/%d/scheduler", accountID), strings.NewReader(`{"score_bias_override":null,"base_concurrency_override":null}`))
	ginCtx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAccountScheduler(ginCtx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].ScoreBiasOverride.Valid {
		t.Fatalf("score_bias_override = %+v, want null", rows[0].ScoreBiasOverride)
	}
	if rows[0].BaseConcurrencyOverride.Valid {
		t.Fatalf("base_concurrency_override = %+v, want null", rows[0].BaseConcurrencyOverride)
	}
}

func TestUpdateAccountSchedulerPartialMetadataPatchPreservesSchedulerConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	accountID := insertTestAccount(t, db)
	keyID := insertTestAPIKey(t, db, "Team A")
	ctx := context.Background()
	if err := db.UpdateAccountSchedulerConfig(ctx, accountID,
		database.OptionalNullInt64{Set: true, Value: sql.NullInt64{Int64: 20, Valid: true}},
		database.OptionalNullInt64{Set: true, Value: sql.NullInt64{Int64: 4, Valid: true}},
		database.OptionalInt64Slice{Set: true, Values: []int64{keyID}},
	); err != nil {
		t.Fatalf("seed scheduler config: %v", err)
	}

	handler := &Handler{db: db}
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", accountID)}}
	ginCtx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/accounts/%d/scheduler", accountID), strings.NewReader(`{"tags":["ops"]}`))
	ginCtx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAccountScheduler(ginCtx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if !rows[0].ScoreBiasOverride.Valid || rows[0].ScoreBiasOverride.Int64 != 20 {
		t.Fatalf("score_bias_override = %+v, want 20", rows[0].ScoreBiasOverride)
	}
	if !rows[0].BaseConcurrencyOverride.Valid || rows[0].BaseConcurrencyOverride.Int64 != 4 {
		t.Fatalf("base_concurrency_override = %+v, want 4", rows[0].BaseConcurrencyOverride)
	}
	if got := rows[0].GetCredentialInt64Slice("allowed_api_key_ids"); len(got) != 1 || got[0] != keyID {
		t.Fatalf("allowed_api_key_ids = %v, want [%d]", got, keyID)
	}
}

func TestUpdateAccountSchedulerClearsAllowedAPIKeyIDsOnNull(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	accountID := insertTestAccount(t, db)
	keyID := insertTestAPIKey(t, db, "Team A")
	if err := db.UpdateCredentials(context.Background(), accountID, map[string]interface{}{
		"allowed_api_key_ids": []int64{keyID},
	}); err != nil {
		t.Fatalf("seed allowed api keys: %v", err)
	}

	handler := &Handler{db: db}
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", accountID)}}
	ginCtx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/accounts/%d/scheduler", accountID), strings.NewReader(`{"score_bias_override":null,"base_concurrency_override":null,"allowed_api_key_ids":null}`))
	ginCtx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAccountScheduler(ginCtx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if got := rows[0].GetCredentialInt64Slice("allowed_api_key_ids"); len(got) != 0 {
		t.Fatalf("allowed_api_key_ids = %v, want empty", got)
	}
}

func TestUpdateAccountSchedulerKeepsAllowedAPIKeyIDsWhenFieldOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	accountID := insertTestAccount(t, db)
	keyID := insertTestAPIKey(t, db, "Team A")
	if err := db.UpdateCredentials(context.Background(), accountID, map[string]interface{}{
		"allowed_api_key_ids": []int64{keyID},
	}); err != nil {
		t.Fatalf("seed allowed api keys: %v", err)
	}

	handler := &Handler{db: db}
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", accountID)}}
	ginCtx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/accounts/%d/scheduler", accountID), strings.NewReader(`{"score_bias_override":12,"base_concurrency_override":3}`))
	ginCtx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAccountScheduler(ginCtx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if got := rows[0].GetCredentialInt64Slice("allowed_api_key_ids"); len(got) != 1 || got[0] != keyID {
		t.Fatalf("allowed_api_key_ids = %v, want [%d]", got, keyID)
	}
}

func TestUpdateAccountSchedulerRejectsMissingAllowedAPIKeyID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	accountID := insertTestAccount(t, db)
	handler := &Handler{db: db}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", accountID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/accounts/%d/scheduler", accountID), strings.NewReader(`{"allowed_api_key_ids":[999]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAccountScheduler(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	assertErrorMessage(t, recorder, "allowed_api_key_ids 包含不存在的 API Key ID: 999")
}

func TestUpdateAccountSchedulerUpdatesRuntimeOverrides(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)
	accountID := insertTestAccount(t, db)
	keyID1 := insertTestAPIKey(t, db, "Team A")
	keyID2 := insertTestAPIKey(t, db, "Team B")
	runtimeAccount := &auth.Account{
		DBID:        accountID,
		AccessToken: "token",
		Status:      auth.StatusReady,
		PlanType:    "pro",
	}
	store := &auth.Store{}
	store.AddAccount(runtimeAccount)

	handler := &Handler{db: db, store: store}
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", accountID)}}
	ginCtx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/accounts/%d/scheduler", accountID), strings.NewReader(fmt.Sprintf(`{"score_bias_override":33,"base_concurrency_override":5,"allowed_api_key_ids":[%d,%d]}`, keyID2, keyID1)))
	ginCtx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateAccountScheduler(ginCtx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	scoreBias, ok := runtimeAccount.GetScoreBiasOverride()
	if !ok || scoreBias != 33 {
		t.Fatalf("runtime score_bias_override = (%d, %t), want (33, true)", scoreBias, ok)
	}
	baseConcurrency, ok := runtimeAccount.GetBaseConcurrencyOverride()
	if !ok || baseConcurrency != 5 {
		t.Fatalf("runtime base_concurrency_override = (%d, %t), want (5, true)", baseConcurrency, ok)
	}
	if got := runtimeAccount.GetAllowedAPIKeyIDs(); len(got) != 2 || got[0] != keyID1 || got[1] != keyID2 {
		t.Fatalf("runtime allowed_api_key_ids = %v, want [%d %d]", got, keyID1, keyID2)
	}
}

// AT-only 账号(没有 refresh_token,只靠 access_token)是规避 Codex Plus "add
// phone" 流程的常用形态。导出/迁移以前会因为 rt=="" 直接跳过这些账号,导致
// issue #123 中的迁移丢号。下面两个测试保护已修好的过滤逻辑。
func TestExportAccountsIncludesATOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)

	rtID, err := db.InsertAccount(context.Background(), "rt-account", "rt_value", "")
	if err != nil {
		t.Fatalf("insert rt account: %v", err)
	}
	if err := db.UpdateCredentials(context.Background(), rtID, map[string]interface{}{
		"email":        "rt@example.com",
		"access_token": "at_for_rt",
	}); err != nil {
		t.Fatalf("update rt credentials: %v", err)
	}

	atID, err := db.InsertAccount(context.Background(), "at-account", "", "")
	if err != nil {
		t.Fatalf("insert at-only account: %v", err)
	}
	if err := db.UpdateCredentials(context.Background(), atID, map[string]interface{}{
		"email":        "at@example.com",
		"access_token": "at_only_value",
	}); err != nil {
		t.Fatalf("update at-only credentials: %v", err)
	}

	handler := &Handler{db: db}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/admin/accounts/export?filter=all", nil)

	handler.ExportAccounts(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var entries []cpaExportEntry
	if err := json.Unmarshal(recorder.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (rt + at-only)", len(entries))
	}

	byEmail := make(map[string]cpaExportEntry, len(entries))
	for _, e := range entries {
		byEmail[e.Email] = e
	}

	rt, ok := byEmail["rt@example.com"]
	if !ok {
		t.Fatal("rt-based account missing from export")
	}
	if rt.RefreshToken != "rt_value" || rt.AccessToken != "at_for_rt" {
		t.Fatalf("rt entry tokens = (rt=%q, at=%q), want (rt_value, at_for_rt)", rt.RefreshToken, rt.AccessToken)
	}

	at, ok := byEmail["at@example.com"]
	if !ok {
		t.Fatal("AT-only account missing from export")
	}
	if at.RefreshToken != "" {
		t.Fatalf("AT-only RefreshToken = %q, want empty", at.RefreshToken)
	}
	if at.AccessToken != "at_only_value" {
		t.Fatalf("AT-only AccessToken = %q, want at_only_value", at.AccessToken)
	}
}

func TestExportAccountsSkipsAccountsWithoutCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newTestAdminDB(t)

	if _, err := db.InsertAccount(context.Background(), "empty-account", "", ""); err != nil {
		t.Fatalf("insert empty account: %v", err)
	}

	handler := &Handler{db: db}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/admin/accounts/export?filter=all", nil)

	handler.ExportAccounts(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var entries []cpaExportEntry
	if err := json.Unmarshal(recorder.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("got %d entries, want 0 (account has no credentials)", len(entries))
	}
}

func newTestAdminDB(t *testing.T) *database.DB {
	t.Helper()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "admin-handler-test.sqlite")
	db, err := database.New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("new test db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_ = os.Remove(dbPath)
	})
	return db
}

func insertTestAccount(t *testing.T, db *database.DB) int64 {
	t.Helper()

	id, err := db.InsertAccount(context.Background(), "test-account", "rt_test", "")
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	return id
}

func insertTestAPIKey(t *testing.T, db *database.DB, name string) int64 {
	t.Helper()

	id, err := db.InsertAPIKey(context.Background(), name, fmt.Sprintf("sk-test-%s-1234567890", strings.ToLower(strings.ReplaceAll(name, " ", "-"))))
	if err != nil {
		t.Fatalf("insert api key: %v", err)
	}
	return id
}

func assertErrorMessage(t *testing.T, recorder *httptest.ResponseRecorder, want string) {
	t.Helper()

	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := payload["error"]; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}
