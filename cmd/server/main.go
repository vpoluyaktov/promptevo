// Command server is the promptevo API entrypoint: load config, open the store,
// run migrations, build the chi router, and serve with graceful shutdown.
// See ARCHITECTURE.md §3 and §12.
package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// newRouter wires the API routes. Backend fills in the handlers.
// Routes per ARCHITECTURE.md §4 (mounted under /api; /healthz at root).
func newRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Route("/api", func(r chi.Router) {
		// TODO(backend): GET /models, GET|POST /runs, GET /runs/{id},
		// GET /runs/{id}/generations, GET /runs/{id}/games,
		// GET /games/{id}/guesses, GET /runs/{id}/stream (SSE), DELETE /runs/{id}.
		_ = r
	})

	return r
}

func main() {
	// TODO(backend): load config (§12) -> open store -> store.Migrate ->
	// load word lists -> build llm.Client + agent + reflector + orchestrator ->
	// inject handlers into the router -> http.Server with graceful shutdown.
	_ = newRouter()
	log.Println("promptevo server: stub — implement per ARCHITECTURE.md")
}
