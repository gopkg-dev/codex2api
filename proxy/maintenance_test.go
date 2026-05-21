package proxy

import (
	"encoding/json"
	mrand "math/rand"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func TestMaintenanceMiddlewareReturnsChatCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ApplyRuntimeSettings(RuntimeSettings{
		APIMaintenance: APIMaintenanceConfig{
			Enabled: true,
			Message: "系统维护中",
		},
	})
	defer ApplyRuntimeSettings(DefaultRuntimeSettings())

	router := gin.New()
	router.POST("/v1/chat/completions", MaintenanceMiddleware(), func(c *gin.Context) {
		t.Fatal("handler should be short-circuited by maintenance middleware")
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4"}`))
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := gjson.Get(recorder.Body.String(), "object").String(); got != "chat.completion" {
		t.Fatalf("object = %q, want chat.completion", got)
	}
	if got := gjson.Get(recorder.Body.String(), "choices.0.message.content").String(); got != "系统维护中" {
		t.Fatalf("content = %q", got)
	}
}

func TestMaintenanceMiddlewareLeavesModelsAlone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ApplyRuntimeSettings(RuntimeSettings{
		APIMaintenance: APIMaintenanceConfig{Enabled: true, Message: "维护"},
	})
	defer ApplyRuntimeSettings(DefaultRuntimeSettings())

	router := gin.New()
	router.GET("/v1/models", MaintenanceMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !gjson.Get(recorder.Body.String(), "ok").Bool() {
		t.Fatalf("/v1/models should pass through, body=%s", recorder.Body.String())
	}
}

func TestMaintenanceResponsesStreamMatchesResponsesEventShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ApplyRuntimeSettings(RuntimeSettings{
		APIMaintenance: APIMaintenanceConfig{
			Enabled: true,
			Message: "你好！有什么我可以帮你的吗？",
		},
	})
	defer ApplyRuntimeSettings(DefaultRuntimeSettings())

	router := gin.New()
	router.POST("/v1/responses", MaintenanceMiddleware(), func(c *gin.Context) {
		t.Fatal("handler should be short-circuited by maintenance middleware")
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","stream":true}`))
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, event := range []string{
		"response.created",
		"response.in_progress",
		"response.output_item.added",
		"response.content_part.added",
		"response.output_text.delta",
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done",
		"response.completed",
	} {
		if !strings.Contains(body, "event: "+event+"\n") {
			t.Fatalf("stream missing event %s:\n%s", event, body)
		}
	}

	dataEvents := parseSSEDataEvents(t, body)
	if len(dataEvents) < 9 {
		t.Fatalf("got %d data events, want at least 9; body=%s", len(dataEvents), body)
	}

	responseID := gjson.Get(dataEvents[0], "response.id").String()
	itemID := gjson.Get(dataEvents[2], "item.id").String()
	if !regexp.MustCompile(`^resp_[0-9a-f]{50}$`).MatchString(responseID) {
		t.Fatalf("response id = %q, want resp_ plus 50 lowercase hex chars", responseID)
	}
	if !regexp.MustCompile(`^msg_[0-9a-f]{50}$`).MatchString(itemID) {
		t.Fatalf("item id = %q, want msg_ plus 50 lowercase hex chars", itemID)
	}
	if got := gjson.Get(dataEvents[0], "response.model").String(); got != "gpt-5.5" {
		t.Fatalf("created model = %q, want gpt-5.5", got)
	}
	if got := gjson.Get(dataEvents[len(dataEvents)-1], "response.model").String(); got != "gpt-5.5" {
		t.Fatalf("completed model = %q, want gpt-5.5", got)
	}
	if got := gjson.Get(dataEvents[len(dataEvents)-1], "response.id").String(); got != responseID {
		t.Fatalf("completed response id = %q, want %q", got, responseID)
	}
	if got := gjson.Get(dataEvents[3], "item_id").String(); got != itemID {
		t.Fatalf("content_part.added item_id = %q, want %q", got, itemID)
	}
	if got := gjson.Get(dataEvents[4], "delta").String(); got != "你好！有什么我可以帮你的吗？" {
		t.Fatalf("delta = %q", got)
	}
	if got := gjson.Get(dataEvents[5], "text").String(); got != "你好！有什么我可以帮你的吗？" {
		t.Fatalf("done text = %q", got)
	}
}

func TestMaintenanceMiddlewareRouteOverrideDisablesMaintenance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	disabled := false
	ApplyRuntimeSettings(RuntimeSettings{
		APIMaintenance: APIMaintenanceConfig{
			Enabled: true,
			Message: "维护",
			Routes: map[string]APIMaintenanceRouteConfig{
				"/v1/responses": {Enabled: &disabled},
			},
		},
	})
	defer ApplyRuntimeSettings(DefaultRuntimeSettings())

	router := gin.New()
	router.POST("/v1/responses", MaintenanceMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusAccepted, gin.H{"passed": true})
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4"}`))
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
}

func TestMaintenanceNonStreamResponseUsesOpenAIStyleIDs(t *testing.T) {
	response := buildMaintenanceResponsesResponse("gpt-5.5", "维护")
	responseID, _ := response["id"].(string)
	output, _ := response["output"].([]gin.H)
	if !regexp.MustCompile(`^resp_[0-9a-f]{50}$`).MatchString(responseID) {
		t.Fatalf("response id = %q, want resp_ plus 50 lowercase hex chars", responseID)
	}
	if len(output) != 1 {
		t.Fatalf("output len = %d, want 1", len(output))
	}
	itemID, _ := output[0]["id"].(string)
	if !regexp.MustCompile(`^msg_[0-9a-f]{50}$`).MatchString(itemID) {
		t.Fatalf("item id = %q, want msg_ plus 50 lowercase hex chars", itemID)
	}
	if response["model"] != "gpt-5.5" {
		t.Fatalf("model = %#v, want gpt-5.5", response["model"])
	}
}

func TestMaintenanceStreamSegmentsKeepMessage(t *testing.T) {
	message := "系统维护中，请稍后重试。"
	segments := maintenanceStreamSegmentsWithRand(message, false, mrand.New(mrand.NewSource(1)))
	if len(segments) == 0 {
		t.Fatal("segments is empty")
	}
	if got := strings.Join(segments, ""); got != message {
		t.Fatalf("joined segments = %q, want %q", got, message)
	}
}

func TestMaintenanceRandomizedSegmentsAddTextPerturbations(t *testing.T) {
	message := "系统维护预计十分钟后恢复"
	segments := maintenanceStreamSegmentsWithRand(message, true, mrand.New(mrand.NewSource(4)))
	if len(segments) == 0 {
		t.Fatal("segments is empty")
	}
	got := strings.Join(segments, "")
	if got == message {
		t.Fatalf("randomized text = %q, want perturbations", got)
	}
	if !containsAnyRune(got, maintenanceTraditionalRunes()) {
		t.Fatalf("randomized text = %q, want at least one traditional Chinese rune", got)
	}
	if !containsAnyRune(got, append(maintenanceRandomPunctuation, maintenanceRandomSymbols...)) {
		t.Fatalf("randomized text = %q, want punctuation or decorative symbol", got)
	}
	normalizedMessage := strings.ReplaceAll(message, " ", "")
	if normalized := normalizeMaintenanceRandomizedText(got); !strings.Contains(normalized, normalizedMessage) {
		t.Fatalf("normalized randomized text = %q, want to contain %q; raw=%q", normalized, normalizedMessage, got)
	}
}

func TestMaintenanceRandomizedSegmentsCanUseEmojiDigits(t *testing.T) {
	message := "系统维护预计 1234567890 分钟后恢复"
	var got string
	for seed := int64(1); seed <= 50; seed++ {
		segments := maintenanceStreamSegmentsWithRand(message, true, mrand.New(mrand.NewSource(seed)))
		got = strings.Join(segments, "")
		if containsAnyString(got, maintenanceEmojiDigits()) {
			break
		}
	}
	if !containsAnyString(got, maintenanceEmojiDigits()) {
		t.Fatalf("randomized text = %q, want at least one emoji digit", got)
	}
	normalizedMessage := strings.ReplaceAll(message, " ", "")
	if normalized := normalizeMaintenanceRandomizedText(got); !strings.Contains(normalized, normalizedMessage) {
		t.Fatalf("normalized randomized text = %q, want to contain %q; raw=%q", normalized, normalizedMessage, got)
	}
}

func TestMaintenanceRandomizedSSEKeepsProtocolShapes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ApplyRuntimeSettings(RuntimeSettings{
		APIMaintenance: APIMaintenanceConfig{
			Enabled:      true,
			Message:      "系统维护预计十分钟后恢复",
			SSERandomize: true,
		},
	})
	defer ApplyRuntimeSettings(DefaultRuntimeSettings())

	router := gin.New()
	router.POST("/v1/chat/completions", MaintenanceMiddleware(), func(c *gin.Context) {
		t.Fatal("chat handler should be short-circuited by maintenance middleware")
	})
	router.POST("/v1/responses", MaintenanceMiddleware(), func(c *gin.Context) {
		t.Fatal("responses handler should be short-circuited by maintenance middleware")
	})
	router.POST("/v1/messages", MaintenanceMiddleware(), func(c *gin.Context) {
		t.Fatal("messages handler should be short-circuited by maintenance middleware")
	})

	chatBody := serveMaintenanceStream(t, router, "/v1/chat/completions")
	if !strings.Contains(chatBody, `"object":"chat.completion.chunk"`) || !strings.Contains(chatBody, `"delta"`) || !strings.Contains(chatBody, "data: [DONE]") {
		t.Fatalf("chat stream shape changed:\n%s", chatBody)
	}

	responsesBody := serveMaintenanceStream(t, router, "/v1/responses")
	for _, event := range []string{"response.created", "response.output_text.delta", "response.completed"} {
		if !strings.Contains(responsesBody, "event: "+event+"\n") {
			t.Fatalf("responses stream missing event %s:\n%s", event, responsesBody)
		}
	}

	messagesBody := serveMaintenanceStream(t, router, "/v1/messages")
	for _, event := range []string{"message_start", "content_block_delta", "message_stop"} {
		if !strings.Contains(messagesBody, "event: "+event+"\n") {
			t.Fatalf("messages stream missing event %s:\n%s", event, messagesBody)
		}
	}
}

func TestMarshalMaintenanceConfigRoundTrip(t *testing.T) {
	enabled := true
	cfg := NormalizeAPIMaintenanceConfig(APIMaintenanceConfig{
		Enabled:      true,
		Message:      "维护",
		SSERandomize: true,
		ImageB64JSON: "abc",
		Routes:       map[string]APIMaintenanceRouteConfig{"/v1/responses": {Enabled: &enabled, Message: "Responses 维护"}},
	})

	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded APIMaintenanceConfig
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	decoded = NormalizeAPIMaintenanceConfig(decoded)
	if !decoded.Enabled || !decoded.SSERandomize || decoded.Message != "维护" || decoded.Routes["/v1/responses"].Message != "Responses 维护" {
		t.Fatalf("decoded config = %#v", decoded)
	}
}

func serveMaintenanceStream(t *testing.T, router *gin.Engine, path string) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5.5","stream":true}`))
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want %d; body=%s", path, recorder.Code, http.StatusOK, recorder.Body.String())
	}
	return recorder.Body.String()
}

