package api

import (
	"net/http"

	"github.com/rangoDJ/meshmanager/internal/settings"
)

type settingsResponse struct {
	Settings settings.Settings `json:"settings"`
	Defaults settings.Settings `json:"defaults"`
	// Path of the JSON file the settings are stored in, for people who edit it by hand.
	File string `json:"file"`
}

func (s *Server) settingsResponse() settingsResponse {
	return settingsResponse{Settings: s.settings.Get(), Defaults: settings.Defaults(), File: s.settings.Path()}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.settingsResponse())
}

// handleSaveSettings replaces the settings. Omitted fields keep their current values.
func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	next := s.settings.Get()
	if !decodeJSON(w, r, &next) {
		return
	}
	if err := next.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.settings.Save(next); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.settingsResponse())
}
