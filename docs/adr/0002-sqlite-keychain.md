# ADR 0002 — SQLite (pure Go) + OS keychain split

Date: 2026-10-16 · Status: accepted

## Context
Need offline cache + instant search; must store account secrets safely.

## Decision
- `modernc.org/sqlite` (pure Go, FTS5) for accounts/endpoints + message cache.
- Passwords and OAuth tokens live ONLY in the OS keychain (`go-keyring`).

## Rationale
- No CGO → `GOOS=windows go build` works from macOS.
- FTS5 gives inbox search without a server round-trip; LIKE fallback covers
  FTS-hostile input.
- Keychain split means a stolen DB file leaks mail, never credentials.

## Consequences
- Larger binary than mattn driver (~+7MB); acceptable.
- Keychain must exist or AddAccount reports it (login still verifies first).
