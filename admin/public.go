package admin

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

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

	usage, err := h.getUsageStatsCached(ctx)
	if err != nil {
		writeInternalError(c, err)
		return
	}

	ops, err := h.buildOpsOverview(ctx)
	if err != nil {
		writeInternalError(c, err)
		return
	}

	maintenance := proxy.CurrentRuntimeSettings().APIMaintenance
	maintenanceRoutes := buildPublicMaintenanceRoutes(maintenance)
	latestKey, err := h.getLatestPublicAPIKey(ctx)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, publicHomeResponse{
		Status:    "ok",
		UpdatedAt: time.Now().Format(time.RFC3339),
		Usage:     usage,
		Ops:       ops,
		LatestKey: latestKey,
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

func buildPublicMaintenanceRoutes(maintenance proxy.APIMaintenanceConfig) []publicMaintenanceRouteResponse {
	maintenance = proxy.NormalizeAPIMaintenanceConfig(maintenance)
	if !maintenance.Enabled {
		return []publicMaintenanceRouteResponse{}
	}

	routes := make([]publicMaintenanceRouteResponse, 0, len(publicMaintenancePaths))
	for _, path := range publicMaintenancePaths {
		route, ok := maintenance.Routes[path]
		routeInMaintenance := !(ok && route.Enabled != nil && !*route.Enabled)
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
