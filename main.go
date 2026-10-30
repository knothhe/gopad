package main

import (
	"bytes"
	"context"
	"embed"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"
	"github.com/knothhe/gopad/internal/buildinfo"
	"github.com/knothhe/gopad/internal/i18n"
	"github.com/knothhe/gopad/internal/launcher"
	"github.com/knothhe/gopad/internal/launchview"
	"github.com/knothhe/gopad/internal/updates"
	"github.com/knothhe/gopad/internal/webserver"
	"github.com/knothhe/gopad/internal/windowing"
)

//go:embed frontend assets
var files embed.FS

func main() {
	locale := i18n.Resolve("system", i18n.EnvironmentLocale())
	preview := flag.Bool("preview", false, i18n.T(locale, "app.previewFlag"))
	listen := flag.String("listen", "127.0.0.1:5189", i18n.T(locale, "app.listenFlag"))
	windowed := flag.Bool("windowed", false, i18n.T(locale, "app.windowedFlag"))
	config := flag.String("config", "", i18n.T(locale, "app.configFlag"))
	flag.Parse()
	if *config == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		*config = filepath.Join(dir, buildinfo.DataName(), "preferences.json")
	}
	configPath, err := filepath.Abs(*config)
	if err != nil {
		log.Fatal(err)
	}
	dataDir := filepath.Dir(configPath)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Fatal(err)
	}
	mygo.App.SetName(buildinfo.Name())
	mygo.App.SetVersion(buildinfo.Version)
	mygo.App.SetPath(mygo.PathUserData, dataDir)
	service, err := launcher.New(*config)
	if err != nil {
		log.Fatal(err)
	}
	frontend, _ := fs.Sub(files, "frontend")
	assets, _ := fs.Sub(files, "assets")
	if *preview {
		runPreview(service, frontend, assets, *listen)
		return
	}
	if runtime.GOOS != "darwin" {
		log.Fatal("gopad 目前支持 macOS；使用 --preview 可以预览界面")
	}
	runDesktop(service, frontend, assets, *windowed, dataDir)
}

