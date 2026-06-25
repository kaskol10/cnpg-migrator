package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/kaskol10/cnpg-migrator/internal/api"
	"github.com/kaskol10/cnpg-migrator/internal/config"
	"github.com/kaskol10/cnpg-migrator/internal/k8s"
	"github.com/kaskol10/cnpg-migrator/internal/migration"
	"github.com/kaskol10/cnpg-migrator/internal/store"
)

const webDist = "./web/dist"

func main() {
	cfg := config.Load()

	k8sClient, err := k8s.NewClient(cfg)
	if err != nil {
		log.Fatalf("kubernetes client: %v", err)
	}

	st := store.New()
	svc := migration.NewService(st, k8sClient, cfg)
	handler := api.NewHandler(svc)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	handler.Register(r)
	setupFrontend(r)

	log.Printf("starting server on %s (namespace: %s)", cfg.Addr, cfg.Namespace)
	if err := http.ListenAndServe(cfg.Addr, r); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func setupFrontend(r chi.Router) {
	if _, err := os.Stat(webDist); err != nil {
		log.Printf("frontend not found at %s, API-only mode", webDist)
		return
	}

	log.Printf("serving frontend from %s", webDist)
	dist := http.Dir(webDist)
	fileServer := http.FileServer(dist)

	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		http.ServeFile(w, req, filepath.Join(webDist, "index.html"))
	})
	r.Handle("/assets/*", http.StripPrefix("/", fileServer))

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}
		http.ServeFile(w, req, filepath.Join(webDist, "index.html"))
	})
}
