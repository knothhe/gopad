// Package updates bridges the app's settings to MyGo's signed native updater.
// Downloading, validation, installation, recovery and scheduling remain owned
// by the framework, rather than a second installer in the application.
package updates

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/knothhe/gopad/internal/buildinfo"
	"github.com/knothhe/gopad/internal/webserver"
)

type Controller struct {
	active bool
	dir    string
	mu     sync.Mutex
}

// Register must run after the channel's data directory is configured and before
// App.Run. Dev/preview builds never register a plugin that could use a feed.
func Register(dataDir string, icon []byte) *Controller {
	c := &Controller{dir: dataDir}
	if buildinfo.UpdatesConfigured() {
		mygo.Use(native.New(updater.Options{Icon: icon}))
		c.active = true
	}
	return c
}

func eligibility(channel string, configured, writable bool) string {
	if channel != "release" {
		return "development"
	}
	if !configured {
		return "not_configured"
	}
	if !writable {
		return "unavailable"
	}
	return "ready"
}

func (c *Controller) State() webserver.UpdateState {
	state := webserver.UpdateState{CurrentVersion: buildinfo.Version}
	state.Status = eligibility(buildinfo.Channel, c.active, c.active && mygo.Updater.Enabled())
	if c.active {
		state.AutomaticChecks = updater.AutomaticChecks()
		if last := updater.LastCheck(); !last.IsZero() {
			state.LastCheck = last.UTC().Format(time.RFC3339)
		}
	}
	return state
}

func (c *Controller) Check() {
	if c.State().Status == "ready" {
		updater.CheckForUpdates()
	}
}

// The framework setter has no error return. Confirm its persisted value before
// reporting success, restoring the in-memory choice if writing failed.
func (c *Controller) SetAutomaticChecks(value bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.State().Status != "ready" {
		return errors.New("updates unavailable")
	}
	before := updater.AutomaticChecks()
	updater.SetAutomaticChecks(value)
	if err := verifyChecks(filepath.Join(c.dir, "updater.json"), value); err != nil {
		updater.SetAutomaticChecks(before)
		return err
	}
	return nil
}

func verifyChecks(path string, want bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var saved struct {
		AutomaticChecks *bool `json:"automaticChecks"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}
	if saved.AutomaticChecks == nil || *saved.AutomaticChecks != want {
		return errors.New("update preference was not persisted")
	}
	return nil
}
