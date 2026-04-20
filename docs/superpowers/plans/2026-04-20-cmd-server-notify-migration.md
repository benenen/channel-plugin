# Cmd Server Notify Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the root cobra command and the `server` subcommand into `cmd/server.go`, keep `main.go` as a thin entrypoint, and add a placeholder `notify` subcommand that prints its message.

**Architecture:** Keep a single command package entry surface: `cmd.Execute(args, stdout, stderr) int`. The command package owns root command construction, server startup wiring, usage handling, and the new `notify` subcommand. The repository root only forwards process args and streams into the command package.

**Tech Stack:** Go 1.23, cobra, standard library `io`/`fmt`/`log`/`net/http`, existing bootstrap/config packages, `go test`

---

## File Map

| File | Responsibility |
|---|---|
| `main.go` | Thin process entrypoint that delegates to `cmd.Execute` |
| `cmd/server.go` | Root command, `server`, `notify`, server startup helpers, usage output |
| `cmd/server_test.go` | Command behavior tests and server helper tests |
| `go.mod` / `go.sum` | Cobra dependency metadata |

## Task 1: Move command behavior into `cmd/server.go`

**Files:**
- Modify: `main.go`
- Create: `cmd/server.go`
- Create: `cmd/server_test.go`
- Delete: `main_test.go`

- [ ] **Step 1: Write the failing command package tests**

Add tests in `cmd/server_test.go` for:
- default args run the injected server function once
- explicit `server` runs the injected server function once
- help aliases print usage
- unknown command returns exit code `1` and prints usage
- `runServer` returns `1` when config load fails
- `serviceURL` preserves current behavior

- [ ] **Step 2: Run focused tests to verify they fail**

Run: `go test ./cmd -run 'TestExecute|TestRunServer|TestServiceURL' -v`
Expected: FAIL because the `cmd` package does not exist yet.

- [ ] **Step 3: Implement the command package**

Create `cmd/server.go` with:
- `package cmd`
- `Execute(args []string, stdout, stderr io.Writer) int`
- root command builder
- `server` subcommand
- `runServer(stderr io.Writer) int`
- `serviceURL(addr string) string`
- injected vars for `loadConfig`, `newApp`, `listenAndServe`

Update `main.go` to:

```go
package main

import (
	"os"

	"github.com/benenen/myclaw/cmd"
)

func main() {
	os.Exit(cmd.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
```

- [ ] **Step 4: Run focused tests to verify they pass**

Run: `go test ./cmd -run 'TestExecute|TestRunServer|TestServiceURL' -v`
Expected: PASS

## Task 2: Add `notify` placeholder command

**Files:**
- Modify: `cmd/server.go`
- Modify: `cmd/server_test.go`

- [ ] **Step 1: Write the failing `notify` tests**

Add tests for:
- `notify hello` prints `hello` to stdout and exits `0`
- `notify` with no positional message exits `1`

- [ ] **Step 2: Run focused `notify` tests to verify they fail**

Run: `go test ./cmd -run 'TestExecuteNotify' -v`
Expected: FAIL because `notify` is not registered yet.

- [ ] **Step 3: Implement the minimal `notify` command**

Register a `notify` subcommand in `cmd/server.go`:
- `Use: "notify <message>"`
- `Args: cobra.ExactArgs(1)`
- print the argument to stdout
- return non-zero on missing arg via existing execute path

- [ ] **Step 4: Run focused `notify` tests**

Run: `go test ./cmd -run 'TestExecuteNotify' -v`
Expected: PASS

## Task 3: Final verification

**Files:**
- Modify: none

- [ ] **Step 1: Run package-level verification**

Run: `go test . ./cmd`
Expected: PASS

- [ ] **Step 2: Run build verification**

Run:

```bash
go build .
go build ./...
```

Expected: PASS

- [ ] **Step 3: Run repository-wide verification**

Run: `go test ./...`
Expected: either PASS, or if pre-existing unrelated failures remain, report them precisely with evidence.
