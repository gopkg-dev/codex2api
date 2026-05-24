package proxy

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestScaleDownstreamUsageJSONKeepsOriginalBodyAtDefaultMultiplier(t *testing.T) {
	raw := []byte(`{"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120}}`)

	got := scaleDownstreamUsageJSON(raw, RuntimeSettings{
		DownstreamUsageMultiplier: 1,
	})

	if string(got) != string(raw) {
		t.Fatalf("default multiplier should keep body, got %s", got)
	}
}

func TestScaleDownstreamUsageJSONOnlyScalesCachedTokens(t *testing.T) {
	raw := []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":40},"output_tokens":20,"output_tokens_details":{"reasoning_tokens":8},"total_tokens":120}}}`)

	got := scaleDownstreamUsageJSON(raw, RuntimeSettings{
		DownstreamUsageMultiplier: 2.5,
	})

	if value := gjson.GetBytes(got, "response.usage.input_tokens").Int(); value != 100 {
		t.Fatalf("input_tokens = %d, want 100, body=%s", value, got)
	}
	if value := gjson.GetBytes(got, "response.usage.input_tokens_details.cached_tokens").Int(); value != 100 {
		t.Fatalf("cached_tokens = %d, want 100, body=%s", value, got)
	}
	if value := gjson.GetBytes(got, "response.usage.output_tokens").Int(); value != 20 {
		t.Fatalf("output_tokens = %d, want 20, body=%s", value, got)
	}
	if value := gjson.GetBytes(got, "response.usage.output_tokens_details.reasoning_tokens").Int(); value != 8 {
		t.Fatalf("reasoning_tokens = %d, want 8, body=%s", value, got)
	}
	if value := gjson.GetBytes(got, "response.usage.total_tokens").Int(); value != 120 {
		t.Fatalf("total_tokens = %d, want 120, body=%s", value, got)
	}
}

func TestBuildCompactResponseOnlyScalesCachedUsageWithoutMutatingLogUsage(t *testing.T) {
	prev := CurrentRuntimeSettings()
	ApplyRuntimeSettings(RuntimeSettings{
		DownstreamUsageMultiplier: 2,
	})
	t.Cleanup(func() { ApplyRuntimeSettings(prev) })

	usage := newUsageInfo(100, 20, 8, 40)
	got := BuildCompactResponse("chatcmpl-test", "gpt-5.5", 123, "ok", "", nil, usage)

	if value := gjson.GetBytes(got, "usage.prompt_tokens").Int(); value != 100 {
		t.Fatalf("prompt_tokens = %d, want 100, body=%s", value, got)
	}
	if value := gjson.GetBytes(got, "usage.prompt_tokens_details.cached_tokens").Int(); value != 80 {
		t.Fatalf("cached_tokens = %d, want 80, body=%s", value, got)
	}
	if usage.PromptTokens != 100 || usage.InputTokensDetails.CachedTokens != 40 {
		t.Fatalf("original usage mutated: %+v", usage)
	}
}

func TestScaleAnthropicUsageForDownstreamOnlyScalesCacheRead(t *testing.T) {
	got := scaleAnthropicUsageForDownstream(anthropicUsage{
		InputTokens:          100,
		OutputTokens:         20,
		CacheReadInputTokens: 40,
	}, RuntimeSettings{
		DownstreamUsageMultiplier: 3,
	})

	if got.InputTokens != 100 || got.OutputTokens != 20 {
		t.Fatalf("usage tokens changed: %+v", got)
	}
	if got.CacheReadInputTokens != 120 {
		t.Fatalf("cache_read_input_tokens = %d, want 120", got.CacheReadInputTokens)
	}
}
