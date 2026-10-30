package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSignedArtifactIdentity(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	version, repo, commit := "0.1.0", "fixture/gopad", "abc123"
	var data bytes.Buffer
	zw := zip.NewWriter(&data)
	metadata, _ := json.Marshal(buildInfo{Schema: 1, Name: "gopad", BundleID: "dev.knothhe.gopad", Version: version, Channel: "release", Executor: "ci", Commit: commit, Target: "darwin-arm64", Source: "source-build"})
	w, err := zw.Create("gopad.app/Contents/Resources/build-info.json")
	if err != nil {
		t.Fatal(err)
	}
	w.Write(metadata)
	zw.Close()
	archiveName := "gopad-0.1.0-macos-arm64.zip"
	os.WriteFile(filepath.Join(dir, archiveName), data.Bytes(), 0644)
	digest := sha256.Sum256(data.Bytes())
	m := manifest{Version: version, URL: "https://github.com/" + repo + "/releases/download/v" + version + "/" + archiveName, Size: int64(data.Len()), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, digest[:]))}
	raw, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(dir, "update-darwin-arm64.json"), raw, 0644)
	key := base64.StdEncoding.EncodeToString(pub)
	if err := check(dir, version, repo, commit, key); err != nil {
		t.Fatal(err)
	}
	if err := check(dir, version, repo, "other-commit", key); err == nil {
		t.Fatal("accepted another source commit")
	}
	if err := check(dir, version, "another/repo", commit, key); err == nil {
		t.Fatal("accepted another repository")
	}
	if err := check(dir, version, repo, commit, base64.StdEncoding.EncodeToString(make([]byte, 32))); err == nil {
		t.Fatal("accepted another public key")
	}
	os.WriteFile(filepath.Join(dir, archiveName), append(data.Bytes(), 0), 0644)
	if err := check(dir, version, repo, commit, key); err == nil {
		t.Fatal("accepted changed archive")
	}
}
