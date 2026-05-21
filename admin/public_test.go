package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/codex2api/auth"
	"github.com/codex2api/cache"
	"github.com/codex2api/database"
	"github.com/codex2api/proxy"
	"github.com/gin-gonic/gin"
)

func TestPublicHomeOverviewIsRegisteredWithoutAdminAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		proxy.ApplyRuntimeSettings(proxy.DefaultRuntimeSettings())
	})

	dbPath := filepath.Join(t.TempDir(), "codex2api.db")
	db, err := database.New("sqlite", dbPath)
	if err != nil {
		t.Fatalf("database.New 返回错误: %v", err)
	}
	defer db.Close()

	settings := &database.SystemSettings{
		SiteName:                     database.DefaultSiteName,
		MaxConcurrency:               2,
		TestModel:                    "gpt-5.4",
		TestConcurrency:              50,
		PgMaxConns:                   50,
		RedisPoolSize:                30,
		ClientCompatMode:             proxy.ClientCompatModePreserve,
		CodexMinCLIVersion:           "0.118.0",
		UsageLogMode:                 database.UsageLogModeFull,
		UsageLogBatchSize:            200,
		UsageLogFlushIntervalSeconds: 5,
		StreamFlushPolicy:            proxy.StreamFlushPolicyImmediate,
		StreamFlushIntervalMS:        20,
		FilterLocalFallbackResponse:  true,
		APIMaintenanceConfig:         `{"enabled":true,"message":"维护中","routes":{"/v1/responses":{"message":"Responses 维护"},"/v1/messages":{"enabled":false}}}`,
	}
	proxy.ApplyRuntimeSettingsFromSystem(settings)
	if _, err := db.InsertAPIKey(context.Background(), "Public Key", "sk-public-latest-1234567890"); err != nil {
		t.Fatalf("InsertAPIKey 返回错误: %v", err)
	}

	tc := cache.NewMemory(4)
	store := auth.NewStore(db, tc, settings)
	handler := NewHandler(store, db, tc, proxy.NewRateLimiter(120), "secret")

	router := gin.New()
	handler.RegisterRoutes(router)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/public/home", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload publicHomeResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != "ok" {
		t.Fatalf("status = %q, want ok", payload.Status)
	}
	if payload.LatestKey != "sk-public-latest-1234567890" {
		t.Fatalf("latest_key = %q, want inserted key", payload.LatestKey)
	}
	if payload.Maintenance.Message != "维护中" || !payload.Maintenance.Enabled {
		t.Fatalf("maintenance = %#v, want enabled message", payload.Maintenance)
	}
	if payload.Maintenance.RoutesCount != len(payload.Maintenance.Routes) {
		t.Fatalf("routes_count = %d, routes len = %d", payload.Maintenance.RoutesCount, len(payload.Maintenance.Routes))
	}
	if len(payload.Maintenance.Routes) == 0 {
		t.Fatal("maintenance routes = empty, want active endpoints")
	}
	var foundResponses bool
	var foundMessagesNormal bool
	for _, route := range payload.Maintenance.Routes {
		if route.Path == "/v1/messages" {
			foundMessagesNormal = true
			if route.Maintenance {
				t.Fatalf("disabled maintenance route marked maintenance: %#v", route)
			}
		}
		if route.Path == "/v1/responses" {
			foundResponses = true
			if !route.Maintenance {
				t.Fatalf("responses route maintenance = false, want true")
			}
			if route.Message != "Responses 维护" {
				t.Fatalf("responses message = %q, want Responses 维护", route.Message)
			}
		}
	}
	if !foundResponses {
		t.Fatalf("maintenance routes missing /v1/responses: %#v", payload.Maintenance.Routes)
	}
	if !foundMessagesNormal {
		t.Fatalf("maintenance routes missing normal /v1/messages: %#v", payload.Maintenance.Routes)
	}
	if payload.Ops.Traffic.RPMLimit != 120 {
		t.Fatalf("rpm_limit = %d, want 120", payload.Ops.Traffic.RPMLimit)
	}
}

func TestPublicChartDataIsRegisteredWithoutAdminAuth(t *testing.T) {
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
	handler := NewHandler(store, db, tc, proxy.NewRateLimiter(0), "secret")

	router := gin.New()
	handler.RegisterRoutes(router)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/public/chart-data?start=2026-05-21T00:00:00Z&end=2026-05-21T01:00:00Z&bucket_minutes=5", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload database.ChartAggregation
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Timeline == nil {
		t.Fatal("timeline = nil, want empty slice or data")
	}
	if payload.Models == nil {
		t.Fatal("models = nil, want empty slice or data")
	}
}
