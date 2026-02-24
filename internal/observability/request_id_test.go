package observability

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestNewRequestIDReturnsUUID(t *testing.T) {
	id := NewRequestID()
	if id == "" {
		t.Fatal("expected non-empty request id")
	}

	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("expected valid UUID, got %q: %v", id, err)
	}
}

func TestRequestIDContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	want := "abc-123"

	ctx = ContextWithRequestID(ctx, want)
	got := RequestIDFromContext(ctx)

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRequestIDFromContextWhenMissingOrWrongType(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		got := RequestIDFromContext(context.Background())
		if got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), RequestIDKey, 42)
		got := RequestIDFromContext(ctx)
		if got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})
}
