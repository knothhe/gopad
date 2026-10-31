// Package launcher discovers installed applications and persists a user's launchpad.
package launcher

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type App struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	BundleID string `json:"bundleId"`
	Version  string `json:"version"`
	Kind     string `json:"kind"`
}

type Preferences struct {
	Hidden          []string   `json:"hidden"`
	Order           []string   `json:"order"`
	IconSize        int        `json:"iconSize"`
	Favorites       []string   `json:"favorites"`
	Categories      []Category `json:"categories"`
	Language        string     `json:"language"`
	ShowMenuBarIcon bool       `json:"showMenuBarIcon"`
	LastCategory    string     `json:"lastCategory"`
}

type State struct {
	Apps        []App       `json:"apps"`
	Preferences Preferences `json:"preferences"`
}

type Service struct {
	mu          sync.Mutex
	configPath  string
	apps        []App
	preferences Preferences
	loaded      bool
	scan        func() ([]App, error)
	icons       map[string][]byte
	iconMu      sync.Mutex
}

// New reads preferences without touching AppKit or scanning the disk.
func New(configPath string) (*Service, error) {
	if configPath == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		configPath = filepath.Join(dir, "gopad", "preferences.json")
	}
	s := &Service{configPath: configPath, preferences: defaultPreferences(), scan: ScanApplications, icons: make(map[string][]byte)}
	data, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read gopad preferences: %w", err)
	}
	if err := json.Unmarshal(data, &s.preferences); err != nil {
		// Later category selections and settings saves can replace this file.
		// Secure a separate recovery copy before allowing writable defaults.
		backup, err := backupMalformedPreferences(configPath, data)
		if err != nil {
			return nil, fmt.Errorf("preserve malformed gopad preferences: %w", err)
		}
		log.Printf("gopad: malformed preferences backed up to %s; using defaults", backup)
		s.preferences = defaultPreferences()
		return s, nil
	}
	s.preferences = normalizePreferences(s.preferences)
	return s, nil
}

func backupMalformedPreferences(configPath string, data []byte) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(configPath), filepath.Base(configPath)+".corrupt-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func defaultPreferences() Preferences {
	return Preferences{Hidden: []string{}, Order: []string{}, Favorites: []string{}, IconSize: 80, Categories: DefaultCategories(), Language: "system", LastCategory: "all"}
}

func normalizePreferences(p Preferences) Preferences {
	if p.IconSize == 0 {
		p.IconSize = 80
	}
	if p.IconSize < 48 {
		p.IconSize = 48
	}
	if p.IconSize > 112 {
		p.IconSize = 112
	}
	p.Hidden = uniqueIDs(p.Hidden)
	p.Order = uniqueIDs(p.Order)
	p.Favorites = uniqueIDs(p.Favorites)
	p.Categories = CategoriesFor(p)
	if strings.TrimSpace(p.Language) == "" {
		p.Language = "system"
	}
	// Keep unknown language preferences for a future version; the presentation
	// layer resolves an English fallback without changing the stored choice.
	if !validCategoryID(p.LastCategory) {
		p.LastCategory = "all"
	}
	visible := VisibleCategories(p)
	found := false
	for _, category := range visible {
		if category.ID == p.LastCategory {
			found = true
			break
		}
	}
	if !found && len(visible) > 0 {
		p.LastCategory = visible[0].ID
	}
	return p
}

func uniqueIDs(ids []string) []string {
	out := make([]string, 0, min(len(ids), maxAppIDs))
	seen := make(map[string]bool, min(len(ids), maxAppIDs))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if validAppID(id) && !seen[id] {
			out = append(out, id)
			seen[id] = true
			if len(out) == maxAppIDs {
				break
			}
		}
	}
	return out
}

func clonePreferences(p Preferences) Preferences {
	p.Hidden = append([]string{}, p.Hidden...)
	p.Order = append([]string{}, p.Order...)
	p.Favorites = append([]string{}, p.Favorites...)
	p.Categories = cloneCategories(p.Categories)
	return p
}

func (s *Service) stateLocked() State {
	return State{Apps: append([]App{}, s.apps...), Preferences: clonePreferences(s.preferences)}
}

func (s *Service) loadLocked() error {
	if s.loaded {
		return nil
	}
	apps, err := s.scan()
	if err != nil {
		return err
	}
	s.apps, s.loaded = apps, true
	return nil
}

func (s *Service) State() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return State{}, err
	}
	return s.stateLocked(), nil
}

func (s *Service) Refresh() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	apps, err := s.scan()
	if err != nil {
		return State{}, err
	}
	s.apps, s.loaded = apps, true
	s.iconMu.Lock()
	s.icons = make(map[string][]byte)
	s.iconMu.Unlock()
	return s.stateLocked(), nil
}

// SavePreferences writes atomically before publishing the new state. IDs of
// temporarily unavailable apps are retained, so reinstalling restores their place.
func (s *Service) SavePreferences(p Preferences) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return State{}, err
	}
	return s.savePreferencesLocked(p)
}

