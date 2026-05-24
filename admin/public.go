package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/codex2api/auth"
	"github.com/codex2api/proxy"
	"github.com/gin-gonic/gin"
)

var publicMaintenancePaths = []string{
	"/v1/chat/completions",
	"/v1/responses",
	"/v1/responses/compact",
	"/v1/messages",
	"/v1/images/generations",
	"/v1/images/edits",
}

// GetPublicHome 返回公开首页所需的聚合状态。
func (h *Handler) GetPublicHome(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	usage, err := h.getUsageStatsCached(ctx, time.Time{}, time.Time{})
	if err != nil {
		writeInternalError(c, err)
		return
	}

	ipWindowStart := parseIPStatsWindowStart(c.Query("ip_window"), time.Now())
	ops, err := h.buildOpsOverview(ctx, ipWindowStart)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	ipStats, err := h.db.GetIPUsageStats(ctx, 20, ipWindowStart)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	settings, err := h.db.GetSystemSettings(ctx)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	ipRPMLimit := 0
	ipQPSLimit := 0
	if settings != nil {
		ipRPMLimit = settings.IPRPMLimit
		ipQPSLimit = settings.IPQPSLimit
	}
	activeBans, err := h.db.ListIPBans(ctx, false)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	classifyIPStatStatuses(ipStats, ipRPMLimit, ipQPSLimit, activeIPBanSet(activeBans))
	ops.IPStats = ipStats

	maintenance := proxy.CurrentRuntimeSettings().APIMaintenance
	maintenanceRoutes := buildPublicMaintenanceRoutes(maintenance)
	latestKey, err := h.getLatestPublicAPIKey(ctx)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, publicHomeResponse{
		Status:      "ok",
		UpdatedAt:   time.Now().Format(time.RFC3339),
		Usage:       usage,
		Ops:         ops,
		AccountPool: buildPublicAccountPool(h.store.Accounts()),
		LatestKey:   latestKey,
		Maintenance: publicMaintenanceResponse{
			Enabled:     maintenance.Enabled,
			Message:     maintenance.Message,
			RoutesCount: len(maintenanceRoutes),
			Routes:      maintenanceRoutes,
		},
	})
}

func (h *Handler) getLatestPublicAPIKey(ctx context.Context) (string, error) {
	keys, err := h.db.ListAPIKeys(ctx)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", nil
	}
	latest := keys[0]
	for _, key := range keys[1:] {
		if key.CreatedAt.After(latest.CreatedAt) || (key.CreatedAt.Equal(latest.CreatedAt) && key.ID > latest.ID) {
			latest = key
		}
	}
	return latest.Key, nil
}

// GetPublicChartData 返回公开首页图表聚合数据。
func (h *Handler) GetPublicChartData(c *gin.Context) {
	h.GetChartData(c)
}

// GetPublicIPBans 返回公开首页 IP 黑名单查询数据。
func (h *Handler) GetPublicIPBans(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	queryIP := strings.TrimSpace(c.Query("ip"))
	if queryIP != "" {
		addr, err := netip.ParseAddr(queryIP)
		if err != nil {
			c.JSON(http.StatusOK, listIPBansResponse{
				Bans:     []ipBanResponse{},
				Total:    0,
				Page:     1,
				PageSize: 20,
			})
			return
		}
		row, err := h.db.GetIPBanByIP(ctx, addr.String())
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusOK, listIPBansResponse{
					Bans:     []ipBanResponse{},
					Total:    0,
					Page:     1,
					PageSize: 20,
				})
				return
			}
			writeInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, listIPBansResponse{
			Bans:     []ipBanResponse{ipBanToResponse(*row)},
			Total:    1,
			Page:     1,
			PageSize: 20,
		})
		return
	}

	pageResult, err := h.db.ListIPBansPaged(ctx, true, 1, 20)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	resp := listIPBansResponse{
		Bans:     make([]ipBanResponse, 0, len(pageResult.Bans)),
		Total:    pageResult.Total,
		Page:     pageResult.Page,
		PageSize: pageResult.PageSize,
	}
	for _, row := range pageResult.Bans {
		resp.Bans = append(resp.Bans, ipBanToResponse(row))
	}
	c.JSON(http.StatusOK, resp)
}

func buildPublicMaintenanceRoutes(maintenance proxy.APIMaintenanceConfig) []publicMaintenanceRouteResponse {
	maintenance = proxy.NormalizeAPIMaintenanceConfig(maintenance)

	routes := make([]publicMaintenanceRouteResponse, 0, len(publicMaintenancePaths))
	for _, path := range publicMaintenancePaths {
		route, ok := maintenance.Routes[path]
		routeInMaintenance := maintenance.Enabled && !(ok && route.Enabled != nil && !*route.Enabled)
		message := maintenance.Message
		if !routeInMaintenance {
			message = ""
		} else if ok && strings.TrimSpace(route.Message) != "" {
			message = strings.TrimSpace(route.Message)
		}
		routes = append(routes, publicMaintenanceRouteResponse{
			Path:        path,
			Message:     message,
			Maintenance: routeInMaintenance,
		})
	}
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Path < routes[j].Path
	})
	return routes
}

