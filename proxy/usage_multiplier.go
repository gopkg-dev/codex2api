package proxy

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func downstreamUsageMultiplier(settings RuntimeSettings) float64 {
	settings = NormalizeRuntimeSettings(settings)
	if !settings.DownstreamUsageMultiplierEnabled || settings.DownstreamUsageMultiplier <= 0 {
		return 1
	}
	return settings.DownstreamUsageMultiplier
}

func scaleUsageNumber(value int, multiplier float64) int {
	if value == 0 || multiplier == 1 {
		return value
	}
	scaled := int(math.Round(float64(value) * multiplier))
	if scaled < 0 {
		return 0
	}
	return scaled
}

func scaleUsageInfoForDownstream(usage *UsageInfo, settings RuntimeSettings) *UsageInfo {
	multiplier := downstreamUsageMultiplier(settings)
	if usage == nil || multiplier == 1 {
		return usage
	}
	scaled := *usage
	scaled.CachedTokens = scaleUsageNumber(usage.CachedTokens, multiplier)
	if usage.PromptTokensDetails != nil {
		details := *usage.PromptTokensDetails
		details.CachedTokens = scaleUsageNumber(details.CachedTokens, multiplier)
		scaled.PromptTokensDetails = &details
	}
	if usage.InputTokensDetails != nil {
		details := *usage.InputTokensDetails
		details.CachedTokens = scaleUsageNumber(details.CachedTokens, multiplier)
		scaled.InputTokensDetails = &details
	}
	return &scaled
}

func scaleUsageRawForDownstream(raw []byte, settings RuntimeSettings) []byte {
	multiplier := downstreamUsageMultiplier(settings)
	if len(raw) == 0 || multiplier == 1 || !json.Valid(raw) {
		return raw
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return raw
	}
	scaled := scaleUsageValue(value, multiplier, "")
	out, err := json.Marshal(scaled)
	if err != nil {
		return raw
	}
	return out
}

func scaleDownstreamUsageJSON(raw []byte, settings RuntimeSettings) []byte {
	multiplier := downstreamUsageMultiplier(settings)
	if len(raw) == 0 || multiplier == 1 || !json.Valid(raw) {
		return raw
	}
	out := raw
	for _, path := range []string{"usage", "response.usage"} {
		usage := gjson.GetBytes(out, path)
		if !usage.Exists() || usage.Raw == "" || !json.Valid([]byte(usage.Raw)) {
			continue
		}
		scaled := scaleUsageRawForDownstream([]byte(usage.Raw), settings)
		if len(scaled) == 0 {
			continue
		}
		next, err := sjson.SetRawBytes(out, path, scaled)
		if err == nil {
			out = next
		}
	}
	return out
}

func scaleUsageValue(value any, multiplier float64, key string) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = scaleUsageValue(item, multiplier, key)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = scaleUsageValue(item, multiplier, key)
		}
		return out
	case json.Number:
		if !isCacheUsageKey(key) {
			return v
		}
		if integer, err := v.Int64(); err == nil {
			return int64(scaleUsageNumber(int(integer), multiplier))
		}
		if number, err := v.Float64(); err == nil {
			return number * multiplier
		}
		return v
	case float64:
		if !isCacheUsageKey(key) {
			return v
		}
		return v * multiplier
	case float32:
		if !isCacheUsageKey(key) {
			return v
		}
		return float64(v) * multiplier
	case int:
		if !isCacheUsageKey(key) {
			return v
		}
		return scaleUsageNumber(v, multiplier)
	case int64:
		if !isCacheUsageKey(key) {
			return v
		}
		return int64(scaleUsageNumber(int(v), multiplier))
	case int32:
		if !isCacheUsageKey(key) {
			return v
		}
		return int32(scaleUsageNumber(int(v), multiplier))
	default:
		return value
	}
}

func isCacheUsageKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return normalized == "cached_tokens" ||
		normalized == "cache_read_input_tokens" ||
		normalized == "cache_creation_input_tokens"
}

func scaleMaintenanceUsageForDownstream(usage maintenanceUsage, settings RuntimeSettings) maintenanceUsage {
	multiplier := downstreamUsageMultiplier(settings)
	if multiplier == 1 {
		return usage
	}
	usage.cached = min(scaleUsageNumber(usage.cached, multiplier), usage.input)
	return usage
}

func scaleAnthropicUsageForDownstream(usage anthropicUsage, settings RuntimeSettings) anthropicUsage {
	multiplier := downstreamUsageMultiplier(settings)
	if multiplier == 1 {
		return usage
	}
	usage.CacheCreationInputTokens = scaleUsageNumber(usage.CacheCreationInputTokens, multiplier)
	usage.CacheReadInputTokens = scaleUsageNumber(usage.CacheReadInputTokens, multiplier)
	return usage
}
