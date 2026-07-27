# asd-mcp Design — an MCP server wrapping the `asd` terminal multiplexer

> **Historical note:** written against the `boo` CLI and mechanically renamed to `asd`. Where this document describes CLI surface — `ls --json`, `new -d`, `--rows`/`--cols`/`--cwd`, `kill --all`, exit code 3, `<session>.state` — asd differs; `mcps/asd` and `mcps/a2a` are authoritative.

## Goal
A new standalone MCP server project `mcps/asd` that exposes the **automation
surface** of the `asd` terminal multiplexer (screen/tmux-like, on libghostty) as
typed MCP tools, so an agent can create headless sessions, drive them, read their
screens, and tear them down. It mirrors the existing `mcps/echo` and `mcps/ping`
project structure.

## Scope

In scope — six tools (the script-driving / "asd help automation" subset):

| Tool | `asd` invocation | Notes |
|---|---|---|
| `asd_ls` | `asd ls --json` | list sessions |
| `asd_new` | `asd new [name] -d [--rows N] [--cols N] [--cwd DIR] -- cmd…` | **always `-d`** (no TTY); prints the session name |
| `asd_send` | `asd send <name> [--text … [--enter]] \| [--key …]` | text XOR keys |
| `asd_peek` | `asd peek <name> --json [--scrollback]` | **always `--json`** (structured screen) |
| `asd_wait` | `asd wait <name> (--text … \| --idle) [--timeout …]` | timeout → not an error |
| `asd_kill` | `asd kill <name> \| --all` | one or all |

Out of scope (v1): interactive `attach`/`ui` (require a real TTY); `rename`,
`restore`, `version` (low value for the automation subset).

## Background: the `asd` CLI
- Binary on `PATH` (here `/usr/local/bin/asd`). Everything except `attach` works
  without a terminal.
- **Exit codes:** `0` success · `1` error · `2` usage error · `3` no such session
  · `4` wait timed out.
- **Machine-readable output:**
  - `asd ls --json` → `[{"name","attached","idle_ms","title"}]`
  - `asd peek --json` → `{"session","title","rows","cols","cursor":{"row","col"},"screen"}`
- `asd new -d` prints the resulting session name on stdout.
- `asd send` is literal: `--text` (no implicit newline), `--enter` appends Enter,
  `--key <comma,list>` for named keys (`Enter`, `C-c`, `Up`, …); `--text` and
  `--key` are mutually exclusive.
- `asd wait` blocks until `--text <substr>` appears or `--idle` (2s quiet);
  `--timeout <dur>` (default 30s), durations like `500ms`, `2s`, `1m`.

## Architecture

Standalone Go module `mcps/asd` (mirrors `mcps/echo`):
- `go.mod` — `module github.com/benenen/myclaw/mcps/asd`, `go 1.23.6`,
  `require github.com/modelcontextprotocol/go-sdk v0.8.0`.
- `main.go` — `mcp.NewServer(&mcp.Implementation{Name:"asd", Version:"0.1.0"}, nil)`,
  registers the six tools via `mcp.AddTool`, runs `server.Run(ctx, &mcp.StdioTransport{})`.
  Diagnostics to stderr; stdout is the JSON-RPC stream.
- `asd.go` — the tool logic.
- `asd_test.go` — tests.
- The module is added to the repo `go.work` (`use ./mcps/asd`) alongside echo/ping,
  and the `Makefile` mcps targets if they enumerate modules.

