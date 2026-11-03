package webserver

import (
	"errors"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/knothhe/gopad/internal/launcher"
)

type preferencesCatalog struct {
	mockCatalog
	preferences launcher.Preferences
	saves       int
	saveErr     error
}

func (c *preferencesCatalog) State() (launcher.State, error) {
	return launcher.State{Preferences: c.preferences}, nil
}
func (c *preferencesCatalog) SavePreferences(p launcher.Preferences) (launcher.State, error) {
	c.saves++
	if c.saveErr != nil {
		return launcher.State{}, c.saveErr
	}
	c.preferences = p
	return c.State()
}
func preferencesHandler(c *preferencesCatalog, controls Controls) http.Handler {
	return New(c, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("settings")}}, fstest.MapFS{}, controls)
}

func TestPreferenceShortcutValidationAndAvailabilityDoNotPersist(t *testing.T) {
	c := &preferencesCatalog{preferences: launcher.Preferences{Shortcut: "Ctrl+Alt+Space"}}
	prepares, changed := 0, 0
	h := preferencesHandler(c, Controls{PreparePreferences: func(previous, next launcher.Preferences) (func(bool), error) {
		prepares++
		return nil, errors.New("error.shortcutUnavailable")
	}, PreferencesChanged: func(launcher.Preferences) { changed++ }})
	w := request(h, "PUT", "/api/preferences", `{"shortcut":"Shift+G"}`, "")
	if w.Code != http.StatusBadRequest || prepares != 0 || c.saves != 0 || changed != 0 {
		t.Fatal("invalid syntax reached native registration or persistence", w.Code, prepares, c.saves, changed)
	}
	w = request(h, "PUT", "/api/preferences", `{"shortcut":"Ctrl+Alt+G"}`, "")
	if w.Code != http.StatusBadRequest || prepares != 1 || c.saves != 0 || changed != 0 || c.preferences.Shortcut != "Ctrl+Alt+Space" {
		t.Fatal("unavailable shortcut persisted", w.Code, prepares, c.saves, changed, c.preferences)
	}
}

func TestPreferenceSaveFailureRollsBackPreparedSystemChanges(t *testing.T) {
	c := &preferencesCatalog{preferences: launcher.Preferences{Shortcut: "Ctrl+Alt+Space"}, saveErr: errors.New("disk full")}
	var finish []bool
	changed := 0
	h := preferencesHandler(c, Controls{PreparePreferences: func(previous, next launcher.Preferences) (func(bool), error) {
		if previous.Shortcut != "Ctrl+Alt+Space" || next.Shortcut != "Ctrl+Alt+G" {
			t.Fatal("invalid prepared state", previous, next)
		}
		return func(saved bool) { finish = append(finish, saved) }, nil
	}, PreferencesChanged: func(launcher.Preferences) { changed++ }})
	w := request(h, "PUT", "/api/preferences", `{"shortcut":" option + ctrl + g "}`, "")
	if w.Code != http.StatusBadRequest || !reflect.DeepEqual(finish, []bool{false}) || changed != 0 || c.preferences.Shortcut != "Ctrl+Alt+Space" {
		t.Fatal(w.Code, finish, changed, c.preferences)
	}
	c.saveErr = nil
	w = request(h, "PUT", "/api/preferences", `{"shortcut":"Ctrl+Alt+G"}`, "")
	if w.Code != http.StatusOK || !reflect.DeepEqual(finish, []bool{false, true}) || changed != 1 || c.preferences.Shortcut != "Ctrl+Alt+G" {
		t.Fatal(w.Code, finish, changed, c.preferences)
	}
}

func TestPreferenceTransactionsSerializeConcurrentNativeChanges(t *testing.T) {
	c := &preferencesCatalog{preferences: launcher.Preferences{Shortcut: "Ctrl+Alt+Space"}}
	current := "Ctrl+Alt+Space"
	h := preferencesHandler(c, Controls{PreparePreferences: func(previous, next launcher.Preferences) (func(bool), error) {
		if previous.Shortcut != current {
			t.Errorf("native and persisted previous shortcut diverged: %q != %q", previous.Shortcut, current)
		}
		return func(saved bool) {
			if saved {
				current = next.Shortcut
			}
		}, nil
	}})
	var wg sync.WaitGroup
	for _, body := range []string{`{"shortcut":"Ctrl+Alt+G"}`, `{"shortcut":"Ctrl+Alt+H"}`} {
		wg.Add(1)
		go func(body string) {
			defer wg.Done()
			if w := request(h, "PUT", "/api/preferences", body, ""); w.Code != http.StatusOK {
				t.Errorf("concurrent save failed: %d", w.Code)
			}
		}(body)
	}
	wg.Wait()
	if c.saves != 2 || c.preferences.Shortcut != current {
		t.Fatal("transactions did not commit consistently", c.preferences, current, c.saves)
	}
}
