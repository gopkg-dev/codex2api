package proxy

import (
	"bytes"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	defaultMaintenanceMessage  = "系统维护中，请稍后重试。"
	defaultMaintenanceImageB64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII="
)

var maintenanceRandomPunctuation = []rune{
	'。', '，', '、', '.', ',', '!',
}

var maintenanceRandomSymbols = []rune{
	'⚠', 'ℹ', '✦', '◆', '⏳', '🙂',
}

var maintenanceSimplifiedToTraditional = map[rune]rune{
	'系': '係',
	'统': '統',
	'维': '維',
	'护': '護',
	'预': '預',
	'计': '計',
	'钟': '鐘',
	'后': '後',
	'复': '復',
	'务': '務',
	'暂': '暫',
	'时': '時',
	'关': '關',
	'闭': '閉',
	'请': '請',
	'开': '開',
	'启': '啟',
	'将': '將',
	'会': '會',
	'对': '對',
	'为': '為',
	'这': '這',
	'个': '個',
	'题': '題',
	'发': '發',
	'现': '現',
	'处': '處',
	'无': '無',
	'过': '過',
	'滤': '濾',
	'输': '輸',
	'错': '錯',
	'误': '誤',
}

var maintenanceTraditionalToSimplified = buildMaintenanceTraditionalToSimplified()

type APIMaintenanceRouteConfig struct {
	Enabled      *bool  `json:"enabled,omitempty"`
	Message      string `json:"message,omitempty"`
	ImageB64JSON string `json:"image_b64_json,omitempty"`
}

type APIMaintenanceConfig struct {
	Enabled      bool                                 `json:"enabled"`
	Message      string                               `json:"message"`
	SSERandomize bool                                 `json:"sse_randomize"`
	ImageB64JSON string                               `json:"image_b64_json"`
	Routes       map[string]APIMaintenanceRouteConfig `json:"routes,omitempty"`
}

type maintenanceResolvedConfig struct {
	Message      string
	SSERandomize bool
	ImageB64JSON string
}

func DefaultAPIMaintenanceConfig() APIMaintenanceConfig {
	return APIMaintenanceConfig{
		Enabled:      false,
		Message:      defaultMaintenanceMessage,
		SSERandomize: false,
		ImageB64JSON: defaultMaintenanceImageB64,
		Routes:       map[string]APIMaintenanceRouteConfig{},
	}
}

func NormalizeAPIMaintenanceConfig(cfg APIMaintenanceConfig) APIMaintenanceConfig {
	defaults := DefaultAPIMaintenanceConfig()
	cfg.Message = strings.TrimSpace(cfg.Message)
	if cfg.Message == "" {
		cfg.Message = defaults.Message
	}
	cfg.ImageB64JSON = strings.TrimSpace(cfg.ImageB64JSON)
	if cfg.ImageB64JSON == "" {
		cfg.ImageB64JSON = defaults.ImageB64JSON
	}
	if cfg.Routes == nil {
		cfg.Routes = map[string]APIMaintenanceRouteConfig{}
	}
	normalizedRoutes := make(map[string]APIMaintenanceRouteConfig, len(cfg.Routes))
	for path, route := range cfg.Routes {
		key := canonicalMaintenancePath(path)
		if key == "" {
			continue
		}
		route.Message = strings.TrimSpace(route.Message)
		route.ImageB64JSON = strings.TrimSpace(route.ImageB64JSON)
		normalizedRoutes[key] = route
	}
	cfg.Routes = normalizedRoutes
	return cfg
}

func ParseAPIMaintenanceConfig(raw string) APIMaintenanceConfig {
	cfg := DefaultAPIMaintenanceConfig()
	if strings.TrimSpace(raw) == "" {
		return cfg
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return DefaultAPIMaintenanceConfig()
	}
	return NormalizeAPIMaintenanceConfig(cfg)
}

