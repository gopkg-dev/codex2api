package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/codex2api/auth"
	"github.com/codex2api/database"
	"github.com/gin-gonic/gin"
)

func TestFetchOpenAIResponsesModelIDsSupportsV1BaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("Authorization = %q, want Bearer sk-test", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4.1"},{"id":"gpt-4.1"},{"id":"gpt-4.1-mini"}]}`))
	}))
	defer server.Close()

	models, err := fetchOpenAIResponsesModelIDs(context.Background(), server.URL+"/v1", "sk-test", "")
	if err != nil {
		t.Fatalf("fetchOpenAIResponsesModelIDs returned error: %v", err)
	}
	want := []string{"gpt-4.1", "gpt-4.1-mini"}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %#v, want %#v", models, want)
	}
}

func TestConnectionTestModelForOpenAIResponsesAccountUsesFirstSupportedFallback(t *testing.T) {
	store := auth.NewStore(nil, nil, &database.SystemSettings{TestModel: "gpt-5.4"})
	handler := &Handler{store: store}
	account := &auth.Account{
		UpstreamType: auth.UpstreamOpenAIResponses,
		BaseURL:      "https://api.openai.com",
		APIKey:       "sk-test",
		Models:       []string{"gpt-4.1-mini", "gpt-4.1"},
	}

	model, err := handler.connectionTestModelForAccount(context.Background(), account, "")
	if err != nil {
		t.Fatalf("connectionTestModelForAccount returned error: %v", err)
	}
	if model != "gpt-4.1-mini" {
		t.Fatalf("model = %q, want first account model", model)
	}

	model, err = handler.connectionTestModelForAccount(context.Background(), account, "gpt-4.1")
	if err != nil {
		t.Fatalf("requested model returned error: %v", err)
	}
	if model != "gpt-4.1" {
		t.Fatalf("requested model = %q, want gpt-4.1", model)
	}
}

func TestAddOpenAIResponsesAccountsCreatesSequentialKeyNames(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "codex2api.db"))
	if err != nil {
		t.Fatalf("database.New returned error: %v", err)
	}
	defer db.Close()

	store := auth.NewStore(nil, nil, &database.SystemSettings{})
	handler := &Handler{db: db, store: store}
	body := `{
		"base_url":"https://example.test/v1",
		"api_keys":["sk-one","sk-two","sk-three"],
		"models":["gpt-5.5","gpt-5.4-mini"],
		"proxy_url":"socks5h://127.0.0.1:1080"
	}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/openai-responses/batch", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.AddOpenAIResponsesAccounts(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload createAccountsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Created != 3 || len(payload.IDs) != 3 {
		t.Fatalf("payload = %#v, want 3 created ids", payload)
	}

	rows, err := db.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive returned error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	for i, row := range rows {
		wantName := "key-" + strconv.Itoa(i+1)
		if row.Name != wantName {
			t.Fatalf("row %d name = %q, want %q", i, row.Name, wantName)
		}
		if row.ProxyURL != "socks5h://127.0.0.1:1080" {
			t.Fatalf("row %d proxy = %q, want shared proxy", i, row.ProxyURL)
		}
		if row.GetCredential("base_url") != "https://example.test/v1" {
			t.Fatalf("row %d base_url = %q, want normalized base URL", i, row.GetCredential("base_url"))
		}
		if got := row.GetCredentialStringSlice("models"); !reflect.DeepEqual(got, []string{"gpt-5.4-mini", "gpt-5.5"}) {
			t.Fatalf("row %d models = %#v", i, got)
		}
	}
	if got := len(store.Accounts()); got != 3 {
		t.Fatalf("store accounts = %d, want 3", got)
	}
}
