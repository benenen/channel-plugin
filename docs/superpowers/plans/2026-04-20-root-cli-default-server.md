# Root CLI Default Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the old `cmd/server` startup path with a root-level CLI where `go run .` defaults to the `server` command and `go run . server` behaves the same way.

**Architecture:** Keep the implementation small and standard-library only. Move the current server bootstrap path into repository-root `main.go`, add a testable `run(args, stdout, stderr)` dispatcher for `server` and help handling, then remove the old `cmd/server` entrypoint and update the README to point at `go run .`.

**Tech Stack:** Go 1.23, standard library `os`/`io`/`fmt`/`net/http`, existing bootstrap/config packages, `go test`

---

## File Map

| File | Responsibility |
|---|---|
| `main.go` | Root binary entrypoint, CLI dispatch, help output, and server startup |
| `main_test.go` | Command dispatch tests and existing `serviceURL` coverage |
| `cmd/server/main.go` | Old direct entrypoint to remove |
| `README.md` | Local run instructions updated to `go run .` |

## Task 1: Add testable root CLI dispatch

**Files:**
- Modify: `main.go`
- Test: `main_test.go`

- [ ] **Step 1: Write the failing dispatch tests**

Add focused tests to `main_test.go` for:

```go
func TestRunDefaultsToServerCommand(t *testing.T) {
	called := 0
	exitCode := runWithServer(nil, io.Discard, io.Discard, func(io.Writer) int {
		called++
		return 0
	})
	if exitCode != 0 {
		t.Fatalf("exitCode = %d", exitCode)
	}
	if called != 1 {
		t.Fatalf("called = %d, want 1", called)
	}
}

func TestRunExplicitServerCommand(t *testing.T) {
	called := 0
	exitCode := runWithServer([]string{"server"}, io.Discard, io.Discard, func(io.Writer) int {
		called++
		return 0
	})
	if exitCode != 0 {
		t.Fatalf("exitCode = %d", exitCode)
	}
	if called != 1 {
		t.Fatalf("called = %d, want 1", called)
	}
}
```

Add tests for help and unknown commands:

```go
func TestRunHelpAliasesWriteUsage(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		var stdout bytes.Buffer
		exitCode := runWithServer(args, &stdout, io.Discard, func(io.Writer) int {
			t.Fatal("server should not run for help")
			return 1
		})
		if exitCode != 0 {
			t.Fatalf("args %v exitCode = %d", args, exitCode)
		}
		if !strings.Contains(stdout.String(), "Usage:") {
			t.Fatalf("stdout = %q", stdout.String())
		}
	}
}

func TestRunUnknownCommandReturnsError(t *testing.T) {
	var stderr bytes.Buffer
	exitCode := runWithServer([]string{"nope"}, io.Discard, &stderr, func(io.Writer) int {
		t.Fatal("server should not run for unknown command")
		return 1
	})
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
```

- [ ] **Step 2: Run the focused CLI tests to verify they fail**

Run: `go test . -run 'TestRun(Default|Explicit|HelpAliases|UnknownCommand)' -v`
Expected: FAIL because the root CLI dispatcher does not exist yet.

- [ ] **Step 3: Implement the minimal dispatcher in `main.go`**

Replace the empty root `main.go` with:

```go
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	stdhttp "net/http"

	"github.com/benenen/myclaw/internal/bootstrap"
	"github.com/benenen/myclaw/internal/config"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithServer(args, stdout, stderr, runServer)
}

func runWithServer(args []string, stdout, stderr io.Writer, server func(io.Writer) int) int {
	if len(args) == 0 {
		return server(stderr)
	}

	switch args[0] {
	case "server":
		return server(stderr)
	case "help", "-h", "--help":
		writeUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		writeUsage(stderr)
		return 1
	}
}
```

Also add:

```go
func writeUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  myclaw [server]")
	fmt.Fprintln(w, "  myclaw help")
}
```

- [ ] **Step 4: Run the focused CLI tests**

Run: `go test . -run 'TestRun(Default|Explicit|HelpAliases|UnknownCommand)' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: add root cli dispatch"
```

