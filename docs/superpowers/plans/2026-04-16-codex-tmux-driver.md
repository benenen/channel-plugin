# Codex tmux Driver Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new `codex-tmux` driver that runs Codex inside tmux via gotmux, reuses one pane per bot session, and returns per-request output using marker-plus-prompt completion detection.

**Architecture:** Follow the existing `Driver.Init(ctx, spec) -> SessionRuntime` lifecycle used by `codex-pty`, but replace PTY reads with tmux `send-keys` writes and `capture-pane` polling. Keep the tmux implementation Codex-specific, mark the runtime broken on timeout or boundary ambiguity, and expand capability discovery/tests to advertise and validate the new mode.

**Tech Stack:** Go 1.23, gotmux, tmux, existing `internal/agent` session manager and test helpers.

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/agent/codex/driver_tmux.go` | New tmux-backed Codex driver, runtime state, marker parsing, tmux pane lifecycle |
| `internal/agent/codex/driver_tmux_test.go` | Pure parsing tests and runtime tests for tmux success/failure paths |
| `internal/app/capability/discoverer.go` | Advertise `codex-tmux` as a supported Codex mode |
| `internal/app/capability/discoverer_test.go` | Lock supported mode expectations for Codex discovery |
| `internal/bootstrap/bootstrap_test.go` | Ensure bootstrap wiring sees `codex-tmux` registration |
| `internal/agent/session_test.go` | Optional focused manager/runtime reuse coverage for the new driver name if the dedicated tmux runtime tests are insufficient |
| `go.mod` / `go.sum` | Add gotmux module dependency |

## Task 1: Add gotmux dependency and advertise the new driver mode

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `internal/app/capability/discoverer.go:26-29`
- Test: `internal/app/capability/discoverer_test.go:73-172`

- [ ] **Step 1: Write the failing discovery test for the new supported mode**

```go
func TestAgentCapabilityDiscovererRefreshesCurrentEnvironment(t *testing.T) {
	repo := &discoveryCapabilityRepoStub{}
	discoverer := NewAgentCapabilityDiscoverer(repo, func(name string) (string, error) {
		switch name {
		case "codex":
			return "/usr/local/bin/codex", nil
		case "claude":
			return "", errors.New("not found")
		default:
			t.Fatalf("unexpected command lookup: %s", name)
			return "", nil
		}
	})

	_, err := discoverer.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	codex, err := repo.GetByKey(context.Background(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	wantModes := []string{"codex-exec", "codex-pty", "codex-tmux"}
	if !reflect.DeepEqual(codex.SupportedModes, wantModes) {
		t.Fatalf("SupportedModes = %#v, want %#v", codex.SupportedModes, wantModes)
	}
}
```

- [ ] **Step 2: Run the focused discovery test to verify it fails**

Run: `go test ./internal/app/capability -run TestAgentCapabilityDiscovererRefreshesCurrentEnvironment`
Expected: FAIL because `codex-tmux` is not yet in `SupportedModes`

- [ ] **Step 3: Add the new supported mode in capability discovery**

```go
var capabilitySeeds = []capabilitySeed{
	{key: "codex", label: "Codex CLI", command: "codex", supportedModes: []string{"codex-exec", "codex-pty", "codex-tmux"}},
	{key: "claude", label: "Claude Code", command: "claude", supportedModes: []string{}},
}
```

- [ ] **Step 4: Add the gotmux dependency to the module file**

```go
require (
	github.com/GianlucaP106/gotmux v0.0.0-PLACEHOLDER
	...
)
```

Replace `v0.0.0-PLACEHOLDER` with the actual version selected by `go get github.com/GianlucaP106/gotmux@<version>` and keep the rest of `go.mod` formatting untouched.

- [ ] **Step 5: Download the dependency and update `go.sum`**

Run: `go get github.com/GianlucaP106/gotmux@<version>`
Expected: `go.mod` and `go.sum` updated with gotmux dependency

- [ ] **Step 6: Re-run the focused discovery test to verify it passes**

Run: `go test ./internal/app/capability -run TestAgentCapabilityDiscovererRefreshesCurrentEnvironment`
Expected: PASS

- [ ] **Step 7: Commit the capability and dependency change**

```bash
git add go.mod go.sum internal/app/capability/discoverer.go internal/app/capability/discoverer_test.go
git commit -m "feat: add codex tmux capability mode"
```

## Task 2: Add driver registration coverage before implementation

**Files:**
- Modify: `internal/bootstrap/bootstrap_test.go:77-84`
- Test: `internal/bootstrap/bootstrap_test.go:77-84`

- [ ] **Step 1: Write the failing bootstrap assertion for `codex-tmux`**

```go
func TestBootstrapUsesRegisteredDriverTypeForBotCLI(t *testing.T) {
	if _, ok := agent.LookupDriver("codex-pty"); !ok {
		t.Fatal("expected codex-pty driver registration for bootstrap wiring")
	}
	if _, ok := agent.LookupDriver("codex-exec"); !ok {
		t.Fatal("expected codex-exec driver registration for bootstrap wiring")
	}
	if _, ok := agent.LookupDriver("codex-tmux"); !ok {
		t.Fatal("expected codex-tmux driver registration for bootstrap wiring")
	}
}
```

- [ ] **Step 2: Run the focused bootstrap test to verify it fails**

Run: `go test ./internal/bootstrap -run TestBootstrapUsesRegisteredDriverTypeForBotCLI`
Expected: FAIL because no `codex-tmux` driver is registered yet

- [ ] **Step 3: Leave the test in place and defer the implementation to the next task**

```go
// No production code change in this step.
// The failing assertion stays as the guardrail for driver registration.
```

- [ ] **Step 4: Commit the failing-first registration test once the driver exists in Task 3**

```bash
git add internal/bootstrap/bootstrap_test.go internal/agent/codex/driver_tmux.go internal/agent/codex/driver_tmux_test.go
git commit -m "feat: add codex tmux driver runtime"
```

## Task 3: Build the tmux runtime and registration path with focused tests

**Files:**
- Create: `internal/agent/codex/driver_tmux.go`
- Create: `internal/agent/codex/driver_tmux_test.go`
- Test: `internal/bootstrap/bootstrap_test.go:77-84`

- [ ] **Step 1: Write the failing driver registration and empty-command tests**

```go
func TestTMUXDriverRegistersCodexTMUX(t *testing.T) {
	driver, ok := agent.LookupDriver("codex-tmux")
	if !ok {
		t.Fatal("expected codex-tmux driver registration")
	}
	if driver == nil {
		t.Fatal("expected non-nil driver")
	}
}

func TestTMUXDriverInitRejectsEmptyCommand(t *testing.T) {
	driver := NewTMUXDriver()
	_, err := driver.Init(context.Background(), agent.Spec{Type: "codex-tmux"})
	if err == nil {
		t.Fatal("expected empty command error")
	}
}
```

- [ ] **Step 2: Run the focused tmux tests to verify they fail**

Run: `go test ./internal/agent/codex -run 'TestTMUXDriverRegistersCodexTMUX|TestTMUXDriverInitRejectsEmptyCommand'`
Expected: FAIL because `driver_tmux.go` does not exist yet

- [ ] **Step 3: Create the driver skeleton with registration, runtime state, and constructor**

```go
package codex

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/GianlucaP106/gotmux"
	"github.com/benenen/myclaw/internal/agent"
)

type TMUXDriver struct{}

type TMUXRuntime struct {
	mu    sync.Mutex
	runMu sync.Mutex

	state runtimeState

	server      *gotmux.Server
	session     *gotmux.Session
	window      *gotmux.Window
	pane        *gotmux.Pane
	sessionName string
	prompt      string
	readErr     error
}

func init() {
	agent.MustRegisterDriver("codex-tmux", func() agent.Driver {
		return NewTMUXDriver()
	})
}

func NewTMUXDriver() *TMUXDriver {
	return &TMUXDriver{}
}

func (d *TMUXDriver) Init(ctx context.Context, spec agent.Spec) (agent.SessionRuntime, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return nil, fmt.Errorf("codex tmux driver requires command")
	}
	return nil, fmt.Errorf("codex tmux init not implemented")
}
```

- [ ] **Step 4: Re-run the focused tmux registration tests to verify partial progress**

Run: `go test ./internal/agent/codex -run 'TestTMUXDriverRegistersCodexTMUX|TestTMUXDriverInitRejectsEmptyCommand'`
Expected: PASS

- [ ] **Step 5: Add fake tmux abstractions for init/run tests before filling behavior**

```go
type tmuxPane interface {
	SendKeys(keys ...string) error
	CapturePane() (string, error)
}