func EncodeAPIMaintenanceConfig(cfg APIMaintenanceConfig) string {
	cfg = NormalizeAPIMaintenanceConfig(cfg)
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func MaintenanceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := CurrentRuntimeSettings().APIMaintenance
		routePath := canonicalMaintenancePath(c.Request.URL.Path)
		if !shouldApplyMaintenance(c.Request.Method, routePath, cfg) {
			c.Next()
			return
		}

		rawBody, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(rawBody))
		model := strings.TrimSpace(gjson.GetBytes(rawBody, "model").String())
		if model == "" {
			model = "maintenance"
		}
		stream := gjson.GetBytes(rawBody, "stream").Bool()
		if strings.Contains(routePath, "/images/") {
			stream = false
		}
		resolved := resolveMaintenanceConfig(routePath, cfg)
		writeMaintenanceResponse(c, routePath, model, stream, resolved)
		c.Abort()
	}
}

func canonicalMaintenancePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if strings.HasPrefix(path, "/v1/") {
		return path
	}
	switch path {
	case "/chat/completions", "/responses", "/responses/compact", "/messages", "/images/generations", "/images/edits":
		return "/v1" + path
	case "/backend-api/codex/responses":
		return "/v1/responses"
	case "/backend-api/codex/responses/compact":
		return "/v1/responses/compact"
	default:
		return path
	}
}

func shouldApplyMaintenance(method, routePath string, cfg APIMaintenanceConfig) bool {
	if strings.ToUpper(strings.TrimSpace(method)) != http.MethodPost {
		return false
	}
	cfg = NormalizeAPIMaintenanceConfig(cfg)
	if !cfg.Enabled {
		return false
	}
	if !isMaintenanceEndpoint(routePath) {
		return false
	}
	if route, ok := cfg.Routes[routePath]; ok && route.Enabled != nil && !*route.Enabled {
		return false
	}
	return true
}

func isMaintenanceEndpoint(routePath string) bool {
	switch routePath {
	case "/v1/chat/completions", "/v1/responses", "/v1/responses/compact", "/v1/messages", "/v1/images/generations", "/v1/images/edits":
		return true
	default:
		return false
	}
}

func resolveMaintenanceConfig(routePath string, cfg APIMaintenanceConfig) maintenanceResolvedConfig {
	cfg = NormalizeAPIMaintenanceConfig(cfg)
	resolved := maintenanceResolvedConfig{
		Message:      cfg.Message,
		SSERandomize: cfg.SSERandomize,
		ImageB64JSON: cfg.ImageB64JSON,
	}
	if route, ok := cfg.Routes[routePath]; ok {
		if strings.TrimSpace(route.Message) != "" {
			resolved.Message = strings.TrimSpace(route.Message)
		}
		if strings.TrimSpace(route.ImageB64JSON) != "" {
			resolved.ImageB64JSON = strings.TrimSpace(route.ImageB64JSON)
		}
	}
	return resolved
}

func writeMaintenanceResponse(c *gin.Context, routePath, model string, stream bool, cfg maintenanceResolvedConfig) {
	switch routePath {
	case "/v1/chat/completions":
		if stream {
			writeMaintenanceChatStream(c, model, cfg)
			return
		}
		c.JSON(http.StatusOK, buildMaintenanceChatResponse(model, cfg.Message))
	case "/v1/responses", "/v1/responses/compact":
		if stream {
			writeMaintenanceResponsesStream(c, model, cfg)
			return
		}
		c.JSON(http.StatusOK, buildMaintenanceResponsesResponse(model, cfg.Message))
	case "/v1/messages":
		if stream {
			writeMaintenanceMessagesStream(c, model, cfg)
			return
		}
		c.JSON(http.StatusOK, buildMaintenanceMessagesResponse(model, cfg.Message))
	case "/v1/images/generations", "/v1/images/edits":
		c.JSON(http.StatusOK, gin.H{
			"created": time.Now().Unix(),
			"data": []gin.H{{
				"b64_json":       cfg.ImageB64JSON,
				"revised_prompt": "",
			}},
		})
	default:
		c.Next()
	}
}

