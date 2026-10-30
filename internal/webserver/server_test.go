package webserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/knothhe/gopad/internal/launcher"
)

type mockCatalog struct {
	launches  int
	launchErr error
}

func (m *mockCatalog) State() (launcher.State, error) {
	return launcher.State{Apps: []launcher.App{{ID: "app-one", Name: "Notes", Path: "/Applications/Notes.app"}}}, nil
}
func (m *mockCatalog) Refresh() (launcher.State, error) { return m.State() }
func (m *mockCatalog) SavePreferences(p launcher.Preferences) (launcher.State, error) {
	s, _ := m.State()
	s.Preferences = p
	return s, nil
}
func (m *mockCatalog) Launch(string) error         { m.launches++; return m.launchErr }
func (m *mockCatalog) Reveal(string) error         { return nil }
func (m *mockCatalog) Icon(string) ([]byte, error) { return []byte("png"), nil }
func handler(m *mockCatalog, c Controls) http.Handler {
	return New(m, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("launcher")}}, fstest.MapFS{"gopad.svg": &fstest.MapFile{Data: []byte("<svg/>")}}, c)
}
func request(h http.Handler, method, path, body, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:5189"+path, strings.NewReader(body))
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestCrossOriginCannotInvokeApplication(t *testing.T) {
	m := &mockCatalog{}
	h := handler(m, Controls{})
	for _, origin := range []string{"https://evil.example", "http://localhost:5189", "http://127.0.0.1:9999", "null"} {
		w := request(h, "POST", "/api/launch", `{"id":"app-one"}`, origin)
		if w.Code != http.StatusForbidden {
			t.Errorf("origin %q: status %d", origin, w.Code)
		}
	}
	if m.launches != 0 {
		t.Fatalf("cross-origin request launched %d applications", m.launches)
	}
	w := request(h, "POST", "/api/launch", `{"id":"app-one"}`, "http://127.0.0.1:5189")
	if w.Code != http.StatusOK || m.launches != 1 {
		t.Fatalf("same-origin request failed: %d %s", w.Code, w.Body)
	}
}

func TestFailedLaunchDoesNotHide(t *testing.T) {
	m := &mockCatalog{launchErr: errors.New("application was moved")}
	hidden := false
	h := handler(m, Controls{Hide: func() { hidden = true }})
	w := request(h, "POST", "/api/launch", `{"id":"app-one"}`, "")
	if w.Code != http.StatusBadRequest || hidden {
		t.Fatalf("failed launch: status %d hidden %v", w.Code, hidden)
	}
	m.launchErr = nil
	w = request(h, "POST", "/api/launch", `{"id":"app-one"}`, "")
	if w.Code != http.StatusOK || !hidden {
		t.Fatalf("successful launch: status %d hidden %v", w.Code, hidden)
	}
}

func TestActionsRejectMalformedInputAndWrongMethods(t *testing.T) {
	m := &mockCatalog{}
	h := handler(m, Controls{})
	for _, body := range []string{`{}`, `{"id":"app-one","path":"/bad.app"}`, `{"id":"app-one"} {"id":"other"}`, `invalid`} {
		w := request(h, "POST", "/api/launch", body, "")
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %q: status %d", body, w.Code)
		}
	}
	w := request(h, "GET", "/api/launch", "", "")
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "POST" {
		t.Fatalf("GET action status %d", w.Code)
	}
	if m.launches != 0 {
		t.Fatalf("invalid requests launched %d applications", m.launches)
	}
}

func TestCopyPathUsesCatalogRatherThanSuppliedPath(t *testing.T) {
	var copied string
	h := handler(&mockCatalog{}, Controls{CopyPath: func(path string) error { copied = path; return nil }})
	w := request(h, "POST", "/api/copy-path", `{"id":"app-one"}`, "")
	if w.Code != http.StatusOK || copied != "/Applications/Notes.app" {
		t.Fatalf("copy status %d path %q", w.Code, copied)
	}
	copied = ""
	w = request(h, "POST", "/api/copy-path", `{"id":"missing"}`, "")
	if w.Code != http.StatusBadRequest || copied != "" {
		t.Fatalf("unknown application copied %q", copied)
	}
}