## Task 2: Move server startup into the root binary

**Files:**
- Modify: `main.go`
- Test: `main_test.go`

- [ ] **Step 1: Write the failing server-path test**

Add a small test that verifies `runServer` reports config/bootstrap failures with a non-zero exit code by using a controllable helper. The simplest seam is to introduce package-level function vars:

```go
var (
	loadConfig = config.Load
	newApp     = bootstrap.New
	listenAndServe = stdhttp.ListenAndServe
)
```

Then add a focused failure test:

```go
func TestRunServerReturnsFailureWhenConfigLoadFails(t *testing.T) {
	origLoadConfig := loadConfig
	loadConfig = func() (config.Config, error) {
		return config.Config{}, errors.New("boom")
	}
	defer func() { loadConfig = origLoadConfig }()

	var stderr bytes.Buffer
	exitCode := runServer(&stderr)
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}
```

- [ ] **Step 2: Run the focused server test to verify it fails**

Run: `go test . -run TestRunServerReturnsFailureWhenConfigLoadFails -v`
Expected: FAIL because `runServer` and the helper seam do not exist yet.

- [ ] **Step 3: Move the existing startup logic into `main.go`**

Implement:

```go
var (
	loadConfig     = config.Load
	newApp         = bootstrap.New
	listenAndServe = stdhttp.ListenAndServe
)

func runServer(stderr io.Writer) int {
	logger := log.New(stderr, "", log.LstdFlags)

	cfg, err := loadConfig()
	if err != nil {
		logger.Printf("load config: %v", err)
		return 1
	}

	app, err := newApp(cfg)
	if err != nil {
		logger.Printf("bootstrap app: %v", err)
		return 1
	}

	logger.Printf("web server listening on %s", serviceURL(cfg.HTTPAddr))
	if err := listenAndServe(cfg.HTTPAddr, app.Handler); err != nil {
		logger.Printf("run server: %v", err)
		return 1
	}

	return 0
}
```

Keep or copy over the existing `serviceURL` helper unchanged.

- [ ] **Step 4: Run the focused root package tests**

Run: `go test . -run 'TestRunServerReturnsFailureWhenConfigLoadFails|TestServiceURL' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "refactor: move server startup to root main"
```

## Task 3: Remove the old entrypoint and update docs

**Files:**
- Delete: `cmd/server/main.go`
- Modify: `README.md`
- Test: `main_test.go`

- [ ] **Step 1: Write the failing build verification**

Run the root package tests and build command before deleting the old file:

```bash
go test .
go build .
```

Expected: once `cmd/server/main.go` is removed later, the repository still has a single working binary entrypoint through the root package.

- [ ] **Step 2: Delete `cmd/server/main.go`**

Remove the obsolete direct startup path so `go run ./cmd/server` is no longer supported.

- [ ] **Step 3: Update `README.md` run instructions**

Change:

```bash
go run ./cmd/server
```

to:

```bash
go run .
```

Optionally add the explicit form:

```bash
go run . server
```

- [ ] **Step 4: Run targeted verification**

Run:

```bash
go test .
go build .
go build ./...
```

Expected:
- root package tests pass
- root binary builds successfully
- repository packages still build after removing `cmd/server/main.go`

- [ ] **Step 5: Commit**

```bash
git add README.md main.go main_test.go
git rm cmd/server/main.go
git commit -m "refactor: make root cli the only entrypoint"
```

## Task 4: Final regression verification

**Files:**
- Modify: none
- Test: repository-wide verification only

- [ ] **Step 1: Run the full test suite**

Run: `go test ./...`
Expected: PASS across the repository.

- [ ] **Step 2: Run the full build verification**

Run: `go build . && go build ./...`
Expected: PASS

- [ ] **Step 3: Review final diff**

Run:

```bash
git status --short
git diff -- main.go main_test.go README.md cmd/server/main.go
```

Expected: only the planned root CLI and doc changes remain.

- [ ] **Step 4: Commit**

```bash
git add main.go main_test.go README.md cmd/server/main.go
git commit -m "feat: default root cli to server"
```
