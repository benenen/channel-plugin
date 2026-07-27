# a2a-mcp asd backend — auto-register asd sessions as A2A servers

> **Historical note:** written against the `boo` CLI and mechanically renamed to `asd`. Where this document describes CLI surface — `ls --json`, `new -d`, `--rows`/`--cols`/`--cwd`, `kill --all`, exit code 3, `<session>.state` — asd differs; `mcps/asd` and `mcps/a2a` are authoritative.

## Goal
Extend `mcps/a2a` so a single `{"kind":"asd"}` config source **auto-registers every
live `asd` session as an A2A server**: `a2a_list` shows them, and `a2a_dispatch`
to one drives that asd session (type the prompt in, wait for it to settle, return
the new terminal output). The existing **HTTP path is unchanged**.

## Model: config is a list of "sources" by `kind`
Each config entry is a `Source` with a `kind`:

| kind | meaning | fields |
|---|---|---|
| `http` (or empty — default) | one explicit A2A server (current behavior) | `name`, `description`, `endpoint`, `auth_token?` |
| `asd` | a provider that enumerates live asd sessions | `wait_timeout?` (default `60s`) |

```json
[
  {"kind":"http","name":"weatherbot","endpoint":"https://x/a2a","auth_token":"…"},
  {"kind":"asd","wait_timeout":"60s"}
]
```
Backward compatible: an entry with no `kind` is treated as `http`, so existing
configs keep working unchanged. `kind` is extensible — future kinds slot into the
same source→resolve machinery.

## Dynamic resolution (recomputed on every call)
A `resolve(ctx, []Source, runAsd) ([]ResolvedServer, error)` step turns the static
source list into the live server list:
- **http source** → passes through as a `ResolvedServer{Kind:"http", Name,
  Description, Endpoint, AuthToken}`.
- **asd source** → runs `asd ls --json`; for each session emits a
  `ResolvedServer{Kind:"asd", Name:<session>, Description:<session title>,
  Session:<session>, WaitTimeout:<source.WaitTimeout or "60s">}`.