func TestStaticAPIAndPreviewControls(t *testing.T) {
	h := handler(&mockCatalog{}, Controls{})
	for _, path := range []string{"/", "/assets/gopad.svg", "/api/state", "/api/icon/app-one"} {
		w := request(h, "GET", path, "", "")
		if w.Code != http.StatusOK {
			t.Errorf("%s: status %d", path, w.Code)
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Error("missing nosniff header")
		}
	}
	if w := request(h, "GET", "/api/missing", "", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown API status %d", w.Code)
	}
	if w := request(h, "POST", "/api/hide-window", "", ""); w.Code != http.StatusNotImplemented {
		t.Fatalf("preview hide status %d", w.Code)
	}
}

func TestNativeSchemeOrigin(t *testing.T) {
	m := &mockCatalog{}
	h := handler(m, Controls{})
	r := httptest.NewRequest("POST", "mygo://localhost/api/launch", strings.NewReader(`{"id":"app-one"}`))
	r.Header.Set("Origin", "mygo://localhost")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || m.launches != 1 {
		t.Fatalf("native scheme request failed: %d %s", w.Code, w.Body)
	}
}

func TestPreferencesBeaconPost(t *testing.T) {
	h := handler(&mockCatalog{}, Controls{})
	w := request(h, "POST", "/api/preferences", `{"hidden":[],"order":[],"favorites":[],"iconSize":96}`, "http://127.0.0.1:5189")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"iconSize":96`) {
		t.Fatalf("preferences beacon failed: %d %s", w.Code, w.Body)
	}
	w = request(h, "POST", "/api/preferences", `{"iconSize":96}`, "https://evil.example")
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin preferences beacon accepted: %d", w.Code)
	}
}

func TestWallpaperUsesNativePNGCacheAndPreviewFallsBack(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nwallpaper-cache")
	want := append([]byte(nil), png...)
	h := handler(&mockCatalog{}, Controls{Wallpaper: png})
	// The UI can release or reuse its input buffer after constructing the server.
	png[len(png)-1] = 'x'
	w := request(h, "GET", "/api/wallpaper", "", "")
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), want) {
		t.Fatalf("wallpaper cache response: status %d bytes %q", w.Code, w.Body.Bytes())
	}
	if w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "private, max-age=3600" {
		t.Fatalf("wallpaper headers: %v", w.Header())
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "img-src 'self'") {
		t.Fatal("same-origin wallpaper is blocked by CSP")
	}
	preview := handler(&mockCatalog{}, Controls{})
	if w := request(preview, "GET", "/api/wallpaper", "", ""); w.Code != http.StatusNotFound {
		t.Fatalf("preview wallpaper must use the CSS fallback: status %d", w.Code)
	}
}

func TestPreferencesAcceptCategoryMembershipAndVisibility(t *testing.T) {
	h := handler(&mockCatalog{}, Controls{})
	body := `{"hidden":[],"order":[],"favorites":[],"iconSize":80,"categories":[{"id":"custom-work","name":"工作","appIds":["app-one","uninstalled"],"visible":true}]}`
	w := request(h, "PUT", "/api/preferences", body, "http://127.0.0.1:5189")
	if w.Code != http.StatusOK {
		t.Fatalf("category preferences rejected: %d %s", w.Code, w.Body)
	}
	var state launcher.State
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Preferences.Categories) != 1 {
		t.Fatal("category schema was lost in the response", state.Preferences)
	}
	category := state.Preferences.Categories[0]
	if category.ID != "custom-work" || category.Name != "工作" || !category.Visible || len(category.AppIDs) != 2 || category.AppIDs[1] != "uninstalled" {
		t.Fatal("category members or visibility changed in the protocol", category)
	}
}
