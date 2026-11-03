package webserver

import (
	"errors"
	"github.com/knothhe/gopad/internal/buildinfo"
	"github.com/knothhe/gopad/internal/i18n"
	"github.com/knothhe/gopad/internal/launcher"
	"net/http"
)

type UpdateState struct {
	Status          string `json:"status"`
	CurrentVersion  string `json:"currentVersion"`
	AutomaticChecks bool   `json:"automaticChecks"`
	LastCheck       string `json:"lastCheck,omitempty"`
}

func (s *server) translations(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	var preferences launcher.Preferences
	if reader, ok := s.catalog.(interface{ Preferences() launcher.Preferences }); ok {
		preferences = reader.Preferences()
	} else {
		state, err := s.catalog.State()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		preferences = state.Preferences
	}
	system := i18n.EnvironmentLocale()
	if s.controls.SystemLocale != nil {
		system = s.controls.SystemLocale()
	}
	language := preferences.Language
	if language == "" {
		language = "system"
	}
	locale := i18n.Resolve(language, system)
	shortcut := i18n.T(locale, "app.shortcut.release")
	if buildinfo.Channel != "release" {
		shortcut = i18n.T(locale, "app.shortcut.dev")
	}
	if preferences.Shortcut != "" {
		shortcut = preferences.Shortcut
	}
	writeJSON(w, http.StatusOK, map[string]any{"locale": locale, "language": language, "messages": i18n.Messages(locale), "appName": buildinfo.Name(), "version": buildinfo.Version, "channel": buildinfo.Channel, "shortcut": shortcut})
}

func (s *server) updates() UpdateState {
	if s.controls.UpdateStatus != nil {
		return s.controls.UpdateStatus()
	}
	return UpdateState{Status: "preview", CurrentVersion: s.controls.Version}
}

func (s *server) updateState(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, s.updates())
}

func (s *server) checkUpdates(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if s.controls.CheckUpdates == nil || s.updates().Status != "ready" {
		writeError(w, http.StatusConflict, errors.New("error.updateUnavailable"))
		return
	}
	s.controls.CheckUpdates()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) updatePreferences(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPut) {
		return
	}
	var body struct {
		AutomaticChecks *bool `json:"automaticChecks"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.AutomaticChecks == nil {
		writeError(w, http.StatusBadRequest, errors.New("error.invalidRequest"))
		return
	}
	if s.controls.SetAutomaticChecks == nil || s.updates().Status != "ready" {
		writeError(w, http.StatusConflict, errors.New("error.updateUnavailable"))
		return
	}
	if err := s.controls.SetAutomaticChecks(*body.AutomaticChecks); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("error.updateSave"))
		return
	}
	writeJSON(w, http.StatusOK, s.updates())
}
