package observability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func TestRecordErrorWritesStandardizedErrorResponse(t *testing.T) {
	ctx := ContextWithRequestID(context.Background(), "req-1")
	span := trace.SpanFromContext(ctx)
	logger := zap.NewNop()

	counter, err := otel.Meter("test").Int64Counter("test.errors.total")
	if err != nil {
		t.Fatalf("creating counter: %v", err)
	}

	w := httptest.NewRecorder()

	RecordError(
		ctx,
		span,
		logger,
		counter,
		"add",
		"invalid request body",
		errors.New("bad json"),
		http.StatusBadRequest,
		w,
	)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}

	if got := body["error"]; got != "invalid request body" {
		t.Fatalf("expected error %q, got %q", "invalid request body", got)
	}

	if _, ok := body["request_id"]; ok {
		t.Fatal("did not expect request_id field in JSON body")
	}
}
