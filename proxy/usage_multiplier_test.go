package proxy

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestScaleDownstreamUsageJSONKeepsOriginalBodyWhenDisabled(t *testing.T) {
	raw := []byte(`{"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120}}`)

	got := scaleDownstreamUsageJSON(raw, RuntimeSettings{
		DownstreamUsageMultiplierEnabled: false,
		DownstreamUsageMultiplier:        3,
	})

	if string(got) != string(raw) {
		t.Fatalf("disabled multiplier should keep body, got %s", got)
	}
}

func TestScaleDownstreamUsageJSONScalesNestedUsage(t *testing.T) {
	raw := []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":40},"output_tokens":20,"output_tokens_details":{"reasoning_tokens":8},"total_tokens":120}}}`)

	got := scaleDownstreamUsageJSON(raw, RuntimeSettings{
		DownstreamUsageMultiplierEnabled: true,
		DownstreamUsageMultiplier:        2.5,
	})

	if value := gjson.GetBytes(got, "response.usage.input_tokens").Int(); value != 250 {
		t.Fatalf("input_tokens = %d, want 250, body=%s", value, got)
	}
	if value := gjson.GetBytes(got, "response.usage.input_tokens_details.cached_tokens").Int(); value != 100 {
		t.Fatalf("cached_tokens = %d, want 100, body=%s", value, got)
	}
	if value := gjson.GetBytes(got, "response.usage.output_tokens_details.reasoning_tokens").Int(); value != 20 {
		t.Fatalf("reasoning_tokens = %d, want 20, body=%s", value, got)
	}
}

func TestBuildCompactResponseScalesUsageWithoutMutatingLogUsage(t *testing.T) {
	prev := CurrentRuntimeSettings()
	ApplyRuntimeSettings(RuntimeSettings{
		DownstreamUsageMultiplierEnabled: true,
		DownstreamUsageMultiplier:        2,
	})
	t.Cleanup(func() { ApplyRuntimeSettings(prev) })

	usage := newUsageInfo(100, 20, 8, 40)
	got := BuildCompactResponse("chatcmpl-test", "gpt-5.5", 123, "ok", nil, usage)

	if value := gjson.GetBytes(got, "usage.prompt_tokens").Int(); value != 200 {
		t.Fatalf("prompt_tokens = %d, want 200, body=%s", value, got)
	}
	if usage.PromptTokens != 100 || usage.InputTokensDetails.CachedTokens != 40 {
		t.Fatalf("original usage mutated: %+v", usage)
	}
}