func containsAnyRune(s string, runes []rune) bool {
	for _, r := range s {
		for _, candidate := range runes {
			if r == candidate {
				return true
			}
		}
	}
	return false
}

func maintenanceTraditionalRunes() []rune {
	runes := make([]rune, 0, len(maintenanceSimplifiedToTraditional))
	for _, r := range maintenanceSimplifiedToTraditional {
		runes = append(runes, r)
	}
	return runes
}

func normalizeMaintenanceRandomizedText(s string) string {
	var out strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\ufe0f' || r == '\u20e3' || containsRune(maintenanceRandomPunctuation, r) || containsRune(maintenanceRandomSymbols, r) {
			continue
		}
		if simplified, ok := maintenanceTraditionalToSimplified[r]; ok {
			r = simplified
		}
		out.WriteRune(r)
	}
	return out.String()
}

func containsRune(runes []rune, target rune) bool {
	for _, r := range runes {
		if r == target {
			return true
		}
	}
	return false
}

func containsAnyString(s string, values []string) bool {
	for _, value := range values {
		if strings.Contains(s, value) {
			return true
		}
	}
	return false
}

func parseSSEDataEvents(t *testing.T, body string) []string {
	t.Helper()
	var events []string
	for _, block := range strings.Split(body, "\n\n") {
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "data: ") {
				events = append(events, strings.TrimPrefix(line, "data: "))
			}
		}
	}
	return events
}
