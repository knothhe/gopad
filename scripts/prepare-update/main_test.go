package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSharedZIPManifestUsesPinnedKeyAndExactBytes(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	archive := filepath.Join(dir, "gopad-0.1.1-macos-arm64.zip")
	data := []byte("ZIP fixture bytes")
	if err := os.WriteFile(archive, data, 0644); err != nil {
		t.Fatal(err)
	}
	pub := base64.StdEncoding.EncodeToString(public)
	priv := base64.StdEncoding.EncodeToString(private)
	if err := prepare(archive, "0.1.1", "fixture/gopad", pub, "release notes", priv); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "update-darwin-arm64.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	signature, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil || !ed25519.Verify(public, digest[:], signature) {
		t.Fatal("manifest does not sign shared ZIP bytes")
	}
	if m.Version != "0.1.1" || m.Size != int64(len(data)) || m.Notes != "release notes" || m.URL != "https://github.com/fixture/gopad/releases/download/v0.1.1/gopad-0.1.1-macos-arm64.zip" {
		t.Fatalf("manifest mismatch: %+v", m)
	}
	if err := prepare(archive, "0.1.1", "fixture/gopad", pub, "", ""); err == nil {
		t.Fatal("accepted missing private key")
	}
	other, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepare(archive, "0.1.1", "fixture/gopad", base64.StdEncoding.EncodeToString(other), "", priv); err == nil {
		t.Fatal("accepted mismatched trust root")
	}
	if err := prepare(archive, "0.1.2", "fixture/gopad", pub, "", priv); err == nil {
		t.Fatal("accepted another version's ZIP")
	}
	broken := append([]byte(nil), private...)
	broken[0] ^= 1
	if err := prepare(archive, "0.1.1", "fixture/gopad", pub, "", base64.StdEncoding.EncodeToString(broken)); err == nil {
		t.Fatal("accepted inconsistent private key seed/public half")
	}
	if err := os.WriteFile(archive, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := prepare(archive, "0.1.1", "fixture/gopad", pub, "", priv); err == nil {
		t.Fatal("accepted empty ZIP")
	}
}
