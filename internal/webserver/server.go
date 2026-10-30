// Package webserver serves the same launcher UI in MyGo and in a local preview.
package webserver

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/knothhe/gopad/internal/appsearch"
	"github.com/knothhe/gopad/internal/i18n"
	"github.com/knothhe/gopad/internal/launcher"
)

type Catalog interface {
	State() (launcher.State, error)
	Refresh() (launcher.State, error)
	SavePreferences(launcher.Preferences) (launcher.State, error)
	Launch(string) error
	Reveal(string) error
	Icon(string) ([]byte, error)
}

// Controls are supplied by the native shell. Preview leaves them unset.
type Controls struct {
	Hide               func()
	OpenManager        func()
	Fullscreen         func()
	CopyPath           func(string) error
	Quit               func()
	SystemLocale       func() string
	PreferencesChanged func(launcher.Preferences)
	UpdateStatus       func() UpdateState
	CheckUpdates       func()
	SetAutomaticChecks func(bool) error
	Version            string
	// Wallpaper is the same PNG loaded for the native launchpad on the UI
	// thread. HTTP handlers never query NSScreen or read arbitrary files.
	Wallpaper []byte
}

type server struct {
	catalog  Catalog
	controls Controls
	static   http.Handler
}

func New(catalog Catalog, frontend fs.FS, assets fs.FS, controls Controls) http.Handler {
	mux := http.NewServeMux()
	controls.Wallpaper = append([]byte(nil), controls.Wallpaper...)
	s := &server{catalog: catalog, controls: controls, static: http.FileServer(http.FS(frontend))}
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("/api/state", s.state)
	mux.HandleFunc("/api/refresh", s.refresh)
	mux.HandleFunc("/api/preferences", s.preferences)
	mux.HandleFunc("/api/i18n", s.translations)
	mux.HandleFunc("/api/update", s.updateState)
	mux.HandleFunc("/api/update/check", s.checkUpdates)
	mux.HandleFunc("/api/update/preferences", s.updatePreferences)
	mux.HandleFunc("/api/launch", s.launch)
	mux.HandleFunc("/api/reveal", s.reveal)
	mux.HandleFunc("/api/copy-path", s.copyPath)
	mux.HandleFunc("/api/icon/", s.icon)
	mux.HandleFunc("/api/wallpaper", s.wallpaper)
	mux.HandleFunc("/api/hide-window", s.control(controls.Hide))
	mux.HandleFunc("/api/manage", s.control(controls.OpenManager))
	mux.HandleFunc("/api/fullscreen", s.control(controls.Fullscreen))
	mux.HandleFunc("/api/quit", s.control(controls.Quit))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		s.static.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		// The preview server has real application actions. Only our own origin
		// may invoke them; an arbitrary web page must not launch applications.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" || !sameOrigin(r) {
				writeError(w, http.StatusForbidden, errors.New("error.untrustedOrigin"))
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host && (u.Scheme == r.URL.Scheme || (r.URL.Scheme == "" && u.Scheme == "http"))
}

func method(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Method == want {
		return true
	}
	w.Header().Set("Allow", want)
	writeError(w, http.StatusMethodNotAllowed, errors.New("error.method"))
	return false
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, err error) {
	key := err.Error()
	if !strings.HasPrefix(key, "error.") {
		key = "error.operation"
	}
	writeJSON(w, status, map[string]string{"error": i18n.T("en", key), "errorKey": key, "detail": err.Error()})
}
func result(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func stateResult(w http.ResponseWriter, state launcher.State, err error) {
	texts := make([]string, 0, len(state.Apps)*2+len(state.Preferences.Categories))
	for _, app := range state.Apps {
		texts = append(texts, app.Name, app.Path)
	}
	for _, category := range state.Preferences.Categories {
		texts = append(texts, category.Name)
	}
	result(w, struct {
		launcher.State
		Pinyin map[string][]string `json:"pinyin"`
	}{state, appsearch.Readings(texts...)}, err)
}
func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("error.invalidRequest"))
		return false
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		writeError(w, http.StatusBadRequest, errors.New("error.invalidRequest"))
		return false
	}
	return true
}
func (s *server) state(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	v, e := s.catalog.State()
	stateResult(w, v, e)
}
func (s *server) refresh(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	v, e := s.catalog.Refresh()
	stateResult(w, v, e)
}
func (s *server) preferences(w http.ResponseWriter, r *http.Request) {
	// sendBeacon uses POST when the page closes; normal edits use PUT.
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		w.Header().Set("Allow", "PUT, POST")
		writeError(w, http.StatusMethodNotAllowed, errors.New("error.method"))
		return
	}
	var p launcher.Preferences
	if !decode(w, r, &p) {
		return
	}
	var v launcher.State
	var e error
	if managed, ok := s.catalog.(interface {
		SaveManagementPreferences(launcher.Preferences) (launcher.State, error)
	}); ok {
		v, e = managed.SaveManagementPreferences(p)
	} else {
		v, e = s.catalog.SavePreferences(p)
	}
	if e == nil && s.controls.PreferencesChanged != nil {
		s.controls.PreferencesChanged(v.Preferences)
	}
	stateResult(w, v, e)
}
func (s *server) action(w http.ResponseWriter, r *http.Request, action func(string) error) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.ID == "" {
		writeError(w, http.StatusBadRequest, errors.New("error.selectApp"))
		return
	}
	if err := action(body.ID); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
func (s *server) launch(w http.ResponseWriter, r *http.Request) {
	s.action(w, r, func(id string) error {
		if e := s.catalog.Launch(id); e != nil {
			return e
		}
		if s.controls.Hide != nil {
			s.controls.Hide()
		}
		return nil
	})
}
func (s *server) reveal(w http.ResponseWriter, r *http.Request) {
	s.action(w, r, func(id string) error {
		if e := s.catalog.Reveal(id); e != nil {
			return e
		}
		if s.controls.Hide != nil {
			s.controls.Hide()
		}
		return nil
	})
}
func (s *server) copyPath(w http.ResponseWriter, r *http.Request) {
	s.action(w, r, func(id string) error {
		state, err := s.catalog.State()
		if err != nil {
			return err
		}
		for _, app := range state.Apps {
			if app.ID == id {
				if s.controls.CopyPath == nil {
					return errors.New("error.copyPreview")
				}
				return s.controls.CopyPath(app.Path)
			}
		}
		return errors.New("error.appNotFound")
	})
}
func (s *server) icon(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/icon/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	data, err := s.catalog.Icon(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(data)
}
func (s *server) wallpaper(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	if len(s.controls.Wallpaper) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(s.controls.Wallpaper)
}
func (s *server) control(action func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !method(w, r, http.MethodPost) {
			return
		}
		if action == nil {
			writeError(w, http.StatusNotImplemented, errors.New("error.desktopOnly"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		action()
	}
}