type tmuxSession interface {
	Kill() error
}

type tmuxRuntimeFactory interface {
	Start(ctx context.Context, spec agent.Spec, sessionName string) (tmuxSession, tmuxPane, error)
}
```

Keep these types in `driver_tmux.go` so the runtime logic can be tested without requiring a real tmux server.

- [ ] **Step 6: Write the failing init/run success test against fake pane content**

```go
func TestTMUXRuntimeRunSuccessfulSingleRequest(t *testing.T) {
	runtime := &TMUXRuntime{
		state:  stateReady,
		prompt: "codex>",
		pane: &fakePane{
			captures: []string{
				"codex>\n",
				"__MYCLAW_CODEX_RUN_BEGIN_1__\nassistant response: say hello\n__MYCLAW_CODEX_RUN_END_1__\ncodex>\n",
			},
		},
	}

	resp, err := runtime.Run(context.Background(), agent.Request{Prompt: "say hello"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Text != "assistant response: say hello" {
		t.Fatalf("Run() text = %q", resp.Text)
	}
}
```

- [ ] **Step 7: Run the focused success test to verify it fails**

Run: `go test ./internal/agent/codex -run TestTMUXRuntimeRunSuccessfulSingleRequest`
Expected: FAIL because `Run` and fake pane support are not implemented yet

- [ ] **Step 8: Implement the minimal init, run, and close path for one healthy request**

```go
func (d *TMUXDriver) Init(ctx context.Context, spec agent.Spec) (agent.SessionRuntime, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return nil, fmt.Errorf("codex tmux driver requires command")
	}

	runtime := &TMUXRuntime{
		state:  stateStarting,
		prompt: defaultPrompt,
	}

	if err := runtime.waitUntilReady(ctx); err != nil {
		runtime.markBroken(err)
		return nil, err
	}
	return runtime, nil
}

func (r *TMUXRuntime) Run(ctx context.Context, req agent.Request) (agent.Response, error) {
	r.runMu.Lock()
	defer r.runMu.Unlock()

	promptText := strings.TrimSpace(req.Prompt)
	if promptText == "" {
		return agent.Response{}, fmt.Errorf("codex tmux request prompt is required")
	}

	r.mu.Lock()
	if r.state == stateBroken {
		err := r.readErr
		if err == nil {
			err = fmt.Errorf("codex tmux runtime is broken")
		}
		r.mu.Unlock()
		return agent.Response{}, err
	}
	r.state = stateRunning
	pane := r.pane
	prompt := r.prompt
	r.mu.Unlock()

	runID := nextTMUXRunID()
	beginMarker := "__MYCLAW_CODEX_RUN_BEGIN_" + runID + "__"
	endMarker := "__MYCLAW_CODEX_RUN_END_" + runID + "__"
	if err := pane.SendKeys(beginMarker, "C-m", promptText, "C-m", endMarker, "C-m"); err != nil {
		r.markBroken(fmt.Errorf("codex tmux send failed: %w", err))
		return agent.Response{}, r.currentError()
	}

	text, err := r.waitRunCompletion(ctx, beginMarker, endMarker, prompt)
	if err != nil {
		r.markBroken(err)
		return agent.Response{}, err
	}

	r.mu.Lock()
	if r.state != stateBroken {
		r.state = stateReady
	}
	r.mu.Unlock()

	return agent.Response{Text: text, ExitCode: 0}, nil
}
```

Use small helper methods for `waitUntilReady`, `waitRunCompletion`, `markBroken`, and `currentError` rather than pushing all logic into `Run`.

- [ ] **Step 9: Re-run the focused tmux tests and bootstrap registration test**

Run: `go test ./internal/agent/codex -run 'TestTMUXDriverRegistersCodexTMUX|TestTMUXDriverInitRejectsEmptyCommand|TestTMUXRuntimeRunSuccessfulSingleRequest' && go test ./internal/bootstrap -run TestBootstrapUsesRegisteredDriverTypeForBotCLI`
Expected: PASS

- [ ] **Step 10: Commit the first working tmux runtime**

```bash
git add internal/agent/codex/driver_tmux.go internal/agent/codex/driver_tmux_test.go internal/bootstrap/bootstrap_test.go
git commit -m "feat: add codex tmux driver runtime"
```

## Task 4: Tighten marker parsing, prompt detection, and broken-state behavior with tests

**Files:**
- Modify: `internal/agent/codex/driver_tmux.go`
- Modify: `internal/agent/codex/driver_tmux_test.go`

- [ ] **Step 1: Write the failing pure tests for slicing and prompt handling**

```go
func TestExtractTMUXRunResultPreservesPromptLikeOutput(t *testing.T) {
	text := strings.Join([]string{
		"ordinary output: codex> appears inside text",
		"__MYCLAW_CODEX_RUN_BEGIN_1__",
		"assistant response: check prompt-like text",
		"__MYCLAW_CODEX_RUN_END_1__",
		"codex>",
	}, "\n")

	got, err := extractTMUXRunResult(text, "__MYCLAW_CODEX_RUN_BEGIN_1__", "__MYCLAW_CODEX_RUN_END_1__", "codex>")
	if err != nil {
		t.Fatalf("extractTMUXRunResult() error = %v", err)
	}
	if got != "assistant response: check prompt-like text" {
		t.Fatalf("extractTMUXRunResult() = %q", got)
	}
}

func TestExtractTMUXRunResultRejectsMissingPromptAfterEnd(t *testing.T) {
	text := strings.Join([]string{
		"__MYCLAW_CODEX_RUN_BEGIN_1__",
		"assistant response: say hello",
		"__MYCLAW_CODEX_RUN_END_1__",
	}, "\n")

	_, err := extractTMUXRunResult(text, "__MYCLAW_CODEX_RUN_BEGIN_1__", "__MYCLAW_CODEX_RUN_END_1__", "codex>")
	if err == nil {
		t.Fatal("expected missing prompt error")
	}
}
```

- [ ] **Step 2: Run the focused parsing tests to verify they fail**

Run: `go test ./internal/agent/codex -run 'TestExtractTMUXRunResultPreservesPromptLikeOutput|TestExtractTMUXRunResultRejectsMissingPromptAfterEnd'`
Expected: FAIL because extraction helpers are not implemented or not strict enough

- [ ] **Step 3: Implement the parsing helpers and cleanup path**

```go
func extractTMUXRunResult(text, beginMarker, endMarker, prompt string) (string, error) {
	normalized := normalizeOutput(text)
	begin := strings.LastIndex(normalized, beginMarker)
	if begin < 0 {
		return "", fmt.Errorf("codex tmux output missing begin marker")
	}
	endSearchStart := begin + len(beginMarker)
	end := strings.Index(normalized[endSearchStart:], endMarker)
	if end < 0 {
		return "", fmt.Errorf("codex tmux output missing end marker")
	}
	end += endSearchStart
	promptIdx, ok := promptIndexOnOwnLine(normalized[end+len(endMarker):], prompt)
	if !ok {
		return "", fmt.Errorf("codex tmux prompt not restored after end marker")
	}
	_ = promptIdx
	body := normalized[endSearchStart:end]
	return cleanupTMUXRunText(body), nil
}

func cleanupTMUXRunText(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "__MYCLAW_CODEX_RUN_BEGIN_") || strings.HasPrefix(trimmed, "__MYCLAW_CODEX_RUN_END_") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}
```

- [ ] **Step 4: Write the failing broken-state runtime tests**

```go
func TestTMUXRuntimeRunTimeoutMarksBroken(t *testing.T) {
	runtime := &TMUXRuntime{
		state:  stateReady,
		prompt: "codex>",
		pane: &fakePane{captures: []string{"__MYCLAW_CODEX_RUN_BEGIN_1__\nworking...\n"}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := runtime.Run(ctx, agent.Request{Prompt: "stall forever"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if runtime.state != stateBroken {
		t.Fatalf("runtime state = %s, want broken", runtime.state)
	}
}
```

- [ ] **Step 5: Run the focused broken-state tests to verify they fail**

Run: `go test ./internal/agent/codex -run TestTMUXRuntimeRunTimeoutMarksBroken`
Expected: FAIL because timeout/broken handling is incomplete

- [ ] **Step 6: Implement strict timeout and broken-state helpers**

```go
func (r *TMUXRuntime) waitRunCompletion(ctx context.Context, beginMarker, endMarker, prompt string) (string, error) {
	for {
		captured, err := r.pane.CapturePane()
		if err != nil {
			return "", fmt.Errorf("codex tmux capture failed: %w", err)
		}
		if text, err := extractTMUXRunResult(captured, beginMarker, endMarker, prompt); err == nil {
			return text, nil
		}
		if ctx.Err() != nil {
			return "", fmt.Errorf("codex tmux run timed out: %w", ctx.Err())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (r *TMUXRuntime) markBroken(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err == nil {
		err = fmt.Errorf("codex tmux runtime is broken")
	}
	r.readErr = err
	r.state = stateBroken
}
```

- [ ] **Step 7: Re-run the focused parsing and broken-state tests**

Run: `go test ./internal/agent/codex -run 'TestExtractTMUXRunResultPreservesPromptLikeOutput|TestExtractTMUXRunResultRejectsMissingPromptAfterEnd|TestTMUXRuntimeRunTimeoutMarksBroken'`
Expected: PASS

- [ ] **Step 8: Commit the stricter runtime behavior**

```bash
git add internal/agent/codex/driver_tmux.go internal/agent/codex/driver_tmux_test.go
git commit -m "fix: tighten codex tmux runtime boundaries"
```

## Task 5: Cover session-manager reuse and full-package verification

**Files:**
- Modify: `internal/agent/session_test.go` (only if dedicated tmux runtime coverage is insufficient)
- Test: `internal/agent/codex/driver_tmux_test.go`
- Test: `internal/app/capability/discoverer_test.go`
- Test: `internal/bootstrap/bootstrap_test.go`
- Test: `internal/agent/session_test.go`

- [ ] **Step 1: Add a focused manager reuse test only if the tmux runtime tests do not already prove serialization and reuse**

```go
func TestManagerUsesCodexTMUXDriverBySpecType(t *testing.T) {
	const driverName = "codex-tmux"
	if _, ok := LookupDriver(driverName); !ok {
		t.Fatalf("expected %s to be registered", driverName)
	}
}
```

If the existing `internal/agent/codex/driver_tmux_test.go` already covers runtime serialization and manager compatibility well enough, skip modifying `internal/agent/session_test.go`.

- [ ] **Step 2: Run the focused tmux package tests**

Run: `go test ./internal/agent/codex`
Expected: PASS

- [ ] **Step 3: Run the related package tests for discovery, bootstrap, and agent session behavior**

Run: `go test ./internal/app/capability ./internal/bootstrap ./internal/agent`
Expected: PASS

- [ ] **Step 4: Run the full test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit the final verified integration**

```bash
git add internal/agent/codex/driver_tmux.go internal/agent/codex/driver_tmux_test.go internal/app/capability/discoverer.go internal/app/capability/discoverer_test.go internal/bootstrap/bootstrap_test.go internal/agent/session_test.go go.mod go.sum
git commit -m "feat: add codex tmux driver"
```

## Self-Review

| Check | Result |
|---|---|
| Spec coverage | Covered driver lifecycle, begin/end markers, prompt restoration, broken-state handling, gotmux adoption, capability discovery, bootstrap registration, and test strategy |
| Placeholder scan | One explicit gotmux version placeholder remains intentionally tied to the `go get` command so the implementer uses the current resolvable version instead of copying a stale value |
| Type consistency | `TMUXDriver`, `TMUXRuntime`, `extractTMUXRunResult`, `cleanupTMUXRunText`, and `codex-tmux` naming are consistent across all tasks |

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-16-codex-tmux-driver.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
