package httpapi

import (
	"encoding/json"
	"net/http"

	"video-record/internal/storage"
)

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func readyz(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil || db.Ready(r.Context()) != nil {
			writeProblem(w, r, http.StatusServiceUnavailable, "Service Unavailable", "not_ready")
			return
		}
		healthz(w, r)
	}
}