func writeDisabledAPIKeyProtocolResponse(c *gin.Context, settings RuntimeSettings) bool {
	routePath := canonicalMaintenancePath(c.Request.URL.Path)
	if strings.ToUpper(strings.TrimSpace(c.Request.Method)) != http.MethodPost || !isMaintenanceEndpoint(routePath) {
		return false
	}

	rawBody, _ := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewReader(rawBody))
	model := strings.TrimSpace(gjson.GetBytes(rawBody, "model").String())
	if model == "" {
		model = "disabled_api_key"
	}
	stream := gjson.GetBytes(rawBody, "stream").Bool()
	if strings.Contains(routePath, "/images/") {
		stream = false
	}
	maintenanceCfg := NormalizeAPIMaintenanceConfig(settings.APIMaintenance)
	writeMaintenanceResponse(c, routePath, model, stream, maintenanceResolvedConfig{
		Message:      settings.APIKeyDisabledMessage,
		SSERandomize: maintenanceCfg.SSERandomize,
		ImageB64JSON: maintenanceCfg.ImageB64JSON,
	})
	return true
}

func buildMaintenanceChatResponse(model, message string) gin.H {
	return gin.H{
		"id":      maintenanceHexID("resp_"),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []gin.H{{
			"index": 0,
			"message": gin.H{
				"role":    "assistant",
				"content": message,
			},
			"finish_reason": "stop",
		}},
		"usage": gin.H{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
	}
}

func buildMaintenanceResponsesResponse(model, message string) gin.H {
	responseID := maintenanceHexID("resp_")
	itemID := maintenanceHexID("msg_")
	return gin.H{
		"id":         responseID,
		"object":     "response",
		"created_at": time.Now().Unix(),
		"status":     "completed",
		"model":      model,
		"output": []gin.H{{
			"id":     itemID,
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []gin.H{{
				"type":        "output_text",
				"text":        message,
				"annotations": []any{},
			}},
		}},
		"output_text": message,
		"usage":       nil,
	}
}

func buildMaintenanceMessagesResponse(model, message string) gin.H {
	return gin.H{
		"id":    maintenanceHexID("msg_"),
		"type":  "message",
		"role":  "assistant",
		"model": model,
		"content": []gin.H{{
			"type": "text",
			"text": message,
		}},
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage":         gin.H{"input_tokens": 0, "output_tokens": 0},
	}
}

func writeMaintenanceChatStream(c *gin.Context, model string, cfg maintenanceResolvedConfig) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	id := maintenanceHexID("chatcmpl_")
	created := time.Now().Unix()
	writeSSEData(c, gin.H{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []gin.H{{"index": 0, "delta": gin.H{"role": "assistant"}, "finish_reason": nil}}})
	for _, segment := range maintenanceStreamSegments(cfg.Message, cfg.SSERandomize) {
		writeSSEData(c, gin.H{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []gin.H{{"index": 0, "delta": gin.H{"content": segment}, "finish_reason": nil}}})
	}
	writeSSEData(c, gin.H{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []gin.H{{"index": 0, "delta": gin.H{}, "finish_reason": "stop"}}})
	fmt.Fprint(c.Writer, "data: [DONE]\n\n")
	flushGin(c)
}