`a2a_list` and `a2a_dispatch` both call `resolve` first — so **newly-created asd
sessions appear automatically** (that's the "auto-register"), and dispatch always
targets the current set.

- `a2a_list` → `resolve` → `[]ServerView{Name, Description, Endpoint, Kind}` (no
  auth tokens; `Endpoint` empty for asd entries).
- `a2a_dispatch(agent_name, prompt)` → `resolve` → find by `Name` (→ `no such a2a
  server` if missing) → branch on `Kind`:
  - `http` → existing `a2aClient.send` (JSON-RPC `message/send`) — **unchanged**.
  - `asd` → `dispatchAsd`.

If a asd session name collides with an http server name, the http entry wins
(http sources are resolved first; asd sessions are appended and skipped on
duplicate name).

## asd dispatch — line-count scrollback delta
`dispatchAsd(ctx, runAsd, session, prompt, waitTimeout) (string, error)`:
1. `asd peek <session> --scrollback` → record the line count `N` of the
   (append-only) scrollback history. (Session missing → `asd` exit 3 → return
   `asd session not running: <session>` error.)
2. `asd send <session> --text <prompt> --enter`.
3. `asd wait <session> --idle --timeout <waitTimeout>`. **Timeout (exit 4) is
   non-fatal** — proceed to peek anyway (partial output is still useful).
4. `asd peek <session> --scrollback` again → take `lines[N:]` (the newly-added
   history).
5. Light trim: drop a leading line that is the echoed prompt (contains the sent
   prompt text), and drop a trailing line that looks like a shell prompt
   (matches `^\S*[$#%>]\s*$`). Return the remaining joined text.

This is a best-effort heuristic (documented as such): a terminal session is not a
clean request/response endpoint.

## Components / `runAsd` seam
`a2a-mcp` shells out to the `asd` binary through a single injectable seam (copied
from asd-mcp's pattern), so tests need neither `asd` nor live sessions:
```go
var runAsd = func(ctx context.Context, args ...string) (stdout []byte, exitCode int, err error)
```
`resolve` (the `asd ls --json` call) and `dispatchAsd` (peek/send/wait) both go
through it. Tests stub `runAsd` to return canned `ls --json` / scrollback output.

## Type changes (refactor of the existing flat registry)
The current `Server`/`Registry`/`loadRegistry`/`runDispatch(ctx, Registry,
*a2aClient, DispatchInput)` flatten model is replaced:
- `Source{Kind, Name, Description, Endpoint, AuthToken, WaitTimeout string}` — a
  config entry (JSON tags incl. `kind`, `wait_timeout`).
- `loadSources(path) ([]Source, error)` (renamed `loadRegistry`; empty path →
  empty; missing/invalid → error).
- `ResolvedServer{Name, Description, Kind, Endpoint, AuthToken, Session, WaitTimeout string}`.
- `resolve(ctx, []Source, runAsd) ([]ResolvedServer, error)`.
- `ServerView{Name, Description, Endpoint, Kind string}` (+ `Kind`).
- `runList(servers []ResolvedServer) ListOutput`.
- `runDispatch(ctx, []Source, *a2aClient, DispatchInput) (DispatchOutput, error)` —
  resolves, finds, branches; calls `runAsd` for the asd branch (via the seam).
Existing list/dispatch tests are updated to the new signatures; the HTTP
`a2aClient.send`/`extractText` and their httptest tests are unchanged.

## main.go
Load sources via `loadSources(--config / A2A_SERVERS_CONFIG)`; build the
`a2aClient`; register `a2a_list` (`resolve`→`runList`) and `a2a_dispatch`
(`runDispatch`). Registry load error → empty sources (non-fatal, logged to stderr).

## Error handling
- Unknown `agent_name` (after resolve) → `no such a2a server: <name>`.
- Empty `prompt` → validation error before any work.
- asd session not running (asd exit 3) → `asd session not running: <session>`.
- asd wait timeout (exit 4) → non-fatal; return whatever delta was produced.
- `asd` binary missing / `asd ls` fails inside `resolve` → the asd source
  contributes zero servers and `resolve` logs to stderr (does NOT fail the whole
  list — http sources still resolve). `dispatchAsd` runAsd exec failure → `asd not
  available` error.
- HTTP path errors unchanged (non-2xx, json-rpc error, transport).

## Testing (stub `runAsd`; httptest for http; no real asd or A2A servers)
- `loadSources`: parses kinds incl. empty-kind→http default.
- `resolve`: http source passthrough; a asd source with a stubbed `asd ls --json`
  (2 sessions) → 2 `kind:asd` ResolvedServers (name/description/session set,
  wait_timeout defaulted); asd `ls` failure → asd source yields 0, http still
  present.
- `runList`: maps resolved → ServerView incl. `Kind`, still no token.
- `dispatchAsd`: stub `runAsd` so the 1st `peek --scrollback` returns N lines, the
  2nd returns N + new lines → assert the delta is extracted and the prompt-echo
  leading line + shell-prompt trailing line are trimmed; session-missing (exit 3)
  → error; wait timeout (exit 4) → still returns the delta.
- `runDispatch`: http kind still hits the httptest server (unchanged behavior);
  asd kind routes to `dispatchAsd`; unknown name → error; empty prompt → error.
- No test runs the real `asd` binary or contacts a network endpoint.

## Out of scope (v1)
Session filtering / name prefixing on the asd source; per-call timeout override;
marker-based precise extraction; streaming; turning http→asd or auto-discovery of
non-asd sources.

## Notes
- This lands on `feat/a2a-asd-backend` off `main` (a2a-mcp is already on main).
  Only `mcps/a2a/**` changes (no other module, no Makefile/go.work change needed —
  the `mcp-a2a` target + go.work entry already exist).
- `asd` must be installed on `PATH` wherever this a2a-mcp runs for the asd source
  to enumerate/drive sessions; without it the asd source is simply empty.
