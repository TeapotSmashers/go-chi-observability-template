package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-chi-observability/internal/calculator"
	"go-chi-observability/internal/observability"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestNewRouterHealthEndpoint(t *testing.T) {
	router := NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if body := w.Body.String(); body != "ok" {
		t.Fatalf("expected body %q, got %q", "ok", body)
	}
}

func TestNewRouterCalculatorAddSetsHeaderAndOmitsRequestIDInBody(t *testing.T) {
	observability.Logger = zap.NewNop()
	if err := calculator.InitMetrics(); err != nil {
		t.Fatalf("initializing calculator metrics: %v", err)
	}

	router := NewRouter()
	body := []byte(`{"a":2,"b":3}`)
	req := httptest.NewRequest(http.MethodPost, "/calculator/add", bytes.NewReader(body))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	requestID := w.Result().Header.Get("X-Request-ID")
	if requestID == "" {
		t.Fatal("expected X-Request-ID header to be set")
	}
	if _, err := uuid.Parse(requestID); err != nil {
		t.Fatalf("expected valid UUID in X-Request-ID, got %q: %v", requestID, err)
	}

	var payload map[string]any
	if err := json.NewDecoder(w.Result().Body).Decode(&payload); err != nil {
		t.Fatalf("decoding JSON response: %v", err)
	}

	if _, ok := payload["request_id"]; ok {
		t.Fatal("did not expect request_id field in success JSON body")
	}

	if got, ok := payload["result"].(float64); !ok || got != 5 {
		t.Fatalf("expected result 5, got %#v", payload["result"])
	}
}
