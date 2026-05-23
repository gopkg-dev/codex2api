package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type publicModelCheckRequest struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

type publicModelCheckResponse struct {
	BaseURL         string                `json:"base_url"`
	Rows            []publicModelCheckRow `json:"rows"`
	ChatUsable      []string              `json:"chat_usable"`
	ResponsesUsable []string              `json:"responses_usable"`
}

type publicModelCheckRow struct {
	Model     string                     `json:"model"`
	Chat      publicModelEndpointSupport `json:"chat"`
	Responses publicModelEndpointSupport `json:"responses"`
	UsableAny bool                       `json:"usable_any"`
	UsableAll bool                       `json:"usable_all"`
}

type publicModelEndpointSupport struct {
	OK     bool   `json:"ok"`
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

type publicModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// PostPublicModelCheck 检测 OpenAI 兼容服务中的模型是否支持 Chat 和 Responses。
func (h *Handler) PostPublicModelCheck(c *gin.Context) {
	var req publicModelCheckRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	req.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	req.APIKey = strings.TrimSpace(req.APIKey)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()

	result, err := checkPublicModels(ctx, &http.Client{Timeout: 45 * time.Second}, req.BaseURL, req.APIKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func checkPublicModels(ctx context.Context, client *http.Client, baseURL, apiKey string) (publicModelCheckResponse, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return publicModelCheckResponse{}, errors.New("base url is required")
	}
	if apiKey == "" {
		return publicModelCheckResponse{}, errors.New("api key is required")
	}
	if err := validatePublicModelCheckBaseURL(baseURL); err != nil {
		return publicModelCheckResponse{}, err
	}
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}

	models, err := fetchPublicModelIDs(ctx, client, baseURL, apiKey)
	if err != nil {
		return publicModelCheckResponse{}, err
	}
	ids := filterPublicModelIDs(models)
	result := publicModelCheckResponse{
		BaseURL:         baseURL,
		ChatUsable:      []string{},
		ResponsesUsable: []string{},
		Rows:            make([]publicModelCheckRow, len(ids)),
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			chat := checkPublicChatModel(ctx, client, baseURL, apiKey, id)
			responses := checkPublicResponsesModel(ctx, client, baseURL, apiKey, id)
			result.Rows[i] = publicModelCheckRow{
				Model:     id,
				Chat:      chat,
				Responses: responses,
				UsableAny: chat.OK || responses.OK,
				UsableAll: chat.OK && responses.OK,
			}
		}(i, id)
	}
	wg.Wait()

	for _, row := range result.Rows {
		if row.Chat.OK {
			result.ChatUsable = append(result.ChatUsable, row.Model)
		}
		if row.Responses.OK {
			result.ResponsesUsable = append(result.ResponsesUsable, row.Model)
		}
	}
	return result, nil
}

func validatePublicModelCheckBaseURL(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid base url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("base url must use http or https")
	}
	if parsed.Host == "" {
		return errors.New("base url host is required")
	}
	return nil
}

func fetchPublicModelIDs(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("models request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("models request returned %d: %s", resp.StatusCode, summarizePublicModelCheckBody(body))
	}

	var decoded publicModelsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}
	models := make([]string, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		if item.ID != "" {
			models = append(models, item.ID)
		}
	}
	return models, nil
}

func filterPublicModelIDs(models []string) []string {
	seen := map[string]bool{}
	filtered := make([]string, 0, len(models))
	for _, model := range models {
		if strings.HasPrefix(model, "gpt-") || model == "codex-auto-review" {
			if !seen[model] {
				filtered = append(filtered, model)
				seen[model] = true
			}
		}
	}
	sort.Strings(filtered)
	return filtered
}

func checkPublicChatModel(ctx context.Context, client *http.Client, baseURL, apiKey, model string) publicModelEndpointSupport {
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with ok."},
		},
		"max_tokens": 8,
	}
	return postPublicModelCheckJSON(ctx, client, baseURL+"/chat/completions", apiKey, payload)
}

func checkPublicResponsesModel(ctx context.Context, client *http.Client, baseURL, apiKey, model string) publicModelEndpointSupport {
	payload := map[string]any{
		"model":             model,
		"input":             "Reply with ok.",
		"max_output_tokens": 8,
	}
	return postPublicModelCheckJSON(ctx, client, baseURL+"/responses", apiKey, payload)
}

func postPublicModelCheckJSON(ctx context.Context, client *http.Client, targetURL, apiKey string, payload any) publicModelEndpointSupport {
	body, err := json.Marshal(payload)
	if err != nil {
		return publicModelEndpointSupport{Error: err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return publicModelEndpointSupport{Error: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return publicModelEndpointSupport{Error: err.Error()}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	support := publicModelEndpointSupport{Status: resp.StatusCode}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		support.OK = true
		return support
	}
	support.Error = summarizePublicModelCheckBody(respBody)
	return support
}

func summarizePublicModelCheckBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "empty response"
	}
	var decoded map[string]any
	if json.Unmarshal(body, &decoded) == nil {
		if errObj, ok := decoded["error"].(map[string]any); ok {
			if msg, ok := errObj["message"].(string); ok && msg != "" {
				text = msg
			}
		}
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 220 {
		return text[:220] + "..."
	}
	return text
}