func writeMaintenanceResponsesStream(c *gin.Context, model string, cfg maintenanceResolvedConfig) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	responseID := maintenanceHexID("resp_")
	itemID := maintenanceHexID("msg_")
	created := time.Now().Unix()
	sequenceNumber := 0
	inProgress := buildMaintenanceStreamResponseObject(responseID, model, created, "in_progress", nil)
	writeNamedSSE(c, "response.created", gin.H{"type": "response.created", "response": inProgress, "sequence_number": sequenceNumber})
	sequenceNumber++
	writeNamedSSE(c, "response.in_progress", gin.H{"type": "response.in_progress", "response": inProgress, "sequence_number": sequenceNumber})
	sequenceNumber++
	writeNamedSSE(c, "response.output_item.added", gin.H{
		"type":            "response.output_item.added",
		"item":            buildMaintenanceStreamMessageItem(itemID, "in_progress", nil),
		"output_index":    0,
		"sequence_number": sequenceNumber,
	})
	sequenceNumber++
	writeNamedSSE(c, "response.content_part.added", gin.H{
		"type":            "response.content_part.added",
		"content_index":   0,
		"item_id":         itemID,
		"output_index":    0,
		"part":            buildMaintenanceOutputTextPart(""),
		"sequence_number": sequenceNumber,
	})
	sequenceNumber++
	for _, segment := range maintenanceStreamSegments(cfg.Message, cfg.SSERandomize) {
		writeNamedSSE(c, "response.output_text.delta", gin.H{
			"type":            "response.output_text.delta",
			"content_index":   0,
			"delta":           segment,
			"item_id":         itemID,
			"logprobs":        []any{},
			"output_index":    0,
			"sequence_number": sequenceNumber,
		})
		sequenceNumber++
	}
	writeNamedSSE(c, "response.output_text.done", gin.H{
		"type":            "response.output_text.done",
		"content_index":   0,
		"item_id":         itemID,
		"logprobs":        []any{},
		"output_index":    0,
		"sequence_number": sequenceNumber,
		"text":            cfg.Message,
	})
	sequenceNumber++
	writeNamedSSE(c, "response.content_part.done", gin.H{
		"type":            "response.content_part.done",
		"content_index":   0,
		"item_id":         itemID,
		"output_index":    0,
		"part":            buildMaintenanceOutputTextPart(cfg.Message),
		"sequence_number": sequenceNumber,
	})
	sequenceNumber++
	writeNamedSSE(c, "response.output_item.done", gin.H{
		"type":            "response.output_item.done",
		"item":            buildMaintenanceStreamMessageItem(itemID, "completed", []gin.H{buildMaintenanceOutputTextPart(cfg.Message)}),
		"output_index":    0,
		"sequence_number": sequenceNumber,
	})
	sequenceNumber++
	writeNamedSSE(c, "response.completed", gin.H{
		"type":            "response.completed",
		"response":        buildMaintenanceStreamResponseObject(responseID, model, created, "completed", []gin.H{}),
		"sequence_number": sequenceNumber,
	})
	flushGin(c)
}

