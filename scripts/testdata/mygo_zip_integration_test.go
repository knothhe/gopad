//go:build darwin && !js

// This test is overlaid into the pinned MyGo package by the Python integration
// test, so it exercises the real private installer and exit-time bundle helper.
package mygo

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/internal/update"
)

func gopadUpdateZIP(t *testing.T, filename string) []byte {
	t.Helper()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	for _, entry := range []struct {
		name, content string
		mode          os.FileMode
	}{
		{filename, "new version", 0755},
		{"gopad.app/Contents/Resources/value", "resource", 0644},
		{"gopad.app/Contents/Resources/current", "value", os.ModeSymlink | 0777},
		{"__MACOSX/gopad.app/._Contents", "AppleDouble", 0644},
	} {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		header.SetMode(entry.mode)
		file, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(entry.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func gopadInstalledBundle(t *testing.T) (string, string) {
	t.Helper()
	target := filepath.Join(t.TempDir(), "gopad.app")
	main := filepath.Join(target, "Contents/MacOS/gopad")
	if err := os.MkdirAll(filepath.Dir(main), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(main, []byte("old version"), 0755); err != nil {
		t.Fatal(err)
	}
	return target, main
}

func gopadAssertMain(t *testing.T, main, want string) {
	t.Helper()
	data, err := os.ReadFile(main)
	if err != nil || string(data) != want {
		t.Fatalf("installed executable = %q, %v; want %q", data, err, want)
	}
}

func TestGopadSignedZIPStagesUntilExit(t *testing.T) {
	archive := gopadUpdateZIP(t, "gopad.app/Contents/MacOS/gopad")
	pub, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(archive)
	m := update.Manifest{Version: "0.1.2", Size: int64(len(archive)), Signature: update.Sign(private, digest[:])}
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	m.URL = server.URL + "/gopad-0.1.2-macos-arm64.zip"
	mux.HandleFunc("/update.json", func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(m) })
	mux.HandleFunc("/gopad-0.1.2-macos-arm64.zip", func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	key := base64.StdEncoding.EncodeToString(pub)
	up, err := checkUpdate(context.Background(), feedSource(server.URL+"/update.json"), key, "0.1.1", false)
	if err != nil || up == nil || up.Version != "0.1.2" {
		t.Fatalf("framework checkUpdate = %v, %v", up, err)
	}
	target, main := gopadInstalledBundle(t)
	app := exec.Command("/bin/sleep", "60")
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	oldPID := appPid
	appPid = app.Process.Pid
	t.Cleanup(func() {
		appPid = oldPID
		if app.ProcessState == nil {
			_ = app.Process.Kill()
			_ = app.Wait()
		}
		if pending := pendingUpdate.Swap(nil); pending != nil {
			_ = pending.helper.Process.Kill()
			_ = pending.helper.Wait()
			_ = os.RemoveAll(pending.work)
		}
	})
	var progress [2]int64
	err = installUpdate(context.Background(), httpGet, up.manifest, key, up.current, target, func(done, total int64) {
		progress = [2]int64{done, total}
	})
	if err != nil {
		t.Fatal(err)
	}
	gopadAssertMain(t, main, "old version")
	pending := pendingUpdate.Load()
	if pending == nil {
		t.Fatal("framework did not stage the ZIP bundle")
	}
	gopadAssertMain(t, filepath.Join(pending.bundle, "Contents/MacOS/gopad"), "new version")
	if progress != [2]int64{m.Size, m.Size} {
		t.Fatalf("download progress = %v, want %d", progress, m.Size)
	}
	_ = app.Process.Kill()
	_ = app.Wait()
	pending = pendingUpdate.Swap(nil)
	if err := pending.helper.Wait(); err != nil {
		t.Fatal(err)
	}
	gopadAssertMain(t, main, "new version")
	info, err := os.Stat(main)
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("executable permissions lost: %v, %v", info, err)
	}
	resource, err := os.ReadFile(filepath.Join(target, "Contents/Resources/current"))
	if err != nil || string(resource) != "resource" {
		t.Fatalf("updated resource link = %q, %v", resource, err)
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil || len(entries) != 1 || entries[0].Name() != "gopad.app" {
		t.Fatalf("helper left update/AppleDouble files: %v, %v", entries, err)
	}
}

func TestGopadSignedZIPRejectsInvalidUpdatesBeforeStaging(t *testing.T) {
	pub, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	valid := gopadUpdateZIP(t, "gopad.app/Contents/MacOS/gopad")
	for _, scenario := range []string{"wrong signature", "wrong size", "tampered bytes", "signed unsafe ZIP"} {
		t.Run(scenario, func(t *testing.T) {
			archive := bytes.Clone(valid)
			if scenario == "signed unsafe ZIP" {
				archive = gopadUpdateZIP(t, "gopad.app/../escaped")
			}
			digest := sha256.Sum256(archive)
			m := update.Manifest{Version: "0.1.2", Size: int64(len(archive)), Signature: update.Sign(private, digest[:])}
			switch scenario {
			case "wrong signature":
				m.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
			case "wrong size":
				m.Size++
			case "tampered bytes":
				archive[len(archive)/2] ^= 1
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(archive) }))
			defer server.Close()
			m.URL = server.URL + "/update.zip"
			target, main := gopadInstalledBundle(t)
			err := installUpdate(context.Background(), httpGet, m, base64.StdEncoding.EncodeToString(pub), "0.1.1", target, nil)
			if err == nil {
				t.Fatal("framework accepted an invalid ZIP update")
			}
			gopadAssertMain(t, main, "old version")
			if pendingUpdate.Load() != nil {
				t.Fatal("invalid ZIP update started the install helper")
			}
			entries, err := os.ReadDir(filepath.Dir(target))
			if err != nil || len(entries) != 1 {
				t.Fatalf("failed update left staging files: %v, %v", entries, err)
			}
		})
	}
}
