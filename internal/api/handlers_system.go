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

	// Validate settings keys and values at the input boundary
	for k, v := range req {
		switch k {
		case "backup_target":
			v = strings.TrimSpace(v)
			if strings.HasPrefix(v, "-") {
				s.respondError(w, http.StatusBadRequest, "invalid backup target: cannot start with a dash")
				return
			}
		case "backup_interval_mins":
			parsed, err := strconv.Atoi(v)
			if err != nil || parsed < 1 {
				s.respondError(w, http.StatusBadRequest, "backup_interval_mins must be a positive integer")
				return
			}
		case "backup_retention_all_days":
			parsed, err := strconv.Atoi(v)
			if err != nil || parsed < 0 {
				s.respondError(w, http.StatusBadRequest, "backup_retention_all_days must be a non-negative integer")
				return
			}
		case "backup_retention_daily_days":
			parsed, err := strconv.Atoi(v)
			if err != nil || parsed < 0 {
				s.respondError(w, http.StatusBadRequest, "backup_retention_daily_days must be a non-negative integer")
				return
			}
		case "auto_populate_env_name":
			if v != "true" && v != "false" {
				s.respondError(w, http.StatusBadRequest, "auto_populate_env_name must be 'true' or 'false'")
				return
			}
		default:
			s.respondError(w, http.StatusBadRequest, "unrecognized setting key: "+k)
			return
		}
	}

	ctx := r.Context()
	for k, v := range req {
		if k == "backup_target" {
			v = strings.TrimSpace(v)
		}
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
