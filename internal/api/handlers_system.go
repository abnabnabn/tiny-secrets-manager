package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	client := r.Context().Value(clientCtxKey).(Client)
	if !client.IsAdmin {
		s.respondError(w, http.StatusForbidden, "forbidden")
		return
	}

	ctx := r.Context()
	settings, err := s.store.GetAllSettings(ctx)
	if err != nil {
		s.logger.Error("failed to get settings", "err", err)
		s.respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	s.respondJSON(w, http.StatusOK, settings)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	client := r.Context().Value(clientCtxKey).(Client)
	if !client.IsAdmin {
		s.respondError(w, http.StatusForbidden, "forbidden")
		return
	}

	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	// Strictly validate keys and values at the input boundary
	for k, v := range req {
		switch k {
		case "backup_target":
			if strings.HasPrefix(strings.TrimSpace(v), "-") {
				s.respondError(w, http.StatusBadRequest, "invalid backup target: cannot start with a dash")
				return
			}
		case "backup_interval_mins":
			val, err := strconv.Atoi(v)
			if err != nil || val < 1 {
				s.respondError(w, http.StatusBadRequest, "invalid backup interval: must be an integer >= 1")
				return
			}
		case "backup_retention_all_days":
			val, err := strconv.Atoi(v)
			if err != nil || val < 0 {
				s.respondError(w, http.StatusBadRequest, "invalid backup retention all days: must be an integer >= 0")
				return
			}
		case "backup_retention_daily_days":
			val, err := strconv.Atoi(v)
			if err != nil || val < 0 {
				s.respondError(w, http.StatusBadRequest, "invalid backup retention daily days: must be an integer >= 0")
				return
			}
		case "auto_populate_env_name":
			if v != "true" && v != "false" {
				s.respondError(w, http.StatusBadRequest, "invalid auto populate env name: must be 'true' or 'false'")
				return
			}
		default:
			s.respondError(w, http.StatusBadRequest, "unsupported settings key: "+k)
			return
		}
	}

	ctx := r.Context()
	for k, v := range req {
		if err := s.store.PutSetting(ctx, k, v); err != nil {
			s.logger.Error("failed to update setting", "err", err)
			s.respondError(w, http.StatusInternalServerError, "internal server error")
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleTriggerBackup(w http.ResponseWriter, r *http.Request) {
	client := r.Context().Value(clientCtxKey).(Client)
	if !client.IsAdmin {
		s.respondError(w, http.StatusForbidden, "forbidden")
		return
	}

	if err := s.runBackup(); err != nil {
		s.logger.Error("manual backup failed", "err", err)
		s.respondError(w, http.StatusInternalServerError, "backup failed: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
}
