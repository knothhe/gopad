package windowing

import "testing"

func TestLauncherUsesCurrentSpacePanelInsteadOfNativeFullscreen(t *testing.T) {
	options := LauncherOptions(false)
	if !options.Panel || !options.Frameless || !options.DisableFullScreen || options.FullScreen || options.Maximized {
		t.Fatalf("launcher is not a current-Space panel: %+v", options)
	}
	if !options.Hidden || !options.AlwaysOnTop || !options.DisableMove || !options.DisableResize {
		t.Fatalf("launcher can flash, move, or resize before placement: %+v", options)
	}
	if options.Parent != nil {
		t.Fatal("launcher must not be owned by the settings window")
	}
}

func TestDeveloperWindowRemainsWindowed(t *testing.T) {
	options := LauncherOptions(true)
	if options.Panel || options.Frameless || options.FullScreen || !options.DisableFullScreen {
		t.Fatalf("developer mode is not an ordinary window: %+v", options)
	}
	if options.DisableResize || options.DisableMove {
		t.Fatal("developer window should support moving and resizing")
	}
}

func TestSettingsIsIndependentOrdinaryWindow(t *testing.T) {
	options := ManagerOptions()
	if options.Parent != nil || options.Panel || options.Frameless || options.AlwaysOnTop || options.Modal || options.FullScreen {
		t.Fatalf("settings inherited launcher behavior: %+v", options)
	}
	if options.HideWindowButtons || options.TitleBarStyle != "" || options.DisableMove || options.DisableResize {
		t.Fatal("settings must keep its ordinary title bar, controls, and resizing")
	}
	if options.Width != 1080 || options.Height != 760 || options.URL != "/?manage=1" {
		t.Fatalf("wrong settings geometry or route: %+v", options)
	}
}
