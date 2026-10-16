# Roadmap — done vs next

Status: v0.3 works (landing sign-in, auto-detect accounts incl. OAuth, all-folder sync,
IDLE push + notifications, bodies + attachments both ways, offline cache + FTS search).

> Process rule: after every feature, tick it `[x]` here and update the Features section in `README.md`.

## Done ✓

- [x] **Scaffold** — Wails v2 + Go 1.25 + React 19 + Tailwind (v3, see ADR 0003), verified `wails doctor`
- [x] **Spacemail 3-pane UI** — sidebar/list/reader, search bar, compose + add-account + settings modals (`lucide-react` icons)
- [x] **Mock data mode** — works in browser preview without backend
- [x] **Autoconfig engine** — Zoho/Gmail/Outlook presets + ISPDB/SRV discovery + `mail.<domain>` guess for cPanel/Roundcube; UI asks Name/Email/Password only
- [x] **SQLite cache + FTS5** — accounts, messages, offline search (`modernc.org/sqlite`, pure Go)
- [x] **Real IMAP** — login test + last-30 envelope fetch (`go-imap/v2`); keychain via `go-keyring`
- [x] **Real SMTP send** — auto SSL/STARTTLS (`go-mail`)
- [x] **Frontend↔Go wiring** — bindings with mock fallback outside Wails
- [x] **Full bodies** — MIME parse (HTML preferred), DOMPurify-sanitized render in reader
- [x] **README** — requirements + plain-words Features + run guide (updated per feature)
- [x] **This roadmap** — done/next tracking file
- [x] **Prod-binary UI fix** — Tailwind v4→v3 (old system WebKit on macOS 12 choked on oklch/cascade layers)
- [x] **Premium landing / first-run login**
  - Zero-account desktop opens a branded split-screen sign-in (form + OAuth), never mock data; reader shows real sender header per message + empty-mailbox placeholder.

## Next — P0 — make it a real daily driver

- [x] **Attachments — receive**
  - MIME parts parsed in `mail.GetFull` (filename, mime, size, base64 ≤12MB); reader row with download + image thumbnails.
- [x] **Attachments — send**
  - Compose paperclip picker (20MB total cap), base64 to Go, `go-mail AttachReader` with content types.
- [x] **Standard-folder discovery and sync** (custom folders still open, see below)
  - `LIST *` discovery + `STATUS` unseen counts, RFC attrs + name mapping (Gmail `[Gmail]/…`, Junk, localized), per-folder fetch+cache, live sidebar counts, top-bar refresh button wired.
- [x] **Refresh + IDLE push**
- [x] **Full-folder paging + backfill**
  - `FetchBefore`/UID pages (`FetchOlder` binding), infinite scroll-back with “loading older” + “all caught up” stop state, `StartBackfill` background fill to completion (capped, progress via `mail:inbox` events).
  - `mail.IdleWait` (EXISTS-driven, 5-min cap, reconnect backoff), per-account watcher goroutine, `mail:inbox` runtime events → UI auto-refresh, safety re-fetch every ~3rd cycle, shutdown cleanup, watch logging.
  - Chunked FETCH (10s, keeps partial on error), UID-based paging with proper ranges that walks down past deleted-mail gaps until the page is full, fetch logging, list header shows shown/total.

## P1 — account types

- [x] **OAuth2 (Gmail / Outlook)** — only in builds configured with owner-registered client IDs (live proof still needs that build); otherwise password/app-password flow.
  - PKCE loopback flow (no secret, no user pasting), tokens in keychain with silent refresh, XOAUTH2 IMAP/SMTP; password flow kept for Zoho/cPanel.
- [x] **cPanel autodiscover**
  - MX sniffing (custom domains hosted on Zoho/Google/M365 → vendor preset), ISPDB `autoconfig.<domain>/mail/config-v1.1.xml` parse + `_imaps`/`_submission` SRV, bounded timeouts, before `mail.` guess.

## P1 — reading / writing quality

- [x] **Compose upgrades**
  - Reply/ReplyAll/Forward with quoting + subject carry-over, draft autosave (localStorage), plain-text→HTML line breaks on send.
- [x] **Reader hardening**
  - Remote-image block + per-mail "Load images" (persisted), link intercept to system browser, print button.
- [x] **Search polish**
  - 300ms debounce, whole-cache FTS via `ListCached` (LIKE fallback for tricky input), `from:`/`subject:`/`to:` operators, match highlight.

## P2 — desktop integration

- [x] **Notifications + offline banner** (tray deferred)
  - Desktop notify on genuinely-new mail (`beeep`, baselined so first sync stays quiet), offline banner via online/offline events.
  - Tray icon skipped: Wails v2.16 has no tray API and external trays need cgo + fight Wails' main thread. Revisit on Wails v3.
- [x] **Settings screen**
  - Gear opens accounts list + remove (wipes cache + keychain + tokens + stops sync), remote-image default toggle, storage usage.
- [ ] **Auto-update + signed builds** (needs Apple/Windows certs + update endpoint)

## Next — visible gaps (proposed order)

- [ ] **Close the 337/350 gap (fix implemented, needs your live confirm)**
  - Zoho INBOX: STATUS total=350 but cache stalled at 337. Landed: no truncation at page limits, continue-past-failed-chunks, skip logging with UIDs, save-error logging, UIDVALIDITY tracking with reset-on-change, UID SEARCH ALL diff (`Reconcile`, runs after backfill + on manual refresh). Restart dev, refresh, expect `recovered N of M` + 350 shown.
- [ ] **Custom/provider folders** — Templates, Snoozed, Notification, Newsletter skipped as unmapped; Trash/Later, Trash/Blocked dropped as duplicate keys. Needs: per-mailbox keys + dynamic sidebar entries.
- [ ] **Reader toolbar actions** — Archive, Star, Delete, Spam buttons are still dead; needs server-side move/flag + cache update
- [ ] **Read-state sync** — reading marks local cache only; push `\Seen` back to the server
- [ ] **Unified inbox + account switcher** — one combined list across accounts instead of one-at-a-time
- [ ] **Compose extras** — rich text (bold/lists/links), send retry queue, undo-send window
- [ ] **Native save dialog** — downloads currently use data-URLs; use Wails save dialog for large files
- [ ] **List virtualization** — long folders now render hundreds of rows; render only visible rows
- [ ] **Playwright E2E** — stubbed-backend run: add account → read → send

## P2 — ship (blocked on user test pass in `wails dev`)

- [ ] **Prod packages**: `wails build` for mac (dmg, arm64+amd64), Windows (nsis), Linux (AppImage/deb)
- [x] **Tests**: Go unit across all 4 packages (`autoconfig`, `mail`, `oauth`, `store`) green; `tsc` + `vite build` + `go vet` clean
- [x] **Docs**: `docs/adr/` (Wails shell, SQLite+keychain, Tailwind v3); screenshot in README still open

## Non-goals (v1)

Calendar, contacts sync, PGP, team collaboration, mobile apps.
