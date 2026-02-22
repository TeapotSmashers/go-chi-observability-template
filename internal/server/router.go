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

	r.Handle("/metrics", observability.PrometheusHandler())
	r.Get("/health", handlers.Health)

	r.Group(func(r chi.Router) {
		r.Use(observability.RequestIDMiddleware)
		r.Use(observability.TracingMiddleware)
		r.Use(observability.LoggingMiddleware)

		// Domain route groups — add new RegisterRoutes calls here as the project grows
		calculator.RegisterRoutes(r)
	})

	return r
}
