package calculator

import (
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Metric instruments — initialized once via InitMetrics().
var (
	opsCounter   metric.Int64Counter
	opsHistogram metric.Float64Histogram
	errorCounter metric.Int64Counter
	resultGauge  metric.Float64Gauge
)

// InitMetrics registers custom OTel metric instruments for the calculator domain.
// Call this once at startup (after observability.InitMetrics).
func InitMetrics() error {
	meter := otel.Meter("calculator")

	var err error

	opsCounter, err = meter.Int64Counter("calculator.operations.total",
		metric.WithDescription("Total number of calculator operations performed"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		return fmt.Errorf("creating ops counter: %w", err)
	}

	opsHistogram, err = meter.Float64Histogram("calculator.operation.duration",
		metric.WithDescription("Duration of calculator operations in milliseconds"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.5, 1, 5, 10),
	)
	if err != nil {
		return fmt.Errorf("creating ops histogram: %w", err)
	}

	errorCounter, err = meter.Int64Counter("calculator.errors.total",
		metric.WithDescription("Total number of calculator errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return fmt.Errorf("creating error counter: %w", err)
	}

	resultGauge, err = meter.Float64Gauge("calculator.last_result",
		metric.WithDescription("The result of the last calculator operation"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("creating result gauge: %w", err)
	}

	return nil
}