func runDesktop(service *launcher.Service, frontend, assets fs.FS, windowed bool, dataDir string) {
	app := mygo.App
	app.SetLanguages("en", "zh-Hans")
	var appliedLanguage atomic.Value
	applyLanguage := func(preference string) {
		if last := appliedLanguage.Load(); last != nil && last.(string) == preference {
			return
		}
		appliedLanguage.Store(preference)
		if preference == "system" || preference == "" {
			app.SetLocale("")
		} else {
			app.SetLocale(i18n.Resolve(preference, "en"))
		}
	}
	applyLanguage(service.Preferences().Language)
	var systemLocale atomic.Value
	systemLocale.Store("en")
	locale := func() string { return i18n.Resolve(appliedLanguage.Load().(string), systemLocale.Load().(string)) }
	text := func(key string) string { return i18n.T(locale(), key) }
	icon := applicationIcon()
	updateController := updates.Register(dataDir, icon)
	mygo.Theme.SetSource(mygo.ThemeDark)
	if !app.RequestSingleInstanceLock() {
		return
	}
	var win, manager *mygo.Window
	var pad *launchview.Launchpad
	var fader *windowing.Fader
	var tray *mygo.Tray
	var trayMenu *mygo.Menu
	quitting := false
	var closeManager func(bool)
	app.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if quitting {
			return
		}
		if manager != nil {
			e.PreventDefault()
			closeManager(true)
			return
		}
		quitting = true
	})
	showMain := func() {
		if win == nil || fader == nil || quitting {
			return
		}
		pad.ResetSearch()
		pad.Reload()
		if !windowed {
			bounds := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint()).Bounds
			windowing.PlaceOverlay(win, bounds)
		}
		fader.Show(mygo.Theme.Preferences().ReduceMotion)
		win.Focus()
		win.Invalidate()
	}
	show := func() { showMain() }
	hide := func() {
		if win != nil {
			// Launch and HTTP callbacks may arrive on worker goroutines.
			win.Update(func() {
				if !quitting && fader != nil {
					fader.Hide(mygo.Theme.Preferences().ReduceMotion)
				}
			})
		}
	}
	var openManager func()
	openManager = func() {
		if quitting || win == nil {
			return
		}
		// Settings is an ordinary independent window, not a child of the
		// screen overlay. Showing it never leaves a full-screen Space.
		if fader != nil {
			fader.HideImmediately()
		}
		if manager != nil {
			manager.Show()
			manager.Focus()
			return
		}
		managerOptions := windowing.ManagerOptions()
		managerOptions.Title = i18n.T(locale(), "app.managerTitle", "name", buildinfo.Name())
		manager = mygo.NewWindow(managerOptions)
		managed := manager
		closer := managerCloser{
			prepare: func(done func(bool)) {
				// Eval waits for JavaScript promises. Keep the UI thread free so
				// the page can send and finish its outstanding preference saves.
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					result, err := managed.Page().EvalContext(ctx, "typeof window.__gopadPrepareClose === 'function' && window.__gopadPrepareClose()")
					cancel()
					ok := err == nil && result == true
					if !ok {
						// Cancel also invalidates an older JS preparation after a
						// timeout, so its eventual completion cannot freeze this page.
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						_, cancelErr := managed.Page().EvalContext(ctx, "typeof window.__gopadCancelClose === 'function' && window.__gopadCancelClose()")
						cancel()
						if err != nil {
							log.Printf("prepare settings close: %v", err)
						}
						if cancelErr != nil {
							log.Printf("cancel settings close: %v", cancelErr)
						}
					}
					managed.Update(func() {
						if manager != managed || managed.IsDestroyed() {
							done(false)
							return
						}
						if !ok {
							managed.Show()
							managed.Focus()
						}
						done(ok)
					})
				}()
			},
			destroy: managed.Destroy,
			quit: func() {
				quitting = true
				app.Quit()
			},
		}
		closeManager = closer.request
		managed.OnClose(func(e *mygo.CloseEvent) {
			if !quitting {
				e.PreventDefault()
				closer.request(false)
			}
		})
		managed.OnReadyToShow(func() {
			if !quitting {
				managed.Show()
				managed.Focus()
			}
		})
		managed.OnClosed(func() { manager = nil; closeManager = nil })
	}
	toggle := func() {
		if win == nil {
			return
		}
		if fader != nil && fader.DesiredVisible() && win.IsFocused() {
			hide()
		} else {
			show()
		}
	}
	controls := webserver.Controls{
		Hide:               hide,
		CopyPath:           func(path string) error { mygo.Clipboard.WriteText(path); return nil },
		Quit:               func() { go func() { time.Sleep(100 * time.Millisecond); app.Quit() }() },
		SystemLocale:       func() string { return systemLocale.Load().(string) },
		PreferencesChanged: func(launcher.Preferences) { applyLanguage(service.Preferences().Language) },
		Version:            buildinfo.Version,
		UpdateStatus:       updateController.State,
		SetAutomaticChecks: updateController.SetAutomaticChecks,
		CheckUpdates: func() {
			if win != nil {
				win.Update(func() {
					if fader != nil {
						fader.HideImmediately()
					}
					updateController.Check()
				})
			}
		},
		OpenManager: func() {
			if win != nil {
				win.Update(openManager)
			}
		},
	}
	refreshMenus := func() {
		entries := func() []*mygo.MenuItem {
			return []*mygo.MenuItem{
				{Label: text("app.show"), Accelerator: "CmdOrCtrl+L", Click: func(*mygo.MenuItem, *mygo.Window) { show() }},
				{Label: text("app.manage"), Accelerator: "CmdOrCtrl+,", Click: func(*mygo.MenuItem, *mygo.Window) { openManager() }},
				{Label: text("app.checkUpdates"), Disabled: updateController.State().Status != "ready", Click: func(*mygo.MenuItem, *mygo.Window) { controls.CheckUpdates() }},
				mygo.Separator(), {Role: mygo.RoleQuit},
			}
		}
		app.SetMenu(mygo.NewMenu([]*mygo.MenuItem{{Label: buildinfo.Name(), Submenu: entries()}, {Role: mygo.RoleEditMenu}}))
		trayMenu = mygo.NewMenu(entries())
		if tray != nil {
			tray.SetToolTip(i18n.T(locale(), "app.tray", "name", buildinfo.Name()))
		}
		if manager != nil {
			manager.SetTitle(i18n.T(locale(), "app.managerTitle", "name", buildinfo.Name()))
		}
		if win != nil {
			win.Invalidate()
		}
	}

	app.OnWindowAllClosed(func() {})
	// MyGo's OnActivate is the Dock-reopen event, not every application
	// activation. Hiding a panel cannot cause it to reappear through focus.
	app.OnActivate(func(bool) {
		if !quitting {
			showMain()
		}
	})
	app.OnSecondInstance(func([]string, string) { show() })
	app.WhenReady(func() {
		updateSystemLocale := func() {
			preferred := app.PreferredLanguages()
			if len(preferred) > 0 {
				systemLocale.Store(preferred[0])
			}
		}
		updateSystemLocale()
		refreshMenus()
		app.OnLocaleChanged(func(string) { updateSystemLocale(); refreshMenus() })
		pad = launchview.New(service, launchview.Callbacks{
			Hide: hide, OpenManager: openManager,
			Locale: locale,
			Invalidate: func() {
				if win != nil {
					win.Invalidate()
				}
			},
		})
		if wallpaper, err := launcher.Wallpaper(); err == nil {
			_ = pad.SetWallpaper(wallpaper)
			controls.Wallpaper = wallpaper
		} else {
			log.Printf("桌面壁纸不可用，使用渐变背景: %v", err)
		}
		// Register the scheme after UI-thread wallpaper loading and before
		// creating any window that uses it.
		if err := mygo.Protocol.Handle("mygo", webserver.New(service, frontend, assets, controls)); err != nil {
			log.Fatal(err)
		}
		options := windowing.LauncherOptions(windowed)
		options.Title = buildinfo.Name()
		options.Content = ui.View(pad.View)
		win = mygo.NewWindow(options)
		fader = windowing.NewFader(win, func(fn func()) { win.Update(fn) })
		win.OnClose(func(e *mygo.CloseEvent) {
			if !quitting {
				e.PreventDefault()
				hide()
			}
		})
		if !windowed {
			win.OnBlur(func() {
				// A panel does not deactivate its owning app. Recheck actual
				// focus after the event so an older blur cannot dismiss a
				// launcher that has already been brought forward again.
				win.Update(func() {
					if !quitting && fader.DesiredVisible() && !win.IsFocused() {
						fader.Hide(mygo.Theme.Preferences().ReduceMotion)
					}
				})
			})
		}
		if len(icon) > 0 {
			_ = app.Dock.SetIcon(icon)
		}
		var err error
		tray, err = mygo.NewTray(mygo.TrayOptions{ID: buildinfo.BundleID() + ".launcher", Icon: trayIcon(), IconIsTemplate: true, ToolTip: i18n.T(locale(), "app.tray", "name", buildinfo.Name())})
		if err != nil {
			log.Printf("菜单栏图标: %v", err)
		} else {
			tray.OnClick(toggle)
			tray.OnRightClick(func() { tray.SetMenu(trayMenu); defer tray.SetMenu(nil); tray.PopUpMenu() })
			app.OnWillQuit(func(*mygo.QuitEvent) { tray.Destroy() })
		}
		shortcut := "Ctrl+Alt+Space"
		if buildinfo.Channel != "release" {
			shortcut = "Ctrl+Alt+Shift+Space"
		}
		if err := mygo.GlobalShortcut.Register(shortcut, toggle); err != nil {
			log.Printf("全局快捷键 Control+Option+Space 不可用: %v", err)
		}
		app.OnWillQuit(func(*mygo.QuitEvent) { mygo.GlobalShortcut.UnregisterAll(); fader.Stop(); pad.Close() })
		mygo.Screen.OnDisplaysChanged(func() {
			if !windowed && win.IsVisible() {
				windowing.PlaceOverlay(win, mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint()).Bounds)
				win.Invalidate()
			}
		})
		showMain()
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// managerCloser is owned by the UI thread, including the completion callback.
// Closing and quitting share one save barrier; a quit request may arrive while
// the page is still preparing an ordinary window close.
type managerCloser struct {
	preparing     bool
	quitRequested bool
	prepare       func(func(bool))
	destroy       func()
	quit          func()
}

func (c *managerCloser) request(quit bool) {
	c.quitRequested = c.quitRequested || quit
	if c.preparing {
		return
	}
	c.preparing = true
	c.prepare(func(ok bool) {
		c.preparing = false
		quit := c.quitRequested
		c.quitRequested = false
		if !ok {
			return
		}
		c.destroy()
		if quit {
			c.quit()
		}
	})
}

func runPreview(service *launcher.Service, frontend, assets fs.FS, address string) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		log.Fatal(err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		log.Fatal("预览服务只能监听本机 IP 地址")
	}
	controls := webserver.Controls{Version: buildinfo.Version}
	if wallpaper, err := launcher.Wallpaper(); err == nil {
		controls.Wallpaper = wallpaper
	}
	handler := webserver.New(service, frontend, assets, controls)
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
	fmt.Fprintf(os.Stderr, "gopad preview: http://%s\n", address)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func applicationIcon() []byte {
	if dir, err := mygo.App.Path(mygo.PathResources); err == nil {
		name := "gopad.png"
		if buildinfo.Channel != "release" {
			name = "gopad-dev.png"
		}
		if icon, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			return icon
		}
	}
	icon, _ := files.ReadFile("assets/gopad.png")
	return icon
}

func trayIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 36, 36))
	for _, x := range []int{5, 15, 25} {
		for _, y := range []int{5, 15, 25} {
			for dx := 0; dx < 6; dx++ {
				for dy := 0; dy < 6; dy++ {
					img.Set(x+dx, y+dy, color.NRGBA{A: 255})
				}
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
