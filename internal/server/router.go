package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"go-chi-observability/internal/calculator"
	"go-chi-observability/internal/handlers"
	"go-chi-observability/internal/observability"
)

func NewRouter() http.Handler {

	r := chi.NewRouter()

	r.Use(observability.RequestIDMiddleware)
	r.Use(observability.TracingMiddleware)
	r.Use(observability.LoggingMiddleware)

	r.Get("/health", handlers.Health)

	// Domain route groups — add new RegisterRoutes calls here as the project grows
	calculator.RegisterRoutes(r)

	r.Handle("/metrics", observability.PrometheusHandler())

	return r
}
