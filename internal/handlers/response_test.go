package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go-chi-observability/internal/testutil"
)

func TestWriteErrorWritesStandardizedJSON(t *testing.T) {
	w := httptest.NewRecorder()

	WriteError(w, http.StatusBadRequest, "something went wrong")

	resp := w.Result()
	testutil.CheckResponseCode(t, http.StatusBadRequest, resp.StatusCode)

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	var body map[string]string
	testutil.DecodeJSONBody(t, resp.Body, &body)

	if got := body["error"]; got != "something went wrong" {
		t.Fatalf("expected error %q, got %q", "something went wrong", got)
	}

	if _, ok := body["request_id"]; ok {
		t.Fatal("did not expect request_id field in JSON body")
	}
}
