package main

import (
	"context"

	"go-chi-observability/internal/calculator"
	"go-chi-observability/internal/observability"
)

// initMetrics initialises all metric providers and application-specific
// metric instruments. Add new domain InitMetrics calls here as the project grows.
func initMetrics(ctx context.Context) (func(context.Context) error, error) {
	shutdown, err := observability.InitMetrics(ctx)
	if err != nil {
		return nil, err
	}

	if err := calculator.InitMetrics(); err != nil {
		return nil, err
	}

	return shutdown, nil
}