func writeMaintenanceMessagesStream(c *gin.Context, model string, cfg maintenanceResolvedConfig) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	messageID := maintenanceHexID("msg_")
	writeNamedSSE(c, "message_start", gin.H{"type": "message_start", "message": gin.H{"id": messageID, "type": "message", "role": "assistant", "model": model, "content": []any{}, "stop_reason": nil, "usage": gin.H{"input_tokens": 0, "output_tokens": 0}}})
	writeNamedSSE(c, "content_block_start", gin.H{"type": "content_block_start", "index": 0, "content_block": gin.H{"type": "text", "text": ""}})
	for _, segment := range maintenanceStreamSegments(cfg.Message, cfg.SSERandomize) {
		writeNamedSSE(c, "content_block_delta", gin.H{"type": "content_block_delta", "index": 0, "delta": gin.H{"type": "text_delta", "text": segment}})
	}
	writeNamedSSE(c, "content_block_stop", gin.H{"type": "content_block_stop", "index": 0})
	writeNamedSSE(c, "message_delta", gin.H{"type": "message_delta", "delta": gin.H{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": gin.H{"output_tokens": 0}})
	writeNamedSSE(c, "message_stop", gin.H{"type": "message_stop"})
	flushGin(c)
}

func writeSSEData(c *gin.Context, payload any) {
	raw, _ := json.Marshal(payload)
	fmt.Fprintf(c.Writer, "data: %s\n\n", raw)
	flushGin(c)
}

func writeNamedSSE(c *gin.Context, event string, payload any) {
	raw, _ := json.Marshal(payload)
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, raw)
	flushGin(c)
}

func flushGin(c *gin.Context) {
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func maintenanceHexID(prefix string) string {
	var raw [25]byte
	if _, err := crand.Read(raw[:]); err != nil {
		return prefix + strings.Repeat("0", 50)
	}
	return prefix + hex.EncodeToString(raw[:])
}

func buildMaintenanceStreamResponseObject(responseID, model string, created int64, status string, output []gin.H) gin.H {
	var completedAt any
	serviceTier := "auto"
	if status == "completed" {
		completedAt = created
		serviceTier = "default"
	}
	if output == nil {
		output = []gin.H{}
	}
	return gin.H{
		"id":                     responseID,
		"object":                 "response",
		"created_at":             created,
		"status":                 status,
		"background":             false,
		"completed_at":           completedAt,
		"error":                  nil,
		"frequency_penalty":      0.0,
		"incomplete_details":     nil,
		"instructions":           nil,
		"max_output_tokens":      nil,
		"max_tool_calls":         nil,
		"model":                  model,
		"moderation":             nil,
		"output":                 output,
		"parallel_tool_calls":    true,
		"presence_penalty":       0.0,
		"previous_response_id":   nil,
		"prompt_cache_key":       nil,
		"prompt_cache_retention": nil,
		"reasoning":              gin.H{"effort": nil, "summary": nil},
		"safety_identifier":      nil,
		"service_tier":           serviceTier,
		"store":                  false,
		"temperature":            1.0,
		"text":                   gin.H{"format": gin.H{"type": "text"}, "verbosity": "medium"},
		"tool_choice":            "auto",
		"tool_usage": gin.H{
			"image_gen":  gin.H{"input_tokens": 0, "input_tokens_details": gin.H{"image_tokens": 0, "text_tokens": 0}, "output_tokens": 0, "output_tokens_details": gin.H{"image_tokens": 0, "text_tokens": 0}, "total_tokens": 0},
			"web_search": gin.H{"num_requests": 0},
		},
		"tools":        []any{},
		"top_logprobs": 0,
		"top_p":        1.0,
		"truncation":   "disabled",
		"usage":        maintenanceStreamUsage(status),
		"user":         nil,
		"metadata":     gin.H{},
	}
}

func maintenanceStreamUsage(status string) any {
	if status != "completed" {
		return nil
	}
	return gin.H{
		"input_tokens": 0,
		"input_tokens_details": gin.H{
			"cached_tokens": 0,
		},
		"output_tokens": 0,
		"output_tokens_details": gin.H{
			"reasoning_tokens": 0,
		},
		"total_tokens": 0,
	}
}

func buildMaintenanceOutputTextPart(text string) gin.H {
	return gin.H{
		"type":        "output_text",
		"annotations": []any{},
		"logprobs":    []any{},
		"text":        text,
	}
}

func buildMaintenanceStreamMessageItem(itemID, status string, content []gin.H) gin.H {
	if content == nil {
		content = []gin.H{}
	}
	return gin.H{
		"id":      itemID,
		"type":    "message",
		"status":  status,
		"content": content,
		"phase":   "final_answer",
		"role":    "assistant",
	}
}

func maintenanceStreamSegments(message string, randomize bool) []string {
	return maintenanceStreamSegmentsWithRand(message, randomize, newMaintenanceRand())
}

func maintenanceStreamSegmentsWithRand(message string, randomize bool, rng *mrand.Rand) []string {
	if message == "" {
		return []string{""}
	}
	if !randomize {
		return []string{message}
	}
	if rng == nil {
		rng = newMaintenanceRand()
	}
	runes := []rune(message)
	segments := make([]string, 0, len(runes))
	var current strings.Builder
	for _, r := range runes {
		r = maintenanceMaybeTraditionalRune(r, rng)
		current.WriteRune(r)
		if rng.Intn(3) == 0 {
			current.WriteRune(' ')
		}
		if rng.Intn(8) == 0 {
			current.WriteRune(maintenanceRandomPunctuation[rng.Intn(len(maintenanceRandomPunctuation))])
		}
		if rng.Intn(12) == 0 {
			current.WriteRune(maintenanceRandomSymbols[rng.Intn(len(maintenanceRandomSymbols))])
		}
		if current.Len() > 0 && rng.Intn(2) == 0 {
			segments = append(segments, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		segments = append(segments, current.String())
	}
	if len(segments) == 0 {
		return []string{message}
	}
	return segments
}

func maintenanceMaybeTraditionalRune(r rune, rng *mrand.Rand) rune {
	traditional, ok := maintenanceSimplifiedToTraditional[r]
	if !ok || rng.Intn(2) == 0 {
		return r
	}
	return traditional
}

func newMaintenanceRand() *mrand.Rand {
	var seedBytes [8]byte
	if _, err := crand.Read(seedBytes[:]); err != nil {
		return mrand.New(mrand.NewSource(time.Now().UnixNano()))
	}
	seed := int64(binary.LittleEndian.Uint64(seedBytes[:]))
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return mrand.New(mrand.NewSource(seed))
}

func buildMaintenanceTraditionalToSimplified() map[rune]rune {
	result := make(map[rune]rune, len(maintenanceSimplifiedToTraditional))
	for simplified, traditional := range maintenanceSimplifiedToTraditional {
		result[traditional] = simplified
	}
	return result
}
