package httpapi

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteOKWrapsEnvelope(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteOK(rr, "req_1", map[string]string{"status": "ok"})

	if rr.Code != stdhttp.StatusOK {
		t.Fatalf("unexpected status: %d", rr.Code)
	}

	var got Envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if got.Code != "OK" {
		t.Fatalf("unexpected code: %s", got.Code)
	}
	if got.Message != "success" {
		t.Fatalf("unexpected message: %s", got.Message)
	}
	if got.RequestID != "req_1" {
		t.Fatalf("unexpected request id: %s", got.RequestID)
	}
	data, ok := got.Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected data type: %T", got.Data)
	}
	if data["status"] != "ok" {
		t.Fatalf("unexpected data.status: %#v", data["status"])
	}
}
