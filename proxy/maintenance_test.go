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
	if got := gjson.Get(recorder.Body.String(), "choices.0.message.content").String(); got != "OpenAI Chat 接口未开启，系统维护中" {
		t.Fatalf("content = %q", got)
	}
}

func TestMaintenanceImageResponseIncludesServiceMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ApplyRuntimeSettings(RuntimeSettings{
		APIMaintenance: APIMaintenanceConfig{
			Enabled:      true,
			Message:      "系统维护中",
			ImageB64JSON: "image-data",
		},
	})
	defer ApplyRuntimeSettings(DefaultRuntimeSettings())

	router := gin.New()
	router.POST("/v1/images/generations", MaintenanceMiddleware(), func(c *gin.Context) {
		t.Fatal("handler should be short-circuited by maintenance middleware")
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"gpt-image-1"}`))
	router.ServeHTTP(recorder, req)

	if got := gjson.Get(recorder.Body.String(), "data.0.b64_json").String(); got != "image-data" {
		t.Fatalf("b64_json = %q", got)
	}
	if got := gjson.Get(recorder.Body.String(), "data.0.revised_prompt").String(); got != "GPT生图 接口未开启，系统维护中" {
		t.Fatalf("revised_prompt = %q", got)
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
	wantMessage := "Codex 接口未开启，你好！有什么我可以帮你的吗？"
	if got := gjson.Get(dataEvents[4], "delta").String(); got != wantMessage {
		t.Fatalf("delta = %q", got)
	}
	if got := gjson.Get(dataEvents[5], "text").String(); got != wantMessage {
		t.Fatalf("done text = %q", got)
	}
}

func TestMaintenanceSSEUsesSmallMessageUsage(t *testing.T) {
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
		t.Fatal("chat handler should be short-circuited by maintenance middleware")
	})
	router.POST("/v1/responses", MaintenanceMiddleware(), func(c *gin.Context) {
		t.Fatal("responses handler should be short-circuited by maintenance middleware")
	})
	router.POST("/v1/messages", MaintenanceMiddleware(), func(c *gin.Context) {
		t.Fatal("messages handler should be short-circuited by maintenance middleware")
	})

	chatEvents := parseSSEDataEvents(t, serveMaintenanceStream(t, router, "/v1/chat/completions"))
	chatUsage := gjson.Get(lastJSONDataEvent(t, chatEvents), "usage")
	assertMaintenanceUsage(t, chatUsage, "prompt_tokens", "completion_tokens", "total_tokens", 0, int64(len([]rune("OpenAI Chat 接口未开启，系统维护中"))))

	responseEvents := parseSSEDataEvents(t, serveMaintenanceStream(t, router, "/v1/responses"))
	responseUsage := gjson.Get(responseEvents[len(responseEvents)-1], "response.usage")
	assertMaintenanceUsage(t, responseUsage, "input_tokens", "output_tokens", "total_tokens", 0, int64(len([]rune("Codex 接口未开启，系统维护中"))))

	messageEvents := parseSSEDataEvents(t, serveMaintenanceStream(t, router, "/v1/messages"))
	messageStartUsage := gjson.Get(messageEvents[0], "message.usage")
	if input := messageStartUsage.Get("input_tokens").Int(); input != 0 {
		t.Fatalf("messages input_tokens = %d, want 0", input)
	}
	messageDeltaUsage := gjson.Get(messageEvents[len(messageEvents)-2], "usage")
	wantMessagesUsage := int64(len([]rune("Claude 接口未开启，系统维护中")))
	if output := messageDeltaUsage.Get("output_tokens").Int(); output != wantMessagesUsage {
		t.Fatalf("messages output_tokens = %d, want %d", output, wantMessagesUsage)
	}
}

func TestMaintenanceSSEAppliesProtocolMessageBlastMultiplier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ApplyRuntimeSettings(RuntimeSettings{
		ProtocolMessageUsageBlastEnabled: true,
		APIMaintenance: APIMaintenanceConfig{
			Enabled: true,
			Message: "维护",
		},
	})
	defer ApplyRuntimeSettings(DefaultRuntimeSettings())

	router := gin.New()
	router.POST("/v1/responses", MaintenanceMiddleware(), func(c *gin.Context) {
		t.Fatal("responses handler should be short-circuited by maintenance middleware")
	})

	responseEvents := parseSSEDataEvents(t, serveMaintenanceStream(t, router, "/v1/responses"))
	responseUsage := gjson.Get(responseEvents[len(responseEvents)-1], "response.usage")
	wantOutput := int64(len([]rune("Codex 接口未开启，维护"))) * 99999
	assertMaintenanceUsage(t, responseUsage, "input_tokens", "output_tokens", "total_tokens", 0, wantOutput)
}

func TestResolveMaintenanceConfigUsesGlobalMessageWithServicePrefix(t *testing.T) {
	cfg := APIMaintenanceConfig{
		Enabled: true,
		Message: "全局维护",
		Routes: map[string]APIMaintenanceRouteConfig{
			"/v1/responses": {Message: "旧的单独文案"},
		},
	}

	resolved := resolveMaintenanceConfig("/v1/responses", cfg)
	if resolved.Message != "Codex 接口未开启，全局维护" {
		t.Fatalf("message = %q", resolved.Message)
	}
}

func TestNormalizeMaintenanceConfigMigratesLegacyGlobalSwitch(t *testing.T) {
	disabled := false
	legacyOff := NormalizeAPIMaintenanceConfig(APIMaintenanceConfig{
		Enabled: true,
		Routes: map[string]APIMaintenanceRouteConfig{
			"/v1/messages": {Enabled: &disabled},
		},
	})
	if !legacyOff.RoutesControlled {
		t.Fatal("RoutesControlled = false, want migrated per-route settings")
	}
	if !maintenanceRouteEnabled("/v1/chat/completions", legacyOff) {
		t.Fatal("chat route = available, want maintenance inherited from legacy enabled switch")
	}
	if maintenanceRouteEnabled("/v1/messages", legacyOff) {
		t.Fatal("messages route = maintenance, want legacy disabled override preserved")
	}

	legacyDisabled := NormalizeAPIMaintenanceConfig(APIMaintenanceConfig{
		Enabled: false,
		Routes: map[string]APIMaintenanceRouteConfig{
			"/v1/responses": {},
		},
	})
	for _, path := range maintenanceEndpointPaths {
		if maintenanceRouteEnabled(path, legacyDisabled) {
			t.Fatalf("route %s = maintenance, want available after legacy global switch off migration", path)
		}
	}
}

func TestParseMaintenanceConfigMigratesStoredLegacyGlobalSwitch(t *testing.T) {
	cfg := ParseAPIMaintenanceConfig(`{"enabled":true,"routes":{"/v1/messages":{"enabled":false}}}`)
	if !maintenanceRouteEnabled("/v1/chat/completions", cfg) {
		t.Fatal("legacy enabled route should remain in maintenance")
	}
	if maintenanceRouteEnabled("/v1/messages", cfg) {
		t.Fatal("legacy disabled route should remain available")
	}
}

func TestMaintenanceMiddlewareUsesPerRouteSwitches(t *testing.T) {
	enabled := true
	disabled := false
	cfg := NormalizeAPIMaintenanceConfig(APIMaintenanceConfig{
		RoutesControlled: true,
		Routes: map[string]APIMaintenanceRouteConfig{
			"/v1/chat/completions": {Enabled: &enabled},
			"/v1/responses":        {Enabled: &disabled},
		},
	})

	if !shouldApplyMaintenance(http.MethodPost, "/v1/chat/completions", cfg) {
		t.Fatal("chat maintenance = false, want enabled service switch")
	}
	if shouldApplyMaintenance(http.MethodPost, "/v1/responses", cfg) {
		t.Fatal("responses maintenance = true, want disabled service switch")
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
		Routes:       map[string]APIMaintenanceRouteConfig{"/v1/responses": {Enabled: &enabled, Message: "旧的单独文案"}},
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
	if !decoded.Enabled || !decoded.RoutesControlled || !decoded.SSERandomize || decoded.Message != "维护" || decoded.Routes["/v1/responses"].Message != "" {
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

func assertMaintenanceUsage(t *testing.T, usage gjson.Result, inputKey, outputKey, totalKey string, wantInput, wantOutput int64) {
	t.Helper()
	input := usage.Get(inputKey).Int()
	output := usage.Get(outputKey).Int()
	total := usage.Get(totalKey).Int()
	if input != wantInput {
		t.Fatalf("%s = %d, want %d; usage=%s", inputKey, input, wantInput, usage.Raw)
	}
	if output != wantOutput {
		t.Fatalf("%s = %d, want %d; usage=%s", outputKey, output, wantOutput, usage.Raw)
	}
	if total != input+output {
		t.Fatalf("%s = %d, want input+output %d; usage=%s", totalKey, total, input+output, usage.Raw)
	}
}

func lastJSONDataEvent(t *testing.T, events []string) string {
	t.Helper()
	for i := len(events) - 1; i >= 0; i-- {
		if gjson.Valid(events[i]) {
			return events[i]
		}
	}
	t.Fatalf("no JSON data event in %v", events)
	return ""
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
