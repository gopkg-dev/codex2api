package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFilterPublicModelIDsKeepsGPTAndCodexAutoReview(t *testing.T) {
	got := filterPublicModelIDs([]string{
		"gpt-4.1",
		"claude-3",
		"codex-auto-review",
		"codex-auto-review-fast",
		"gpt-test",
	})

	want := []string{"codex-auto-review", "gpt-4.1", "gpt-test"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("filterPublicModelIDs() = %v, want %v", got, want)
	}
}

func TestCheckPublicModelsDetectsChatAndResponsesSupport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			writePublicModelCheckJSON(t, w, map[string]any{
				"data": []map[string]string{
					{"id": "gpt-ok"},
					{"id": "gpt-chat-only"},
					{"id": "other-model"},
					{"id": "codex-auto-review"},
				},
			})
		case "/v1/chat/completions":
			var req map[string]any
			decodePublicModelCheckJSON(t, r, &req)
			model := req["model"].(string)
			if model == "codex-auto-review" {
				http.Error(w, "chat disabled", http.StatusBadRequest)
				return
			}
			writePublicModelCheckJSON(t, w, map[string]any{
				"choices": []map[string]any{{"message": map[string]string{"content": "ok"}}},
			})
		case "/v1/responses":
			var req map[string]any
			decodePublicModelCheckJSON(t, r, &req)
			model := req["model"].(string)
			if model == "gpt-chat-only" {
				http.Error(w, "responses unsupported", http.StatusNotFound)
				return
			}
			writePublicModelCheckJSON(t, w, map[string]any{"output_text": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := checkPublicModels(context.Background(), http.DefaultClient, server.URL+"/v1", "test-key")
	if err != nil {
		t.Fatalf("checkPublicModels returned error: %v", err)
	}

	if strings.Join(result.ChatUsable, ",") != "gpt-chat-only,gpt-ok" {
		t.Fatalf("ChatUsable = %v", result.ChatUsable)
	}
	if strings.Join(result.ResponsesUsable, ",") != "codex-auto-review,gpt-ok" {
		t.Fatalf("ResponsesUsable = %v", result.ResponsesUsable)
	}
	if len(result.Rows) != 3 {
		t.Fatalf("Rows length = %d, want 3", len(result.Rows))
	}
}

func TestValidatePublicModelCheckBaseURLRequiresHTTPHost(t *testing.T) {
	for _, baseURL := range []string{"", "ftp://example.com/v1", "https:///v1"} {
		if err := validatePublicModelCheckBaseURL(baseURL); err == nil {
			t.Fatalf("validatePublicModelCheckBaseURL(%q) returned nil", baseURL)
		}
	}
	if err := validatePublicModelCheckBaseURL("https://example.com/v1"); err != nil {
		t.Fatalf("validatePublicModelCheckBaseURL valid URL: %v", err)
	}
}

func writePublicModelCheckJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func decodePublicModelCheckJSON(t *testing.T, r *http.Request, v any) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
		t.Fatalf("Authorization = %q", got)
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		t.Fatalf("decode request: %v", err)
	}
}
