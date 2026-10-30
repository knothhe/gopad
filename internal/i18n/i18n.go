// Package i18n is the shared copy source for the native shell and web UI.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

//go:embed locales/*/*.json
var resources embed.FS

var catalogs = load()

func load() map[string]map[string]string {
	result := map[string]map[string]string{}
	for _, locale := range []string{"en", "zh-CN"} {
		result[locale] = map[string]string{}
		paths, err := fs.Glob(resources, "locales/"+locale+"/*.json")
		if err != nil {
			panic(err)
		}
		for _, path := range paths {
			data, err := resources.ReadFile(path)
			if err != nil {
				panic(err)
			}
			var part map[string]string
			if err := json.Unmarshal(data, &part); err != nil {
				panic(fmt.Errorf("%s: %w", path, err))
			}
			for key, value := range part {
				if _, exists := result[locale][key]; exists {
					panic("duplicate copy key: " + key)
				}
				result[locale][key] = value
			}
		}
	}
	return result
}

// Resolve keeps system mode distinct from its effective language. Unsupported
// script variants fall back to English, not a different Chinese writing system.
func Resolve(preference, system string) string {
	if preference == "" || preference == "system" {
		preference = system
	}
	tag := strings.ToLower(strings.ReplaceAll(preference, "_", "-"))
	if tag == "zh" || tag == "zh-cn" || tag == "zh-sg" || strings.HasPrefix(tag, "zh-hans") {
		return "zh-CN"
	}
	return "en"
}

// EnvironmentLocale serves the browser preview; desktop uses MyGo preferences.
func EnvironmentLocale() string {
	for _, key := range []string{"MYGO_LOCALE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(key); value != "" {
			return strings.Split(value, ".")[0]
		}
	}
	return "en"
}

func Messages(locale string) map[string]string {
	values := catalogs[Resolve(locale, "en")]
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

// T interpolates named placeholders using alternating name/value arguments.
// Values remain plain text; rendering surfaces escape them as needed.
func T(locale, key string, variables ...string) string {
	value, ok := catalogs[Resolve(locale, "en")][key]
	if !ok {
		value = catalogs["en"][key]
	}
	if value == "" {
		return key
	}
	args := make([]string, 0, len(variables))
	for i := 0; i+1 < len(variables); i += 2 {
		args = append(args, "{"+variables[i]+"}", variables[i+1])
	}
	if len(args) > 0 {
		value = strings.NewReplacer(args...).Replace(value)
	}
	return value
}
