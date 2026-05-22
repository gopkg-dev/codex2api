package admin

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

type cpuSampler struct {
	mu        sync.Mutex
	lastTotal uint64
	lastIdle  uint64
	hasLast   bool
}

func newCPUSampler() *cpuSampler {
	return &cpuSampler{}
}

func (s *cpuSampler) Sample() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	total, idle, err := readLinuxCPUTicks()
	if err != nil {
		return 0
	}

	if !s.hasLast {
		s.lastTotal = total
		s.lastIdle = idle
		s.hasLast = true

		time.Sleep(120 * time.Millisecond)
		total, idle, err = readLinuxCPUTicks()
		if err != nil {
			return 0
		}
	}

	totalDelta := total - s.lastTotal
	idleDelta := idle - s.lastIdle
	s.lastTotal = total
	s.lastIdle = idle

	if totalDelta == 0 {
		return 0
	}

	busy := float64(totalDelta-idleDelta) / float64(totalDelta) * 100
	if busy < 0 {
		return 0
	}
	if busy > 100 {
		return 100
	}
	return busy
}

func readLinuxCPUTicks() (uint64, uint64, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0, 0, scanner.Err()
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, nil
	}

	var total uint64
	for _, field := range fields[1:] {
		v, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		total += v
	}

	idle, err := strconv.ParseUint(fields[4], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	return total, idle, nil
}

func readSystemMemory() (usedBytes uint64, totalBytes uint64, percent float64) {
	if runtime.GOOS == "linux" {
		file, err := os.Open("/proc/meminfo")
		if err == nil {
			defer file.Close()

			var totalKB uint64
			var availableKB uint64
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "MemTotal:") {
					totalKB = parseMeminfoKB(line)
				}
				if strings.HasPrefix(line, "MemAvailable:") {
					availableKB = parseMeminfoKB(line)
				}
			}
			if totalKB > 0 {
				totalBytes = totalKB * 1024
				usedBytes = (totalKB - availableKB) * 1024
				percent = float64(usedBytes) / float64(totalBytes) * 100
				return usedBytes, totalBytes, percent
			}
		}
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	usedBytes = mem.Alloc
	totalBytes = mem.Sys
	if totalBytes > 0 {
		percent = float64(usedBytes) / float64(totalBytes) * 100
	}

	return usedBytes, totalBytes, percent
}

func parseMeminfoKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}

	v, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}

	return v
}

func parseIPStatsWindowStart(value string, now time.Time) time.Time {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1m":
		return now.Add(-1 * time.Minute)
	case "15m":
		return now.Add(-15 * time.Minute)
	case "1h":
		return now.Add(-1 * time.Hour)
	case "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	default:
		return now.Add(-5 * time.Minute)
	}
}

