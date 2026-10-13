package api

import "net/http"

type healthResponse struct {
	Status string `json:"status"`
}

// health handles GET /health. It returns 503 when the database is unreachable.
func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if h.Pinger != nil {
		if err := h.Pinger.Ping(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "database unavailable"})
			return
		}
	}

	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}
