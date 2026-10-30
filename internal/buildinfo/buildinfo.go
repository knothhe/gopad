// Package buildinfo exposes the identity injected by scripts/macos.py.
// A go run / go test binary remains a Dev build without an update trust root.
package buildinfo

var (
	Version    = "dev"
	Channel    = "dev"
	Executor   = "local"
	Commit     = "unknown"
	Repository = ""
	PublicKey  = ""
)

func Name() string {
	if Channel == "release" {
		return "gopad"
	}
	return "gopad Dev"
}

func BundleID() string {
	if Channel == "release" {
		return "dev.knothhe.gopad"
	}
	return "dev.knothhe.gopad.dev"
}

func DataName() string { return Name() }

func UpdatesConfigured() bool {
	return Channel == "release" && Repository != "" && PublicKey != ""
}
