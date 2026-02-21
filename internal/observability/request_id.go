package observability

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

func NewRequestID() string {
	return uuid.New().String()
}

func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, RequestIDKey, id)
}

func RequestIDFromContext(ctx context.Context) string {
	id, ok := ctx.Value(RequestIDKey).(string)
	if !ok {
		return ""
	}
	return id
}
