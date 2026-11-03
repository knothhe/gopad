package main

import (
	"errors"
	"reflect"
	"testing"

	"github.com/knothhe/gopad/internal/launcher"
)

type nativeSettingsHarness struct {
	n           *nativePreferences
	registered  map[string]bool
	login       bool
	events      []string
	registerErr error
	loginErr    error
}

func settingsHarness() *nativeSettingsHarness {
	h := &nativeSettingsHarness{registered: map[string]bool{"Ctrl+Alt+Space": true}}
	h.n = &nativePreferences{shortcut: "Ctrl+Alt+Space", register: func(value string) error {
		h.events = append(h.events, "register:"+value)
		if h.registerErr != nil {
			return h.registerErr
		}
		h.registered[value] = true
		return nil
	}, unregister: func(value string) { h.events = append(h.events, "unregister:"+value); delete(h.registered, value) }, setLogin: func(value bool) error {
		if value {
			h.events = append(h.events, "login:on")
		} else {
			h.events = append(h.events, "login:off")
		}
		if h.loginErr != nil {
			return h.loginErr
		}
		h.login = value
		return nil
	}, login: func() bool { return h.login }}
	return h
}

func TestNativePreferencesKeepsWorkingShortcutUntilCommit(t *testing.T) {
	h := settingsHarness()
	finish, err := h.n.prepare(launcher.Preferences{}, launcher.Preferences{Shortcut: "Ctrl+Alt+G"})
	if err != nil {
		t.Fatal(err)
	}
	if !h.registered["Ctrl+Alt+Space"] || !h.registered["Ctrl+Alt+G"] || h.n.shortcut != "Ctrl+Alt+Space" {
		t.Fatal("working shortcut removed before preferences saved", h)
	}
	finish(true)
	if h.registered["Ctrl+Alt+Space"] || !h.registered["Ctrl+Alt+G"] || h.n.shortcut != "Ctrl+Alt+G" {
		t.Fatal("new shortcut not committed", h)
	}
	if !reflect.DeepEqual(h.events, []string{"register:Ctrl+Alt+G", "unregister:Ctrl+Alt+Space"}) {
		t.Fatal(h.events)
	}
}

func TestNativePreferencesDiskFailureRollsBackSystemChanges(t *testing.T) {
	h := settingsHarness()
	finish, err := h.n.prepare(launcher.Preferences{}, launcher.Preferences{Shortcut: "Ctrl+Alt+G", LaunchAtLogin: true})
	if err != nil || !h.login {
		t.Fatal("preparation failed", err, h)
	}
	finish(false)
	if !h.registered["Ctrl+Alt+Space"] || h.registered["Ctrl+Alt+G"] || h.login || h.n.shortcut != "Ctrl+Alt+Space" {
		t.Fatal("save failure did not restore actual system state", h)
	}
	// A rollback must also release the transaction lock for the next save.
	finish, err = h.n.prepare(launcher.Preferences{}, launcher.Preferences{Shortcut: "Ctrl+Alt+G"})
	if err != nil {
		t.Fatal(err)
	}
	finish(true)
}

func TestNativePreferencesUnavailableShortcutAndLoginFailureKeepOldShortcut(t *testing.T) {
	for _, failure := range []string{"shortcut", "login"} {
		t.Run(failure, func(t *testing.T) {
			h := settingsHarness()
			if failure == "shortcut" {
				h.registerErr = errors.New("claimed")
			} else {
				h.loginErr = errors.New("requires approval")
			}
			finish, err := h.n.prepare(launcher.Preferences{}, launcher.Preferences{Shortcut: "Ctrl+Alt+G", LaunchAtLogin: true})
			if err == nil || finish != nil || !h.registered["Ctrl+Alt+Space"] || h.registered["Ctrl+Alt+G"] || h.n.shortcut != "Ctrl+Alt+Space" {
				t.Fatal("failed preparation changed working shortcut", finish != nil, err, h)
			}
			h.registerErr, h.loginErr = nil, nil
			finish, err = h.n.prepare(launcher.Preferences{}, launcher.Preferences{Shortcut: "Ctrl+Alt+G"})
			if err != nil {
				t.Fatal("failed prepare retained transaction lock", err)
			}
			finish(true)
		})
	}
}

func TestNativePreferencesUnchangedSaveDoesNotRegisterAgain(t *testing.T) {
	h := settingsHarness()
	finish, err := h.n.prepare(launcher.Preferences{}, launcher.Preferences{Shortcut: "Ctrl+Alt+Space"})
	if err != nil {
		t.Fatal(err)
	}
	finish(true)
	if len(h.events) != 0 {
		t.Fatal("unrelated preference edit re-registered the working shortcut", h.events)
	}
}

func TestNativePreferencesLoginFailureRestoresPartialSystemChange(t *testing.T) {
	h := settingsHarness()
	// SMAppService can register an item, then report requires-approval as an
	// error. Such a failure still needs an unregister before returning.
	h.n.setLogin = func(value bool) error {
		h.login = value
		if value {
			return errors.New("requires approval")
		}
		return nil
	}
	finish, err := h.n.prepare(launcher.Preferences{}, launcher.Preferences{Shortcut: "Ctrl+Alt+G", LaunchAtLogin: true})
	if err == nil || finish != nil || h.login || !h.registered["Ctrl+Alt+Space"] || h.registered["Ctrl+Alt+G"] {
		t.Fatal("failed login update left a partial system registration", err, h.login, h.registered)
	}
}
