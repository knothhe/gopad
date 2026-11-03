package main

import (
	"fmt"
	"sync"

	"github.com/knothhe/gopad/internal/buildinfo"
	"github.com/knothhe/gopad/internal/launcher"
)

func launchShortcut(value string) string {
	if value != "" {
		return value
	}
	if buildinfo.Channel == "release" {
		return "Ctrl+Alt+Space"
	}
	return "Ctrl+Alt+Shift+Space"
}

// Keep the working shortcut registered until persistence succeeds. A failed
// login-item or disk update restores the actual system state as well as the UI.
type nativePreferences struct {
	mu         sync.Mutex
	shortcut   string
	register   func(string) error
	unregister func(string)
	setLogin   func(bool) error
	login      func() bool
}

func (n *nativePreferences) prepare(previous, next launcher.Preferences) (func(bool), error) {
	n.mu.Lock()
	wanted := launchShortcut(next.Shortcut)
	changed := wanted != n.shortcut
	if changed {
		if err := n.register(wanted); err != nil {
			n.mu.Unlock()
			return nil, fmt.Errorf("error.shortcutUnavailable")
		}
	}
	loginChanged := previous.LaunchAtLogin != next.LaunchAtLogin
	loginBefore := previous.LaunchAtLogin
	if loginChanged {
		loginBefore = n.login()
		if err := n.setLogin(next.LaunchAtLogin); err != nil {
			_ = n.setLogin(loginBefore)
			if changed {
				n.unregister(wanted)
			}
			n.mu.Unlock()
			return nil, fmt.Errorf("error.loginUnavailable")
		}
	}
	return func(saved bool) {
		defer n.mu.Unlock()
		if saved {
			if changed {
				if n.shortcut != "" {
					n.unregister(n.shortcut)
				}
				n.shortcut = wanted
			}
		} else {
			if changed {
				n.unregister(wanted)
			}
			if loginChanged {
				_ = n.setLogin(loginBefore)
			}
		}
	}, nil
}
