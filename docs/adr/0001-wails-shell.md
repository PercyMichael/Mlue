# ADR 0001 — Wails (Go) for the desktop shell

Date: 2026-10-16 · Status: accepted

## Context
Need one app for macOS, Windows, Linux with a Spacemail-grade UI (HTML mail
rendering) and a strong mail-sync engine. Candidates: Electron, Tauri (Rust),
Wails (Go), pure native (Fyne / iced).

## Decision
Wails v2: Go backend + system-webview UI in React/Tailwind.

## Rationale
- Go was already installed; goroutines fit IMAP IDLE + multi-folder sync.
- HTML email needs a browser engine — pure-native toolkits would force a
  second renderer or degraded plain-text mail.
- Binaries ~12MB vs ~150MB Electron; no Rust toolchain needed (Tauri).
- `modernc.org/sqlite` keeps builds CGO-free for easy cross-compilation.

## Consequences
- Tied to system WebKit/WebView2 age (see ADR 0003).
- No tray API in Wails v2.16 — deferred to v3.
