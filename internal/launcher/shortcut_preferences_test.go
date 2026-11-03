package launcher

import "testing"

func TestNormalizeShortcutCanonicalizesSupportedMacLaunchKeys(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", ""}, {"  ", ""}, {"option + control + space", "Ctrl+Alt+Space"}, {"shift+command+g", "Cmd+Shift+G"}, {"F4", "F4"}, {"f20", "F20"}, {"Control+Return", "Ctrl+Enter"}, {"Meta+PageDown", "Cmd+PageDown"}, {"CmdOrCtrl+,", "Cmd+,"},
	} {
		got, err := NormalizeShortcut(tc.input)
		if err != nil || got != tc.want {
			t.Errorf("NormalizeShortcut(%q)=(%q,%v), want %q", tc.input, got, err, tc.want)
		}
	}
}

func TestNormalizeShortcutRejectsTypingAndUnsupportedKeys(t *testing.T) {
	for _, value := range []string{"G", "Shift+G", "Space", "Cmd+", "Cmd+Cmd+G", "Option+Alt+G", "Super+Meta+G", "Ctrl+Unknown", "F0", "F21", "F24", "F01", "Ctrl+Fn+G", "Ctrl+A+B", "Ctrl++", "Ctrl+🤔"} {
		if got, err := NormalizeShortcut(value); err == nil {
			t.Errorf("accepted %q as %q", value, got)
		}
	}
}
