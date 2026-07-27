# asd session capability panel — asd.capabilities.json in a2a_list

> **Historical note:** written against the `boo` CLI and mechanically renamed to `asd`. Where this document describes CLI surface — `ls --json`, `new -d`, `--rows`/`--cols`/`--cwd`, `kill --all`, exit code 3, `<session>.state` — asd differs; `mcps/asd` and `mcps/a2a` are authoritative.

## Goal
When `mcps/a2a` resolves a live `asd` session into an A2A server, enrich its
**`description`** from a `asd.capabilities.json` file in that session's working
directory, so the dispatching agent can route by capability via `a2a_list`. If the
file is absent or unreadable, fall back to the current default (the session title).

## Background — recovering a session's folder
`asd ls --json` / `peek --json` do NOT expose a session's cwd. But the asd daemon
snapshots each session's cwd (for `asd restore`) at
`<asd-config-dir>/<session>.state` — a **single line of plain text = the cwd**
(e.g. `~/.config/asd/myclaw.state` contains `/root/workspace/master/myclaw\n`).
`<asd-config-dir>` resolves as: `dir($ASD_CONFIG)` if `ASD_CONFIG` is set, else
`$XDG_CONFIG_HOME/asd`, else `~/.config/asd`.

## The capabilities file
`<cwd>/asd.capabilities.json` (all fields optional):
```json
{ "description": "A coding agent: edits Go, runs tests", "skills": ["go", "testing"] }
```
The resulting A2A `description` = `description` text, with skills appended as
` [skills: go, testing]` when present. (Per the chosen design, capabilities fold
into the existing `description` string — NO new structured field on the output.)

## Components (added to `mcps/a2a/a2a.go`)
Three small, pure-ish helpers (read real files; tested via `t.Setenv` + temp dirs,
no FS stubbing):
- `asdConfigDir() string` — env-driven dir resolution above.
- `asdSessionCwd(session string) (string, bool)` — read
  `<asdConfigDir>/<session>.state`; trim; `("", false)` on any read error / empty.
- `asdCapabilitiesDescription(cwd string) (string, bool)` — read
  `<cwd>/asd.capabilities.json`; JSON-decode `{description, skills}`; build the
  description string (description + ` [skills: …]`); `("", false)` on missing file,
  parse error, or empty result.

## Wiring — `resolve()` asd expansion
For each session from `asd ls --json`, compute the description:
```go
desc := sess.Title
if cwd, ok := asdSessionCwd(sess.Name); ok {
    if cap, ok := asdCapabilitiesDescription(cwd); ok {
        desc = cap
    }
}
// ResolvedServer{..., Description: desc, ...}
```
Everything else in `resolve` / `dispatchAsd` / the HTTP path is unchanged. The
`a2a_list` ServerView already carries `Description`, so the capability text flows
to the agent with no schema change.

## Error handling
All best-effort: a missing/empty `.state`, a missing/invalid `asd.capabilities.json`,
or an empty description → silently fall back to the session title. Never fails
`a2a_list`. (No stderr spam for the common "no file" case; only log genuinely
unexpected errors if any.)

## Testing (real files via `t.Setenv` + `t.TempDir`; runAsd stubbed for `asd ls`)
- `asdSessionCwd`: write `<tmp>/asd/<session>.state` with a path, set
  `XDG_CONFIG_HOME=<tmp>` → returns the trimmed cwd; missing file → `false`.
- `asdConfigDir`: `ASD_CONFIG=/x/config.toml` → `/x`; `XDG_CONFIG_HOME=/y` → `/y/asd`;
  neither → `~/.config/asd`.
- `asdCapabilitiesDescription`: temp cwd with `asd.capabilities.json`
  {description+skills} → `"… [skills: a, b]"`; description only → plain; missing
  file → `false`; invalid JSON → `false`; empty object → `false`.
- `resolve` enrichment: stub `runAsd` (`asd ls` → one session "build"); set
  `XDG_CONFIG_HOME` to a temp dir with `asd/build.state` → a temp cwd containing
  `asd.capabilities.json` → the resolved asd server's `Description` is the
  capability text. With NO `.state` (or no capabilities file) → `Description` ==
  the session title (fallback).

## Out of scope (v1)
File-change watching (resolve re-reads live on each call anyway); a full A2A
AgentCard; a structured `skills` field on the output; using capabilities to
auto-rank/route inside a2a-mcp (routing stays the dispatching agent's job, informed
by the description).

## Notes
- Branch `feat/a2a-asd-capabilities` off `main`; only `mcps/a2a/{a2a.go,a2a_test.go}`
  change. No new deps (uses `os`, `path/filepath`, `encoding/json`, `strings`).
