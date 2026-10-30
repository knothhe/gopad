# gopad

English | [简体中文](README.zh-CN.md)

<img src="assets/gopad.svg" alt="gopad icon" width="96" />

A macOS app launcher with a native glass interface, powered by Go and [MyGo](https://github.com/egoist/mygo). No Node.js runtime or frontend server required.

## Features

- Search and launch apps from an icon grid on your current desktop, with keyboard navigation and pagination.
- Organize apps with custom categories, favorites, drag-and-drop ordering, and hidden apps.
- Manage apps and preferences in a separate, resizable settings window.
- Adjust icon size and restore your last category when reopening the launcher.
- Choose English, Simplified Chinese, or your system language; animations respect Reduce Motion.

## Quick start

Current target: **macOS 12+ on Apple Silicon**. Running from source requires Xcode Command Line Tools and the Go version specified in [go.mod](go.mod).

```sh
make run
```

Open the launcher from the Dock, menu bar, or global shortcut. Use the gear button or `⌘,` to open settings.

For app bundles and installation, `make build` / `make install` also require Python 3.9+, Pillow, and a configured signing identity. Follow the [build, signing, and release guide (中文)](docs/build-release.md) for setup.

For GitHub CI signing, run `make signing-upload` to choose an existing signing identity or P12 and upload the required secrets interactively.

## Shortcuts

| Shortcut | Action |
| --- | --- |
| `⌃⌥Space` | Toggle launcher (release) |
| `⌃⌥⇧Space` | Toggle launcher (Dev / preview) |
| `⌘F` | Search apps |
| `⌘,` | Open settings |
| `Tab` / `⇧Tab` | Next / previous visible category in the app grid |
| Arrow keys / `Return` | Select / launch an app |
| `Page Up` / `Page Down` | Previous / next launcher page |
| `Esc` | Hide launcher; in settings, close the current menu, dialog, or window |

## Development

Checks also require Python 3.9+ and Node.js 22 (for frontend tests):

```sh
npm ci --prefix tests/frontend
make check
```

UI translations live in [`internal/i18n/locales`](internal/i18n/locales).
