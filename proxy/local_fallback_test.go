package proxy

import "testing"

func TestIsLocalFallbackResponseDetectsKnownIDPaths(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "root id", payload: `{"id":"resp_local_abc"}`},
		{name: "response id", payload: `{"response":{"id":"RESP_LOCAL_ABC"}}`},
		{name: "item id", payload: `{"item":{"id":" msg_local_abc "}}`},
		{name: "response output id", payload: `{"response":{"output":[{"id":"msg_ok"},{"id":"msg_local_abc"}]}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !isLocalFallbackResponse([]byte(test.payload)) {
				t.Fatalf("isLocalFallbackResponse(%s) = false, want true", test.payload)
			}
		})
	}
}

func TestIsLocalFallbackResponseIgnoresNormalIDs(t *testing.T) {
	payload := []byte(`{"id":"resp_abc","response":{"id":"resp_abc","output":[{"id":"msg_abc"}]},"item":{"id":"item_abc"}}`)

	if isLocalFallbackResponse(payload) {
		t.Fatalf("isLocalFallbackResponse(%s) = true, want false", payload)
	}
}

func TestHasAnyResponseID(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    bool
	}{
		{name: "root id", payload: `{"id":"resp_abc"}`, want: true},
		{name: "response id", payload: `{"response":{"id":"resp_abc"}}`, want: true},
		{name: "item id", payload: `{"item":{"id":"msg_abc"}}`, want: true},
		{name: "output id", payload: `{"response":{"output":[{"id":"msg_abc"}]}}`, want: true},
		{name: "empty id", payload: `{"id":"   "}`, want: false},
		{name: "missing id", payload: `{"type":"response.output_text.delta","delta":"hello"}`, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := hasAnyResponseID([]byte(test.payload)); got != test.want {
				t.Fatalf("hasAnyResponseID(%s) = %t, want %t", test.payload, got, test.want)
			}
		})
	}
}
