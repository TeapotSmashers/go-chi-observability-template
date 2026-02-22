package calculator

import "github.com/go-chi/chi/v5"

// RegisterRoutes mounts all calculator endpoints onto the given router
// under the /calculator prefix.
func RegisterRoutes(r chi.Router) {
	r.Route("/calculator", func(r chi.Router) {
		r.Post("/add", Add)
		r.Post("/subtract", Subtract)
		r.Post("/multiply", Multiply)
		r.Post("/divide", Divide)
		r.Post("/chain", Chain)
	})
}