// Preferences reads settings without rescanning applications or calling AppKit.
func (s *Service) Preferences() Preferences {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePreferences(s.preferences)
}

// SaveManagementPreferences prevents a stale settings page from overwriting
// the native launcher's most recently selected section.
func (s *Service) SaveManagementPreferences(p Preferences) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return State{}, err
	}
	p.LastCategory = s.preferences.LastCategory
	return s.savePreferencesLocked(p)
}

// SetLastCategory is a narrow atomic mutation, so an asynchronous native
// selection never overwrites unrelated edits made in the management window.
func (s *Service) SetLastCategory(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := clonePreferences(s.preferences)
	p.LastCategory = id
	p = normalizePreferences(p)
	if p.LastCategory == s.preferences.LastCategory {
		return nil
	}
	_, err := s.savePreferencesLocked(p)
	return err
}

func (s *Service) savePreferencesLocked(p Preferences) (State, error) {
	p = normalizePreferences(p)
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return State{}, err
	}
	dir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return State{}, err
	}
	f, err := os.CreateTemp(dir, ".preferences-*")
	if err != nil {
		return State{}, err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(append(data, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp, s.configPath)
	}
	if err != nil {
		return State{}, fmt.Errorf("save gopad preferences: %w", err)
	}
	s.preferences = p
	return s.stateLocked(), nil
}

func (s *Service) find(id string) (App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return App{}, err
	}
	for _, a := range s.apps {
		if a.ID == id {
			return a, nil
		}
	}
	return App{}, errors.New("application no longer exists; refresh the application list")
}

func (s *Service) Launch(id string) error {
	a, err := s.find(id)
	if err != nil {
		return err
	}
	if err := validateBundle(a.Path); err != nil {
		return err
	}
	return openApplication(a.Path, false)
}

func (s *Service) Reveal(id string) error {
	a, err := s.find(id)
	if err != nil {
		return err
	}
	return openApplication(a.Path, true)
}

// Path returns only catalogued paths, never an arbitrary file supplied by a client.
func (s *Service) Path(id string) (string, error) {
	a, err := s.find(id)
	return a.Path, err
}

func (s *Service) Icon(id string) ([]byte, error) {
	a, err := s.find(id)
	if err != nil {
		return nil, err
	}
	s.iconMu.Lock()
	defer s.iconMu.Unlock()
	if png, ok := s.icons[id]; ok {
		return png, nil
	}
	png, err := applicationIcon(a.Path)
	if err != nil {
		return nil, err
	}
	s.icons[id] = png
	return png, nil
}

func applicationID(path string) string {
	// The same bundle can be found through a directory alias or a .app
	// symlink. Adding an earlier-sorting alias must not reset its preferences.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:12])
}

func validateBundle(path string) error {
	if !filepath.IsAbs(path) || !strings.HasSuffix(strings.ToLower(path), ".app") {
		return errors.New("application path is invalid; refresh the application list")
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return errors.New("application was moved or removed; refresh the application list")
	}
	info, err = os.Stat(filepath.Join(path, "Contents", "Info.plist"))
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("application bundle is no longer valid; refresh the application list")
	}
	return nil
}

func ScanApplications() ([]App, error) {
	roots := []string{"/Applications", "/System/Applications", "/System/Library/CoreServices/Applications"}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, "Applications"))
	}
	paths, err := discoverBundles(roots)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat("/System/Library/CoreServices/Finder.app"); err == nil && info.IsDir() {
		paths = append(paths, "/System/Library/CoreServices/Finder.app")
	}
	apps, err := describeApplications(paths)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(apps, func(i, j int) bool {
		a, b := strings.ToLower(apps[i].Name), strings.ToLower(apps[j].Name)
		if a == b {
			return apps[i].Path < apps[j].Path
		}
		return a < b
	})
	return apps, nil
}

// discoverBundles stops at .app boundaries, never exposing nested helper apps.
// Directory symlinks are followed with canonical-path deduplication and cycles
// are ignored, including Safari's symlink into the system cryptex.
func discoverBundles(roots []string) ([]string, error) {
	var paths []string
	seen := map[string]bool{}
	var visit func(string, int)
	visit = func(path string, depth int) {
		if depth > 32 {
			return
		}
		real, err := filepath.EvalSymlinks(path)
		if err != nil || seen[real] {
			return
		}
		info, err := os.Stat(real)
		if err != nil || !info.IsDir() {
			return
		}
		seen[real] = true
		if strings.HasSuffix(strings.ToLower(path), ".app") {
			// Ignore abandoned .app directories and ordinary folders named .app.
			if _, err := os.Stat(filepath.Join(real, "Contents", "Info.plist")); err == nil {
				paths = append(paths, path)
			}
			return
		}
		entries, err := os.ReadDir(real)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				visit(filepath.Join(path, entry.Name()), depth+1)
			}
		}
	}
	for _, root := range roots {
		visit(root, 0)
	}
	return paths, nil
}
