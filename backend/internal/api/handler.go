package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/kaskol10/cnpg-migrator/internal/migration"
	"github.com/kaskol10/cnpg-migrator/internal/models"
	"github.com/kaskol10/cnpg-migrator/internal/store"
)

type Handler struct {
	svc *migration.Service
}

func NewHandler(svc *migration.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/health", h.health)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/config", h.getConfig)
		r.Get("/migrations", h.listMigrations)
		r.Post("/migrations", h.createMigration)
		r.Get("/migrations/{id}", h.getMigration)
		r.Get("/migrations/{id}/logs", h.getLogs)
		r.Get("/migrations/{id}/verification", h.getVerification)
		r.Post("/migrations/{id}/verify", h.startVerification)
		r.Post("/migrations/{id}/cancel", h.cancelMigration)
		r.Delete("/migrations/{id}/resources", h.cleanupResources)
	})
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Config())
}

func (h *Handler) listMigrations(w http.ResponseWriter, r *http.Request) {
	migrations := h.svc.List()
	writeJSON(w, http.StatusOK, migrations)
}

func (h *Handler) createMigration(w http.ResponseWriter, r *http.Request) {
	var req models.CreateMigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	m, err := h.svc.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, sanitizeMigration(m))
}

func (h *Handler) getMigration(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, err := h.svc.Get(id)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "migration not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sanitizeMigration(m))
}

func (h *Handler) getLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.svc.Get(id); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "migration not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logs := h.svc.Logs(id)
	writeJSON(w, http.StatusOK, logs)
}

func (h *Handler) getVerification(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	v, err := h.svc.Verification(id)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "migration not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) startVerification(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	v, err := h.svc.StartVerification(r.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "migration not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) cancelMigration(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, err := h.svc.Cancel(r.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "migration not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sanitizeMigration(m))
}

func (h *Handler) cleanupResources(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.Cleanup(r.Context(), id); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "migration not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleaned"})
}

func sanitizeMigration(m *models.Migration) *models.Migration {
	copy := *m
	copy.Source.Password = ""
	copy.Target.Password = ""
	return &copy
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