### The injectable runner (testability)
All tools shell out to the `asd` binary through a single package-level seam so
tests need neither the binary nor live sessions:
```go
// runAsd executes `asd <args>` and returns stdout, the process exit code, and a
// non-nil err only for failures that are NOT a normal asd non-zero exit
// (e.g. binary not found). Overridable in tests.
var runAsd = func(ctx context.Context, args ...string) (stdout []byte, exitCode int, err error) {
    cmd := exec.CommandContext(ctx, "asd", args...)
    var out, errb bytes.Buffer
    cmd.Stdout, cmd.Stderr = &out, &errb
    err = cmd.Run()
    if exitErr, ok := err.(*exec.ExitError); ok {
        return out.Bytes(), exitErr.ExitCode(), nil // asd ran; non-zero is data, with stderr in errb
    }
    // also capture stderr into the returned error for non-ExitError failures
    ...
    return out.Bytes(), 0, err
}
```
(The exact stderr-plumbing is an implementation detail; the contract is: `err` is
reserved for "couldn't run asd", and a non-zero `exitCode` with stdout/stderr is
returned for asd's own exits so handlers can map exit 3/4.)

### Per-tool shape
Each tool is split into a **pure `argsFor<Tool>(in)`** that builds the `asd` argv
(fully unit-testable, no I/O) and a handler that calls `runAsd`, maps the exit
code, and parses JSON where applicable.

Input/Output structs (jsonschema-tagged, matching echo/ping style). Highlights:
- `asd_ls`: input `struct{}`; output `{ sessions []Session }` where
  `Session{Name string; Attached bool; IdleMs int64; Title string}` parsed from
  `ls --json`.
- `asd_new`: input `{ Name string `json:",omitempty"`; Command []string; Rows,Cols int `omitempty`; Cwd string `omitempty` }`;
  argv always includes `-d`; `Command` (if set) appended after `--`; output `{ Name string }`
  (trimmed stdout — the printed session name).
- `asd_send`: input `{ Name string; Text string `omitempty`; Enter bool `omitempty`; Keys string `omitempty` }`;
  validate exactly one of Text / Keys is set (`text XOR keys`); output `{ Ok bool }`.
- `asd_peek`: input `{ Name string; Scrollback bool `omitempty` }`; argv always
  `--json`; output the parsed peek object `{ Session, Title string; Rows, Cols int; Cursor{Row,Col int}; Screen string }`.
- `asd_wait`: input `{ Name string; Mode string (text|idle); Text string `omitempty`; Timeout string `omitempty` }`;
  validate Mode ∈ {text, idle} and Text present when Mode=text; output `{ Matched bool }`
  (exit 4 → `Matched:false`, not an error).
- `asd_kill`: input `{ Name string `omitempty`; All bool `omitempty` }`; validate
  exactly one of Name / All; output `{ Ok bool }`.

### Error / exit-code mapping (in handlers)
- `0` → success (parse output as above).
- `3` (no such session) → tool error `no such session: <name>`.
- `4` (wait timed out) → `asd_wait` returns `{Matched:false}` (success result); not
  applicable to other tools.
- `1`/`2`/other → tool error carrying asd's stderr (trimmed).
- `runAsd` `err != nil` (couldn't execute `asd`) → tool error
  `asd not available: <err>`.

Input-validation failures (e.g. both Text and Keys on `send`) return an error
**before** calling `runAsd`.

## Testing
Stub `runAsd` in tests; no real asd binary or sessions required.
- **argv builders (pure):** `argsForLs/New/Send/Peek/Wait/Kill` produce the exact
  expected `[]string` for representative inputs (e.g. `asd_new{Name:"build",
  Command:["bash"]}` → `["new","build","-d","--","bash"]`; `asd_send` with `Text`
  + `Enter` → `["send","build","--text","make","--enter"]`; `asd_peek{Scrollback}`
  → `["peek","build","--json","--scrollback"]`).
- **output parsing:** `ls --json` and `peek --json` fixtures parse into the structs.
- **exit-code mapping:** stub returns exitCode 3 → no-such-session error; 4 on a
  `asd_wait` → `{Matched:false}`; 1 with stderr → error carrying stderr.
- **validation:** `send` with both/neither Text & Keys → error; `wait` Mode=text
  with empty Text → error; `kill` with neither/both Name & All → error.
- No test invokes the real `asd` binary (keeps the suite hermetic; a manual smoke
  against real asd is a separate verification step).

## Assumptions / notes
- `asd` must be installed on `PATH` wherever this stdio server runs.
- Lives under the existing parallel `mcps/` effort on branch
  `feat/per-bot-mcp-servers`; only new `mcps/asd/**` files are added (plus a
  one-line `go.work` `use` entry), leaving `mcps/echo` and `mcps/ping` untouched.
- Once built, an operator wires it to a bot via the per-bot MCP feature, e.g.
  `myclaw mcp add --name asd --type stdio --command <path-to-asd-mcp-binary>` then
  `myclaw mcp attach --bot <id> --server asd`.
