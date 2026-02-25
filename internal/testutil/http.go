package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func ExecuteRequest(req *http.Request, handler http.Handler) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func CheckResponseCode(t testing.TB, expected, actual int) {
	t.Helper()
	if expected != actual {
		t.Fatalf("expected status %d, got %d", expected, actual)
	}
}

func DecodeJSONBody(t testing.TB, body io.Reader, dst any) {
	t.Helper()
	if err := json.NewDecoder(body).Decode(dst); err != nil {
		t.Fatalf("decoding JSON response: %v", err)
	}
}
