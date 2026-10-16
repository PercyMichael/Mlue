# ADR 0003 — Tailwind v3, not v4 (old WebKit)

Date: 2026-10-16 · Status: accepted

## Context
Prod binary on macOS 12 rendered black borders/boxy UI while Chrome was
perfect.

## Decision
Downgrade Tailwind v4 → v3.4 + PostCSS/autoprefixer, explicit plugin import.

## Rationale
- v4 emits `oklch()` colors and cascade `@layer`s; the system WebKit on
  older macOS drops them silently (borders fall back to black).
- v3 emits hex/rgb + prefixed CSS the old engine understands.
- Verified: zero `oklch`/`@layer` in built CSS.

## Consequences
- Miss v4 features (not needed for this UI).
- Revisit when minimum macOS moves past the old WebKit.
