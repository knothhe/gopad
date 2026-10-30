// Sign the same ZIP used for manual installation using MyGo's manifest format.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type manifest struct {
	Version   string `json:"version"`
	Notes     string `json:"notes,omitempty"`
	Date      string `json:"date"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	Signature string `json:"signature"`
}

func prepare(archive, version, repository, publicKey, notes, privateKey string) error {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privateKey))
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid update signing key")
	}
	private := ed25519.PrivateKey(key)
	if !bytes.Equal(ed25519.NewKeyFromSeed(private.Seed()), private) {
		return fmt.Errorf("invalid update signing key")
	}
	if base64.StdEncoding.EncodeToString(private.Public().(ed25519.PublicKey)) != publicKey {
		return fmt.Errorf("update signing key differs from pinned public key")
	}
	if filepath.Base(archive) != "gopad-"+version+"-macos-arm64.zip" {
		return fmt.Errorf("unexpected ZIP filename")
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > 1<<30 {
		return fmt.Errorf("invalid ZIP size")
	}
	digest := sha256.Sum256(data)
	m := manifest{Version: version, Notes: notes, Date: time.Now().UTC().Format(time.RFC3339),
		URL:  "https://github.com/" + repository + "/releases/download/v" + version + "/" + filepath.Base(archive),
		Size: int64(len(data)), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, digest[:]))}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(filepath.Dir(archive), "update-darwin-arm64.json"), append(raw, '\n'), 0644)
}

func main() {
	archive := flag.String("archive", "", "shared release ZIP")
	version := flag.String("version", "", "release version")
	repository := flag.String("repo", "", "owner/repository")
	publicKey := flag.String("public-key", "", "pinned Ed25519 public key")
	notesFile := flag.String("notes", "", "release notes file")
	flag.Parse()
	notes, err := os.ReadFile(*notesFile)
	if err == nil {
		// The build changelog has exactly one version heading.
		_, body, _ := strings.Cut(string(notes), "\n")
		err = prepare(*archive, *version, *repository, *publicKey, strings.TrimSpace(body), os.Getenv("MYGO_UPDATER_PRIVATE_KEY"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
