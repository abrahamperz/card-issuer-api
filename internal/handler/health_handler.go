package handler

import (
	"context"
	"net/http"
)

type HealthChecker interface {
	Check(ctx context.Context) error
}

type HealthHandler struct {
	checker HealthChecker
}

func NewHealthHandler(checker HealthChecker) *HealthHandler {
	return &HealthHandler{checker: checker}
}

func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HealthHandler) HandleReady(w http.ResponseWriter, r *http.Request) {
	if h.checker != nil {
		if err := h.checker.Check(r.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, "service unavailable", "UNAVAILABLE")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
