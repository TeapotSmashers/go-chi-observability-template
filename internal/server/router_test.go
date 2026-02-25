package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go-chi-observability/internal/calculator"
	"go-chi-observability/internal/observability"
	"go-chi-observability/internal/testutil"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

var setupRouterTestsOnce sync.Once

func setupRouterTests(t *testing.T) {
	t.Helper()

	setupRouterTestsOnce.Do(func() {
		observability.Logger = zap.NewNop()
		if err := calculator.InitMetrics(); err != nil {
			t.Fatalf("initializing calculator metrics: %v", err)
		}
	})
}

func TestNewRouterHealthEndpoint(t *testing.T) {
	router := NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := testutil.ExecuteRequest(req, router)

	testutil.CheckResponseCode(t, http.StatusOK, w.Code)

	if body := w.Body.String(); body != "ok" {
		t.Fatalf("expected body %q, got %q", "ok", body)
	}
}

func TestNewRouterCalculatorAddSetsHeaderAndOmitsRequestIDInBody(t *testing.T) {
	setupRouterTests(t)

	router := NewRouter()
	body := []byte(`{"a":2,"b":3}`)
	req := httptest.NewRequest(http.MethodPost, "/calculator/add", bytes.NewReader(body))
	w := testutil.ExecuteRequest(req, router)

	testutil.CheckResponseCode(t, http.StatusOK, w.Code)

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

func TestNewRouterCalculatorBinaryOperations(t *testing.T) {
	setupRouterTests(t)
	router := NewRouter()

	tests := []struct {
		name      string
		path      string
		body      string
		operation string
		result    float64
	}{
		{name: "add", path: "/calculator/add", body: `{"a":9,"b":3}`, operation: "add", result: 12},
		{name: "subtract", path: "/calculator/subtract", body: `{"a":9,"b":3}`, operation: "subtract", result: 6},
		{name: "multiply", path: "/calculator/multiply", body: `{"a":9,"b":3}`, operation: "multiply", result: 27},
		{name: "divide", path: "/calculator/divide", body: `{"a":9,"b":3}`, operation: "divide", result: 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			w := testutil.ExecuteRequest(req, router)

			testutil.CheckResponseCode(t, http.StatusOK, w.Code)

			requestID := w.Result().Header.Get("X-Request-ID")
			if requestID == "" {
				t.Fatal("expected X-Request-ID header to be set")
			}

			var payload map[string]any
			if err := json.NewDecoder(w.Result().Body).Decode(&payload); err != nil {
				t.Fatalf("decoding JSON response: %v", err)
			}

			if got := payload["operation"]; got != tc.operation {
				t.Fatalf("expected operation %q, got %#v", tc.operation, got)
			}

			if got := payload["result"]; got != tc.result {
				t.Fatalf("expected result %v, got %#v", tc.result, got)
			}
		})
	}
}

func TestNewRouterCalculatorDivideByZero(t *testing.T) {
	setupRouterTests(t)
	router := NewRouter()

	req := httptest.NewRequest(http.MethodPost, "/calculator/divide", strings.NewReader(`{"a":10,"b":0}`))
	w := testutil.ExecuteRequest(req, router)

	testutil.CheckResponseCode(t, http.StatusBadRequest, w.Code)

	var payload map[string]any
	if err := json.NewDecoder(w.Result().Body).Decode(&payload); err != nil {
		t.Fatalf("decoding JSON response: %v", err)
	}

	errText, ok := payload["error"].(string)
	if !ok {
		t.Fatalf("expected error field to be string, got %#v", payload["error"])
	}
	if !strings.Contains(errText, "division by zero") {
		t.Fatalf("expected divide-by-zero error, got %q", errText)
	}
}

func TestNewRouterCalculatorChain(t *testing.T) {
	setupRouterTests(t)
	router := NewRouter()

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/calculator/chain", strings.NewReader(`{"initial":10,"steps":[{"op":"add","value":5},{"op":"multiply","value":2},{"op":"subtract","value":4}]}`))
		w := testutil.ExecuteRequest(req, router)

		testutil.CheckResponseCode(t, http.StatusOK, w.Code)

		var payload map[string]any
		if err := json.NewDecoder(w.Result().Body).Decode(&payload); err != nil {
			t.Fatalf("decoding JSON response: %v", err)
		}

		if got := payload["result"]; got != 26.0 {
			t.Fatalf("expected result 26, got %#v", got)
		}
	})

	t.Run("unknown operation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/calculator/chain", strings.NewReader(`{"initial":10,"steps":[{"op":"pow","value":2}]}`))
		w := testutil.ExecuteRequest(req, router)

		testutil.CheckResponseCode(t, http.StatusBadRequest, w.Code)

		var payload map[string]any
		if err := json.NewDecoder(w.Result().Body).Decode(&payload); err != nil {
			t.Fatalf("decoding JSON response: %v", err)
		}

		errText, ok := payload["error"].(string)
		if !ok {
			t.Fatalf("expected error field to be string, got %#v", payload["error"])
		}
		if !strings.Contains(errText, "unknown operation") {
			t.Fatalf("expected unknown operation error, got %q", errText)
		}
	})
}
