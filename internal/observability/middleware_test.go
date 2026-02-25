package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go-chi-observability/internal/testutil"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRequestIDMiddlewareSetsHeaderAndContext(t *testing.T) {
	var ctxRequestID string

	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxRequestID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	r := httptest.NewRequest(http.MethodGet, "/calculator/add", nil)
	w := testutil.ExecuteRequest(r, h)

	headerRequestID := w.Result().Header.Get("X-Request-ID")
	if headerRequestID == "" {
		t.Fatal("expected X-Request-ID header to be set")
	}

	if _, err := uuid.Parse(headerRequestID); err != nil {
		t.Fatalf("expected header to contain UUID, got %q: %v", headerRequestID, err)
	}

	if ctxRequestID != headerRequestID {
		t.Fatalf("expected context request_id %q to match header %q", ctxRequestID, headerRequestID)
	}
}

func TestShouldTraceRequest(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/health", want: false},
		{path: "/metrics", want: false},
		{path: "/calculator/add", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			got := shouldTraceRequest(r)
			if got != tc.want {
				t.Fatalf("path %q: expected %t, got %t", tc.path, tc.want, got)
			}
		})
	}
}

func TestLoggingMiddlewareWritesCompletionLog(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	oldLogger := Logger
	Logger = zap.New(core)
	t.Cleanup(func() { Logger = oldLogger })

	h := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	r := httptest.NewRequest(http.MethodPost, "/calculator/add", nil)
	r = r.WithContext(ContextWithRequestID(r.Context(), "req-123"))
	_ = testutil.ExecuteRequest(r, h)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Message != "request completed" {
		t.Fatalf("expected message %q, got %q", "request completed", entry.Message)
	}

	fields := entry.ContextMap()
	if fields["method"] != http.MethodPost {
		t.Fatalf("expected method %q, got %#v", http.MethodPost, fields["method"])
	}
	if fields["path"] != "/calculator/add" {
		t.Fatalf("expected path %q, got %#v", "/calculator/add", fields["path"])
	}
	if fields["request_id"] != "req-123" {
		t.Fatalf("expected request_id %q, got %#v", "req-123", fields["request_id"])
	}
}
