package proxy

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/codex2api/database"
)

const (
	ClientCompatModePreserve = "preserve"
	ClientCompatModeAuto     = "auto"
	ClientCompatModeForce    = "force"

	StreamFlushPolicyImmediate = "immediate"
	StreamFlushPolicyCoalesce  = "coalesce"

	defaultClientCompatMode      = ClientCompatModePreserve
	defaultCodexMinCLIVersion    = "0.118.0"
	defaultStreamFlushPolicy     = StreamFlushPolicyImmediate
	defaultStreamFlushIntervalMS = 20
	minStreamFlushIntervalMS     = 1
	maxStreamFlushIntervalMS     = 1000
)

type RuntimeSettings struct {
	ClientCompatMode                 string
	CodexMinCLIVersion               string
	StreamFlushPolicy                string
	StreamFlushIntervalMS            int
	FilterLocalFallbackResponse      bool
	DisableFastServiceTier           bool
	DownstreamUsageMultiplier        float64
	ProtocolMessageUsageBlastEnabled bool
	APIMaintenance                   APIMaintenanceConfig
}

var runtimeSettings atomic.Value // stores RuntimeSettings

func init() {
	runtimeSettings.Store(DefaultRuntimeSettings())
}

func DefaultRuntimeSettings() RuntimeSettings {
	return RuntimeSettings{
		ClientCompatMode:                 defaultClientCompatMode,
		CodexMinCLIVersion:               defaultCodexMinCLIVersion,
		StreamFlushPolicy:                defaultStreamFlushPolicy,
		StreamFlushIntervalMS:            defaultStreamFlushIntervalMS,
		FilterLocalFallbackResponse:      true,
		DisableFastServiceTier:           false,
		DownstreamUsageMultiplier:        1,
		ProtocolMessageUsageBlastEnabled: false,
		APIMaintenance:                   DefaultAPIMaintenanceConfig(),
	}
}

func NormalizeClientCompatMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ClientCompatModePreserve:
		return ClientCompatModePreserve
	case ClientCompatModeAuto:
		return ClientCompatModeAuto
	case ClientCompatModeForce:
		return ClientCompatModeForce
	default:
		return ClientCompatModePreserve
	}
}

func NormalizeStreamFlushPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", StreamFlushPolicyImmediate:
		return StreamFlushPolicyImmediate
	case StreamFlushPolicyCoalesce:
		return StreamFlushPolicyCoalesce
	default:
		return StreamFlushPolicyImmediate
	}
}

func NormalizeRuntimeSettings(settings RuntimeSettings) RuntimeSettings {
	defaults := DefaultRuntimeSettings()
	settings.ClientCompatMode = NormalizeClientCompatMode(settings.ClientCompatMode)
	settings.StreamFlushPolicy = NormalizeStreamFlushPolicy(settings.StreamFlushPolicy)
	if strings.TrimSpace(settings.CodexMinCLIVersion) == "" {
		settings.CodexMinCLIVersion = defaults.CodexMinCLIVersion
	} else {
		settings.CodexMinCLIVersion = strings.TrimSpace(settings.CodexMinCLIVersion)
	}
	if settings.StreamFlushIntervalMS < minStreamFlushIntervalMS {
		settings.StreamFlushIntervalMS = defaults.StreamFlushIntervalMS
	}
	if settings.StreamFlushIntervalMS > maxStreamFlushIntervalMS {
		settings.StreamFlushIntervalMS = maxStreamFlushIntervalMS
	}
	if settings.DownstreamUsageMultiplier <= 0 {
		settings.DownstreamUsageMultiplier = defaults.DownstreamUsageMultiplier
	}
	if settings.DownstreamUsageMultiplier > 1000 {
		settings.DownstreamUsageMultiplier = 1000
	}
	settings.APIMaintenance = NormalizeAPIMaintenanceConfig(settings.APIMaintenance)
	return settings
}

func ApplyRuntimeSettingsFromSystem(settings *database.SystemSettings) RuntimeSettings {
	next := DefaultRuntimeSettings()
	if settings != nil {
		next.ClientCompatMode = settings.ClientCompatMode
		next.CodexMinCLIVersion = settings.CodexMinCLIVersion
		next.StreamFlushPolicy = settings.StreamFlushPolicy
		next.StreamFlushIntervalMS = settings.StreamFlushIntervalMS
		next.FilterLocalFallbackResponse = settings.FilterLocalFallbackResponse
		next.DisableFastServiceTier = settings.DisableFastServiceTier
		next.DownstreamUsageMultiplier = settings.DownstreamUsageMultiplier
		next.ProtocolMessageUsageBlastEnabled = settings.ProtocolMessageUsageBlastEnabled
		next.APIMaintenance = ParseAPIMaintenanceConfig(settings.APIMaintenanceConfig)
	}
	next = NormalizeRuntimeSettings(next)
	runtimeSettings.Store(next)
	return next
}

func ApplyRuntimeSettings(settings RuntimeSettings) RuntimeSettings {
	settings = NormalizeRuntimeSettings(settings)
	runtimeSettings.Store(settings)
	return settings
}

func CurrentRuntimeSettings() RuntimeSettings {
	if v, ok := runtimeSettings.Load().(RuntimeSettings); ok {
		return NormalizeRuntimeSettings(v)
	}
	return DefaultRuntimeSettings()
}

func currentStreamFlushInterval() time.Duration {
	ms := CurrentRuntimeSettings().StreamFlushIntervalMS
	if ms < minStreamFlushIntervalMS {
		ms = defaultStreamFlushIntervalMS
	}
	return time.Duration(ms) * time.Millisecond
}
