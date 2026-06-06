package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	client := r.Context().Value(clientCtxKey).(Client)
	if !client.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ctx := r.Context()
	settings, err := s.store.GetAllSettings(ctx)
	if err != nil {
		s.logger.Error("failed to get settings", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(settings)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	client := r.Context().Value(clientCtxKey).(Client)
	if !client.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	for k, v := range req {
		if err := s.store.PutSetting(ctx, k, v); err != nil {
			s.logger.Error("failed to update setting", "key", k, "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleTriggerBackup(w http.ResponseWriter, r *http.Request) {
	client := r.Context().Value(clientCtxKey).(Client)
	if !client.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := s.runBackup(); err != nil {
		s.logger.Error("manual backup failed", "err", err)
		http.Error(w, "backup failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
