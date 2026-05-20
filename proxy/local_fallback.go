package proxy

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

var errLocalFallbackResponse = errors.New(localFallbackErrorMessage)

const (
	localFallbackErrorMessage = "Upstream returned invalid local fallback response"
	localFallbackErrorType    = "upstream_invalid_response"
	localFallbackErrorCode    = "local_fallback_response"
)

func hasLocalID(value string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(value)), "_local_")
}

func isLocalFallbackResponse(payload []byte) bool {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return false
	}
	root := gjson.ParseBytes(payload)
	if hasLocalID(root.Get("id").String()) ||
		hasLocalID(root.Get("response.id").String()) ||
		hasLocalID(root.Get("item.id").String()) {
		return true
	}
	for _, item := range root.Get("response.output").Array() {
		if hasLocalID(item.Get("id").String()) {
			return true
		}
	}
	return false
}

func hasAnyResponseID(payload []byte) bool {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return false
	}
	root := gjson.ParseBytes(payload)
	for _, path := range []string{"id", "response.id", "item.id"} {
		if strings.TrimSpace(root.Get(path).String()) != "" {
			return true
		}
	}
	for _, item := range root.Get("response.output").Array() {
		if strings.TrimSpace(item.Get("id").String()) != "" {
			return true
		}
	}
	return false
}

func localFallbackErrorBody() gin.H {
	return gin.H{
		"error": gin.H{
			"message": localFallbackErrorMessage,
			"type":    localFallbackErrorType,
			"code":    localFallbackErrorCode,
		},
	}
}

func sendLocalFallbackError(c *gin.Context) {
	c.JSON(http.StatusBadGateway, localFallbackErrorBody())
}

func localFallbackStreamOutcome() streamOutcome {
	return streamOutcome{
		logStatusCode:  http.StatusBadGateway,
		failureKind:    localFallbackErrorCode,
		failureMessage: localFallbackErrorMessage,
		penalize:       true,
	}
}

type localFallbackStreamFilter struct {
	enabled  bool
	released bool
	buffered []string
}

func newLocalFallbackStreamFilter(enabled bool) *localFallbackStreamFilter {
	return &localFallbackStreamFilter{
		enabled:  enabled,
		released: !enabled,
		buffered: make([]string, 0, 4),
	}
}

func (f *localFallbackStreamFilter) observe(data []byte) bool {
	if f == nil || !f.enabled {
		return false
	}
	if isLocalFallbackResponse(data) {
		return true
	}
	if hasAnyResponseID(data) {
		f.released = true
	}
	return false
}

func (f *localFallbackStreamFilter) write(payload string, write func(string) error) error {
	if payload == "" {
		return nil
	}
	if f == nil || !f.enabled || f.released {
		return write(payload)
	}
	f.buffered = append(f.buffered, payload)
	return nil
}

func (f *localFallbackStreamFilter) flush(write func(string) error) error {
	if f == nil || len(f.buffered) == 0 {
		return nil
	}
	for _, payload := range f.buffered {
		if err := write(payload); err != nil {
			return err
		}
	}
	f.buffered = f.buffered[:0]
	return nil
}

func (f *localFallbackStreamFilter) flushIfReleased(write func(string) error) error {
	if f == nil || !f.released {
		return nil
	}
	return f.flush(write)
}

func (f *localFallbackStreamFilter) flushAll(write func(string) error) error {
	if f == nil {
		return nil
	}
	f.released = true
	return f.flush(write)
}