func (h *Handler) buildOpsOverview(ctx context.Context, ipWindowStart time.Time) (opsOverviewResponse, error) {
	usageStats, err := h.getUsageStatsCached(ctx)
	if err != nil {
		return opsOverviewResponse{}, err
	}

	trafficSnapshot, err := h.db.GetTrafficSnapshot(ctx)
	if err != nil {
		return opsOverviewResponse{}, err
	}

	ipStats, err := h.db.GetIPUsageStats(ctx, 8, ipWindowStart)
	if err != nil {
		return opsOverviewResponse{}, err
	}
	ipStatsTotal, err := h.db.CountIPUsageStats(ctx)
	if err != nil {
		return opsOverviewResponse{}, err
	}
	settings, err := h.db.GetSystemSettings(ctx)
	if err != nil {
		return opsOverviewResponse{}, err
	}
	ipRPMLimit := 0
	ipQPSLimit := 0
	if settings != nil {
		ipRPMLimit = settings.IPRPMLimit
		ipQPSLimit = settings.IPQPSLimit
	}
	activeBans, err := h.db.ListIPBans(ctx, false)
	if err != nil {
		return opsOverviewResponse{}, err
	}
	classifyIPStatStatuses(ipStats, ipRPMLimit, ipQPSLimit, activeIPBanSet(activeBans))

	dbHealthy := h.db.Ping(ctx) == nil
	dbStats := h.db.Stats()
	dbUsage := 0.0
	if dbStats.MaxOpenConnections > 0 {
		dbUsage = float64(dbStats.OpenConnections) / float64(dbStats.MaxOpenConnections) * 100
	}

	redisHealthy := h.cache != nil && h.cache.Ping(ctx) == nil
	var redisTotal uint32
	var redisIdle uint32
	var redisStale uint32
	var redisPoolSize int
	var redisUsage float64
	if h.cache != nil {
		poolStats := h.cache.Stats()
		redisTotal = poolStats.TotalConns
		redisIdle = poolStats.IdleConns
		redisStale = poolStats.StaleConns
		redisPoolSize = h.cache.PoolSize()

		activeRedis := int(redisTotal) - int(redisIdle) - int(redisStale)
		if activeRedis < 0 {
			activeRedis = 0
		}
		if redisPoolSize > 0 {
			redisUsage = float64(activeRedis) / float64(redisPoolSize) * 100
		}
	}

	usedMemory, totalMemory, memoryPercent := readSystemMemory()
	cpuPercent := h.cpuSampler.Sample()

	var processMemory uint64
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	processMemory = memStats.Sys

	var activeRequests int64
	var totalRuntimeRequests int64
	for _, acc := range h.store.Accounts() {
		activeRequests += acc.GetActiveRequests()
		totalRuntimeRequests += acc.GetTotalRequests()
	}

	rxBytes, txBytes := readSystemNetworkBytes()

	return opsOverviewResponse{
		UpdatedAt:      time.Now().Format(time.RFC3339),
		UptimeSeconds:  int64(time.Since(h.startedAt).Seconds()),
		DatabaseDriver: h.databaseDriver,
		DatabaseLabel:  h.databaseLabel,
		CacheDriver:    h.cacheDriver,
		CacheLabel:     h.cacheLabel,
		CPU: opsCPUResponse{
			Percent: cpuPercent,
			Cores:   runtime.NumCPU(),
		},
		Memory: opsMemoryResponse{
			Percent:      memoryPercent,
			UsedBytes:    usedMemory,
			TotalBytes:   totalMemory,
			ProcessBytes: processMemory,
		},
		Runtime: opsRuntimeResponse{
			Goroutines:        runtime.NumGoroutine(),
			AvailableAccounts: h.store.AvailableCount(),
			TotalAccounts:     h.store.AccountCount(),
		},
		Requests: opsRequestsResponse{
			Active: activeRequests,
			Total:  totalRuntimeRequests,
		},
		Postgres: opsDatabaseResponse{
			Healthy:      dbHealthy,
			Open:         dbStats.OpenConnections,
			InUse:        dbStats.InUse,
			Idle:         dbStats.Idle,
			MaxOpen:      dbStats.MaxOpenConnections,
			WaitCount:    dbStats.WaitCount,
			UsagePercent: dbUsage,
		},
		Redis: opsRedisResponse{
			Healthy:      redisHealthy,
			TotalConns:   redisTotal,
			IdleConns:    redisIdle,
			StaleConns:   redisStale,
			PoolSize:     redisPoolSize,
			UsagePercent: redisUsage,
		},
		Traffic: opsTrafficResponse{
			QPS:           trafficSnapshot.QPS,
			QPSPeak:       trafficSnapshot.QPSPeak,
			TPS:           trafficSnapshot.TPS,
			TPSPeak:       trafficSnapshot.TPSPeak,
			RPM:           usageStats.RPM,
			TPM:           usageStats.TPM,
			ErrorRate:     usageStats.ErrorRate,
			TodayRequests: usageStats.TodayRequests,
			TodayTokens:   usageStats.TodayTokens,
			RPMLimit:      h.rateLimiter.GetRPM(),
			AvgDurationMs: usageStats.AvgDurationMs,
		},
		Network: opsNetworkResponse{
			RxBytes:    rxBytes,
			TxBytes:    txBytes,
			TotalBytes: rxBytes + txBytes,
		},
		IPStats:      ipStats,
		IPStatsTotal: ipStatsTotal,
	}, nil
}

func readSystemNetworkBytes() (rxBytes uint64, txBytes uint64) {
	if runtime.GOOS != "linux" {
		return 0, 0
	}

	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		if iface == "" || iface == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}
		rx, rxErr := strconv.ParseUint(fields[0], 10, 64)
		tx, txErr := strconv.ParseUint(fields[8], 10, 64)
		if rxErr == nil {
			rxBytes += rx
		}
		if txErr == nil {
			txBytes += tx
		}
	}

	return rxBytes, txBytes
}

func activeIPBanSet(rows []database.IPBanRow) map[string]struct{} {
	result := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		result[row.IP] = struct{}{}
	}
	return result
}

func classifyIPStatStatuses(stats []database.IPUsageStat, rpmLimit int, qpsLimit int, banned map[string]struct{}) {
	for i := range stats {
		stats[i].Status = classifyIPStatStatus(stats[i], rpmLimit, qpsLimit, banned)
	}
}

func classifyIPStatStatus(stat database.IPUsageStat, rpmLimit int, qpsLimit int, banned map[string]struct{}) string {
	if _, ok := banned[stat.IP]; ok {
		return "banned"
	}
	status := "normal"
	if rpmLimit > 0 {
		status = maxIPStatStatus(status, thresholdIPStatStatus(stat.RPM, float64(rpmLimit)))
	}
	if qpsLimit > 0 {
		status = maxIPStatStatus(status, thresholdIPStatStatus(stat.QPS, float64(qpsLimit)))
	}
	return status
}

func thresholdIPStatStatus(value float64, limit float64) string {
	if value >= limit {
		return "abnormal"
	}
	if value >= limit*0.8 {
		return "watch"
	}
	return "normal"
}

func maxIPStatStatus(a string, b string) string {
	if ipStatStatusRank(b) > ipStatStatusRank(a) {
		return b
	}
	return a
}

func ipStatStatusRank(status string) int {
	switch status {
	case "banned":
		return 3
	case "abnormal":
		return 2
	case "watch":
		return 1
	default:
		return 0
	}
}

// GetOpsOverview 获取系统运维概览
func (h *Handler) GetOpsOverview(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	overview, err := h.buildOpsOverview(ctx, parseIPStatsWindowStart(c.Query("ip_window"), time.Now()))
	if err != nil {
		writeInternalError(c, fmt.Errorf("获取运维概览失败: %w", err))
		return
	}

	c.JSON(200, overview)
}
