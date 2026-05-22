package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

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
	if len(payload.AccountPool.Plans) == 0 {
		t.Fatal("account_pool.plans = empty, want plan buckets")
	}
}

func TestBuildPublicAccountPoolBucketsPlansAndRateLimits(t *testing.T) {
	now := time.Now()
	accounts := []*auth.Account{
		{DBID: 1, AccessToken: "tok-pro", PlanType: "pro", Status: auth.StatusReady},
		{DBID: 2, AccessToken: "tok-lite", PlanType: "prolite", Status: auth.StatusReady},
		{DBID: 3, AccessToken: "tok-plus", PlanType: "plus", Status: auth.StatusCooldown, CooldownReason: "rate_limited", CooldownUtil: now.Add(time.Hour)},
		{DBID: 4, AccessToken: "tok-team", PlanType: "team", Status: auth.StatusReady, UsagePercent5h: 100, UsagePercent5hValid: true, Reset5hAt: now.Add(time.Hour)},
		{DBID: 5, AccessToken: "tok-free", PlanType: "free", Status: auth.StatusReady, UsagePercent7d: 100, UsagePercent7dValid: true, Reset7dAt: now.Add(time.Hour)},
		{DBID: 6, AccessToken: "tok-pro-7d", PlanType: "pro", Status: auth.StatusCooldown, CooldownReason: "rate_limited_7d", CooldownUtil: now.Add(time.Hour)},
		{DBID: 7, PlanType: "free", Status: auth.StatusError},
	}
	atomic.StoreInt32(&accounts[1].Disabled, 1)

	stats := buildPublicAccountPool(accounts)
	if stats.Total != 7 {
		t.Fatalf("total = %d, want 7", stats.Total)
	}
	if stats.Available != 1 {
		t.Fatalf("available = %d, want 1", stats.Available)
	}
	if stats.RateLimited != 4 || stats.RateLimited5h != 2 || stats.RateLimited7d != 2 {
		t.Fatalf("rate limited = %d/%d/%d, want total=4 5h=2 7d=2", stats.RateLimited, stats.RateLimited5h, stats.RateLimited7d)
	}
	if stats.Disabled != 1 || stats.Error != 1 {
		t.Fatalf("disabled/error = %d/%d, want 1/1", stats.Disabled, stats.Error)
	}

	plans := map[string]publicAccountPlanResponse{}
	for _, plan := range stats.Plans {
		plans[plan.Type] = plan
	}
	for _, planType := range []string{"pro", "prolite", "plus", "team", "free", "api"} {
		if _, ok := plans[planType]; !ok {
			t.Fatalf("missing plan bucket %s in %#v", planType, stats.Plans)
		}
	}
	if plans["prolite"].Total != 1 {
		t.Fatalf("prolite total = %d, want 1", plans["prolite"].Total)
	}
	if plans["free"].RateLimited != 1 {
		t.Fatalf("free rate_limited = %d, want 1", plans["free"].RateLimited)
	}
}

func TestBuildPublicMaintenanceRoutesReturnsAvailableRoutesWhenDisabled(t *testing.T) {
	routes := buildPublicMaintenanceRoutes(proxy.DefaultAPIMaintenanceConfig())

	if len(routes) != len(publicMaintenancePaths) {
		t.Fatalf("routes len = %d, want %d", len(routes), len(publicMaintenancePaths))
	}
	for _, route := range routes {
		if route.Maintenance {
			t.Fatalf("route %s marked maintenance while maintenance disabled", route.Path)
		}
		if route.Message != "" {
			t.Fatalf("route %s message = %q, want empty", route.Path, route.Message)
		}
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
