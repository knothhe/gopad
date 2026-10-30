// Verify the complete archive produced by MyGo, without performing an installation.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type manifest struct {
	Version, URL, Signature string
	Size                    int64
}
type buildInfo struct {
	Schema                                                             int
	Name, BundleID, Version, Channel, Executor, Commit, Target, Source string
}

func check(dir, version, repository, commit, publicKey string) error {
	target := "darwin-arm64"
	names := []string{"gopad-" + version + "-macos-arm64.zip", "gopad-" + version + "-" + target + ".tar.gz", "update-" + target + ".json"}
	contents, err := os.ReadFile(filepath.Join(dir, names[2]))
	if err != nil {
		return err
	}
	var m manifest
	if err := json.Unmarshal(contents, &m); err != nil {
		return err
	}
	expectedURL := "https://github.com/" + repository + "/releases/download/v" + version + "/" + names[1]
	if m.Version != version || m.URL != expectedURL || m.Size <= 0 || m.Size > 1<<30 {
		return fmt.Errorf("manifest target/version/size mismatch")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(publicKey))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid pinned update public key")
	}
	signature, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return err
	}
	archive, err := os.ReadFile(filepath.Join(dir, names[1]))
	if err != nil {
		return err
	}
	digest := sha256.Sum256(archive)
	if int64(len(archive)) != m.Size || !ed25519.Verify(key, digest[:], signature) {
		return fmt.Errorf("update archive signature or size mismatch")
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(io.LimitReader(gz, 1<<30))
	var found bool
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Name != "gopad.app/Contents/Resources/build-info.json" {
			continue
		}
		if found || header.Size > 16384 {
			return fmt.Errorf("invalid build metadata in archive")
		}
		found = true
		var meta buildInfo
		if err := json.NewDecoder(io.LimitReader(tr, 16384)).Decode(&meta); err != nil {
			return err
		}
		if meta.Schema != 1 || meta.Name != "gopad" || meta.BundleID != "dev.knothhe.gopad" || meta.Version != version || meta.Channel != "release" || meta.Executor != "ci" || meta.Commit != commit || meta.Target != target || meta.Source != "source-build" {
			return fmt.Errorf("release identity does not match tagged CI build")
		}
	}
	if !found {
		return fmt.Errorf("archive lacks build identity")
	}
	var sums strings.Builder
	for _, name := range names {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		h := sha256.New()
		n, err := io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("empty release artifact %s", name)
		}
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(h.Sum(nil)), name)
	}
	// Only the release preparation step writes this file; verification compares it.
	sumPath := filepath.Join(dir, "SHA256SUMS")
	old, err := os.ReadFile(sumPath)
	if err == nil && string(old) != sums.String() {
		return fmt.Errorf("release checksum list mismatch")
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		return os.WriteFile(sumPath, []byte(sums.String()), 0644)
	}
	return nil
}
func main() {
	dir := flag.String("dir", "build/release/darwin-arm64", "artifact directory")
	version := flag.String("version", "", "VERSION")
	repo := flag.String("repo", "", "owner/repository")
	commit := flag.String("commit", "", "tagged source commit")
	keyFile := flag.String("config", "config/updates.json", "public update configuration")
	flag.Parse()
	data, err := os.ReadFile(*keyFile)
	if err == nil {
		var config struct{ PublicKey string }
		err = json.Unmarshal(data, &config)
		if err == nil {
			err = check(*dir, *version, *repo, *commit, config.PublicKey)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Verified exact release artifacts and MyGo update signature.")
}
