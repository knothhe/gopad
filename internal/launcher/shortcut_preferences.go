package launcher

import (
	"errors"
	"strconv"
	"strings"
)

// NormalizeShortcut accepts a deliberately small, portable set of launch keys.
// Empty means the build channel's default. Modifiers prevent ordinary typing
// keys from accidentally becoming global shortcuts; function keys stand alone.
func NormalizeShortcut(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	invalid := errors.New("error.invalidShortcut")
	parts := strings.Split(value, "+")
	mods := map[string]bool{}
	for _, raw := range parts[:len(parts)-1] {
		var modifier string
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "cmd", "command", "super", "meta", "cmdorctrl":
			modifier = "Cmd"
		case "ctrl", "control":
			modifier = "Ctrl"
		case "alt", "option":
			modifier = "Alt"
		case "shift":
			modifier = "Shift"
		default:
			return "", invalid
		}
		if mods[modifier] {
			return "", invalid
		}
		mods[modifier] = true
	}
	key := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
	function := false
	if strings.HasPrefix(key, "f") {
		n, err := strconv.Atoi(key[1:])
		if err == nil && n >= 1 && n <= 20 && strconv.Itoa(n) == key[1:] {
			key = "F" + strconv.Itoa(n)
			function = true
		}
	}
	if !function {
		named := map[string]string{"space": "Space", "tab": "Tab", "return": "Enter", "enter": "Enter", "escape": "Escape", "esc": "Escape", "up": "Up", "down": "Down", "left": "Left", "right": "Right", "home": "Home", "end": "End", "pageup": "PageUp", "pagedown": "PageDown", "backspace": "Backspace", "delete": "Delete"}
		if canonical, ok := named[key]; ok {
			key = canonical
		} else if len(key) == 1 && (key[0] >= 'a' && key[0] <= 'z' || key[0] >= '0' && key[0] <= '9' || strings.ContainsAny(key, ",./;[]-=")) {
			key = strings.ToUpper(key)
		} else {
			return "", invalid
		}
		if !mods["Cmd"] && !mods["Ctrl"] && !mods["Alt"] {
			return "", invalid
		}
	}
	var result []string
	for _, mod := range []string{"Cmd", "Ctrl", "Alt", "Shift"} {
		if mods[mod] {
			result = append(result, mod)
		}
	}
	return strings.Join(append(result, key), "+"), nil
}