func buildPublicAccountPool(accounts []*auth.Account) publicAccountPoolResponse {
	planStats := map[string]*publicAccountPlanResponse{}
	for _, plan := range publicAccountPlanOrder() {
		stat := plan
		planStats[stat.Type] = &stat
	}

	result := publicAccountPoolResponse{
		Total: len(accounts),
	}

	for _, account := range accounts {
		if account == nil {
			continue
		}
		available := account.IsAvailable()
		status := strings.ToLower(strings.TrimSpace(account.RuntimeStatus()))
		planType, planLabel := publicAccountPlan(account.GetPlanType())
		plan, ok := planStats[planType]
		if !ok {
			plan = &publicAccountPlanResponse{Type: planType, Label: planLabel}
			planStats[planType] = plan
		}

		if available {
			result.Available++
			plan.Available++
		}
		if atomic.LoadInt32(&account.Disabled) != 0 || atomic.LoadInt32(&account.DispatchPaused) != 0 {
			result.Disabled++
		}
		switch status {
		case "unauthorized":
			result.Unauthorized++
		case "error":
			result.Error++
		case "refreshing":
			result.Refreshing++
		}
		if window := publicAccountRateLimitWindow(account, status); window != "" {
			result.RateLimited++
			plan.RateLimited++
			if window == "7d" {
				result.RateLimited7d++
			} else {
				result.RateLimited5h++
			}
		}
		plan.Total++
	}

	result.Plans = make([]publicAccountPlanResponse, 0, len(planStats))
	for _, plan := range publicAccountPlanOrder() {
		result.Plans = append(result.Plans, *planStats[plan.Type])
	}
	if other, ok := planStats["other"]; ok && other.Total > 0 {
		result.Plans = append(result.Plans, *other)
	}

	return result
}

func publicAccountPlanOrder() []publicAccountPlanResponse {
	return []publicAccountPlanResponse{
		{Type: "pro", Label: "Pro"},
		{Type: "prolite", Label: "ProLite"},
		{Type: "plus", Label: "Plus"},
		{Type: "team", Label: "Team"},
		{Type: "free", Label: "Free"},
		{Type: "api", Label: "Api"},
	}
}

func publicAccountPlan(plan string) (string, string) {
	raw := strings.ToLower(strings.TrimSpace(plan))
	switch {
	case raw == "prolite" || raw == "pro_lite" || raw == "pro-lite":
		return "prolite", "ProLite"
	case strings.Contains(raw, "team"):
		return "team", "Team"
	case strings.HasPrefix(raw, "pro"):
		return "pro", "Pro"
	case strings.Contains(raw, "plus"):
		return "plus", "Plus"
	case raw == "api":
		return "api", "Api"
	case raw == "" || raw == "free":
		return "free", "Free"
	default:
		return "other", "Other"
	}
}

func publicAccountRateLimitWindow(account *auth.Account, status string) string {
	reason, _ := account.GetCooldownSnapshot()
	reason = strings.ToLower(strings.TrimSpace(reason))
	explicitlyRateLimited := status == "rate_limited" ||
		status == "usage_exhausted" ||
		status == "rate_limited_5h" ||
		status == "rate_limited_7d" ||
		reason == "rate_limited" ||
		reason == "rate_limited_5h" ||
		reason == "rate_limited_7d"

	if status == "usage_exhausted" ||
		status == "rate_limited_7d" ||
		reason == "rate_limited_7d" ||
		publicAccountUsageWindowExhausted(account.GetUsagePercent7d, account.GetReset7dAt) {
		return "7d"
	}

	if status == "rate_limited_5h" ||
		reason == "rate_limited_5h" ||
		(publicAccountPremium5hPlan(account.GetPlanType()) && publicAccountUsageWindowExhausted(account.GetUsagePercent5h, account.GetReset5hAt)) {
		return "5h"
	}

	if explicitlyRateLimited {
		return "5h"
	}

	return ""
}

func publicAccountUsageWindowExhausted(
	getPercent func() (float64, bool),
	getResetAt func() time.Time,
) bool {
	percent, ok := getPercent()
	if !ok || percent < 100 {
		return false
	}
	resetAt := getResetAt()
	return resetAt.IsZero() || resetAt.After(time.Now())
}

func publicAccountPremium5hPlan(plan string) bool {
	switch auth.NormalizePlanType(plan) {
	case "plus", "pro", "team", "teamplus":
		return true
	default:
		return false
	}
}
