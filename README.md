# Spacemail-style Desktop Email Client

Clean, minimalistic desktop email client for **macOS, Windows, Linux** — looks like the Spacemail screenshot in `docs/` (3-pane: sidebar / list / reader).

## Requirements

1. **Desktop, Go-powered**: Wails v2 (Go backend + webview UI). One binary per OS. No Rust needed.
2. **Spacemail look**: top search bar, `Compose` pill, folders (Inbox/Drafts/Sent/Archive/Spam/Trash), storage meter, message list with unread dots + avatars, reader with action toolbar + HTML welcome mail.
3. **Real mail, all IMAP/SMTP**: Zoho, cPanel hosting, Roundcube-backed mailboxes, Gmail, Outlook, Yahoo, iCloud, GMX, Yandex, Fastmail, generic IMAP.
4. **No SMTP/port prompts**: user enters only Name + Email + Password. App auto-detects hosts/ports/security — provider presets, MX sniffing (custom domains on Zoho/Google/M365), then the domain's own published settings (cPanel autodiscover / ISPDB / SRV), then a `mail.domain` guess.
5. **Offline-first**: SQLite cache + full-text search (FTS5). Works without network after first sync.
6. **Secure secrets**: passwords in OS keychain (macOS Keychain / Win Credential Manager / Linux Secret Service) via `go-keyring`. SQLite stores endpoints + mail only.
7. **Full v1 flows**: add account, list/read, search, compose/send, folder switch.
8. **Icons**: `lucide-react` (MIT) throughout — Mail, Inbox, Send, Archive, Trash2, Star, Reply/Forward, Search, Settings, etc.

## Features (plain words)

What you can do today, and what's coming. Rule: this list is updated after every feature lands.

**Works now ✓**
- **Premium first run** — fresh installs open a branded sign-in screen (never an empty inbox): your details or one-click Google/Microsoft, then straight into mail.
- **Add any email account easily** — type your name, email, and password. No server names, no port numbers. Zoho, Gmail, Outlook, and your own domain/cPanel mail all work.
- **Or sign in with Google/Microsoft** — one click, approve in your browser, no app passwords, nothing to paste. (Needs a build configured with the provider's OAuth client ID — see setup below; otherwise use password/app-password sign-in.)
- **See your mailboxes** — the six standard folders (Inbox, Drafts, Sent, Archive, Spam, Trash) in a quiet sidebar, like Spacemail. Real folders are discovered on the server (Gmail's names included) with true unread counts. Provider extras and subfolders are hidden for now.
- Known gap: provider-made extras (Zoho's Templates/Snoozed/Notification/Newsletter, nested folders like Trash/Later) don't have a sidebar home yet — tracked in ROADMAP.
- **Refresh when you want** — the top-bar refresh button re-loads the current folder and updates all counts.
- **New mail arrives by itself** — the app holds a quiet background connection (IMAP IDLE) and lights up the list when mail lands, with a safety re-check behind it.
- **Told, not spammed** — a desktop notification pops when genuinely new mail arrives (never for old mail on first sync).
- **Honest offline** — lose the network and a banner says you're reading saved mail until you reconnect.
- **Settings that make sense** — gear icon shows your accounts (remove any), image-loading choice, and how much mail is stored.
- **Read comfortably** — click a message, read the full letter with pictures and formatting, safely cleaned. Remote images stay blocked until you allow them; links open in your browser, never inside the mail.
- **Find fast** — search box filters by sender, subject, and text; recent mail stays on your computer so it works offline. Power users can type `from:boss`, `subject:invoice`, `to:me`.
- **The whole mailbox, eventually** — newest mail first, older pages load as you scroll, background backfill fills the rest, and a UID-diff repair pass closes any gaps. (One account showed 337/350 — fix implemented, live confirmation pending.)
- **Write and send** — Compose button, To/Subject/message, attach photos and documents (20MB total cap), sends through your own provider.
- **Reply and forward properly** — quotes the original with subject carried over (`Re:`/`Fwd:`); half-written mail is auto-saved and waiting when you come back. (Reply All currently behaves like Reply.)
- **Private by design** — your password lives in your computer's keychain, never in a file we could leak.
- **Open attachments** — files sent to you show under the message with size; photos preview inline, everything downloads with one click (up to 12MB per file shown in-app).

**Coming next ○**
- Installers for Windows/Linux, richer editor, calendar.

Full build order lives in `ROADMAP.md` (done items ticked `[x]`, rest `[ ]`).

## How the system works

```
┌─ frontend (React+TS+Vite+Tailwind, lucide-react) ─┐
│ TopBar search │ Sidebar │ List │ Reader │ Modals  │
└────────────── bindings (wailsjs/go/main/App) ────┘
┌─ Go backend ──────────────────────────────────────┐
│ autoconfig → mail (IMAP/SMTP) → store (SQLite)    │
│ secrets → OS keychain (go-keyring)                │
└───────────────────────────────────────────────────┘
```

### Add account (no ports asked)

1. UI collects `name, email, password` only.
2. `DetectSettings(email)`:
   - known domain → preset (e.g. `zoho.com → imap.zoho.com:993 SSL / smtp.zoho.com:465 SSL`)
   - else → `mail.<domain>:993 / :465` guess (covers 95% of cPanel/Roundcube hosts)
   - source tagged `preset | mx | autoconfig | srv | mx-guess | oauth | manual`
3. `AddAccount` → `mail.TestLogin` (IMAP LOGIN over TLS) → on success:
   - `keyring.Set("emailclient", email, password)`
   - `store.SaveAccount` (endpoints only)
4. `FetchMail` pulls newest ~30 envelopes, caches, returns cache.

Probe helper `ProbeHost(host,port,ssl)` does TCP+TLS dial (6s timeout) for the "Finding settings…" spinner and tries `mail./imap.` candidates concurrently from the UI if needed.

### Read / search

- `FetchMail(email,folder)` → IMAP SELECT + FETCH (UID, ENVELOPE, FLAGS) → map to `{uid,from,subject,date,unread}` → `SaveMessages` → `ListMessages`.
- `ListCached(email,folder,query)` → SQLite; empty query = `ORDER BY uid DESC LIMIT 500`, else FTS5 `MATCH` (LIKE fallback).
- Offline or login fail → returns last cache silently.

### Send

- `SendMail(from,to,subject,htmlBody)` → password from keychain → `go-mail` client (SSL on 465, else STARTTLS) → `DialAndSend`.

## Stack

- Shell: Wails v2.16, Go 1.26
- UI: React 19, Tailwind v3 (v3, not v4 — the system webview on older macOS can't do oklch/cascade layers), `lucide-react`
- Mail: `emersion/go-imap/v2`, `go-sasl`, `go-message`, `wneessen/go-mail`
- Store: `modernc.org/sqlite` (pure Go, no CGO → easy Win builds), FTS5
- Secrets: `zalando/go-keyring`

## Layout

```
app.go                    Wails bindings
main.go                   embed frontend/dist
internal/autoconfig/      presets + Detect/Probe/CandidateHosts
internal/mail/            TestLogin/FetchRecent/Send
internal/store/           SQLite: accounts, messages, FTS
frontend/src/             App.tsx (3-pane), mock.ts, style.css
```

## Run

```bash
# UI preview only (mock data, no Go)
cd frontend && npm install && npm run dev

# Full desktop (live IMAP, keychain, SQLite)
~/go/bin/wails dev

# Prod binaries (mac dmg / win exe / linux)
~/go/bin/wails build
```

DB lives at `~/.emailclient/emailclient.db`.

### One-click sign-in setup (app owner, once)

End users just click “Sign in with Google/Microsoft” — no IDs, no pasting. To enable it,
register one OAuth app per vendor (redirect URI `http://127.0.0.1:<port>/callback` — the app picks a free loopback port at runtime; register the loopback pattern your provider accepts), then bake the
client IDs into the binary at build time:

```bash
~/go/bin/wails build -ldflags "-X emailclient/internal/oauth.defaultGoogleID=YOUR_GOOGLE_ID -X emailclient/internal/oauth.defaultMicrosoftID=YOUR_MS_ID"
```

For local dev, `EMAILCLIENT_GOOGLE_CLIENT_ID` / `EMAILCLIENT_MICROSOFT_CLIENT_ID` env vars work too.
Without baked-in IDs, password/app-password sign-in still works everywhere.

## Notes / limits (v1)

- OAuth works after the owner step above; until then Gmail/Outlook need app passwords.
- Providers often need IMAP switched on plus an app-specific password (Gmail, Zoho, Yandex with 2FA).
- Known open bug: one Zoho inbox shows 337/350 cached — UID-diff repair is implemented, live confirmation pending.
- Provider-made folders (Templates, Snoozed, Newsletter…) and subfolders are hidden for now.
- No tray icon (Wails v2.16 has no tray API — notifications + offline banner included).
- Keychain must be available or `AddAccount` reports it (login still verified first).
- Offline you get fetched headers plus bodies you already opened; the rest needs connection.

Full status: `ROADMAP.md` (done `[x]`, next `[ ]`).
# Mlue
