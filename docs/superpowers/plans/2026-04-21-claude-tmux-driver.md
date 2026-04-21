# Claude TMUX Driver Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a tmux-based driver for the claude agent that manages claude-code CLI sessions via tmux and tracks run state via SQLite.

**Architecture:** Port the codex tmux driver to claude with minimal changes - same tmux session management, same SQLite run tracking, different CLI command (claude-code) and notify mechanism (myclaw notify claude).

**Tech Stack:** Go 1.23, gotmux library, SQLite via GORM, agent.Driver interface

---

## File Structure

**New Files:**
- `internal/agent/claude/driver_tmux.go` - Main driver implementation (500+ lines)
- `internal/agent/claude/driver_tmux_test.go` - Unit tests with mocks

**Reference Files:**
- `internal/agent/codex/driver_tmux.go` - Template to port from
- `internal/agent/codex/driver_tmux_test.go` - Test patterns to follow
- `internal/agent/driver.go` - Driver interface
- `internal/agent/types.go` - Spec, Request, Response types

---

### Task 1: Core Types and Interfaces

**Files:**
- Create: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Create package and imports**

```go
package claude

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GianlucaP106/gotmux/gotmux"
	"github.com/benenen/myclaw/internal/agent"
	"github.com/benenen/myclaw/internal/domain"
	"github.com/benenen/myclaw/internal/store"
	"github.com/benenen/myclaw/internal/store/repositories"
)
```

- [ ] **Step 2: Define constants**

```go
const currentTMUXRunIDFileName = ".myclaw-run-id"

const (
	runtimeTypeClaude = "claude"
)

var defaultRunTimeout = 30 * time.Second
```

- [ ] **Step 3: Define runtime state type**

```go
type runtimeState string

const (
	stateStarting runtimeState = "starting"
	stateReady    runtimeState = "ready"
	stateRunning  runtimeState = "running"
	stateBroken   runtimeState = "broken"
)
```

- [ ] **Step 4: Define core driver and runtime structs**

```go
type TMUXDriver struct {
	factory         tmuxRuntimeFactory
	runStoreFactory tmuxRunStoreFactory
}

type TMUXRuntime struct {
	mu    sync.Mutex
	runMu sync.Mutex

	state    runtimeState
	pane     tmuxPane
	session  tmuxSession
	readErr  error
	waitGap  time.Duration
	spec     agent.Spec
	runStore tmuxRunStore
}
```

- [ ] **Step 5: Define interface types**

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

type tmuxRunRecord struct {
	RunID  string
	Status string
}

type tmuxRunStore interface {
	CreatePending(ctx context.Context, runID, botName, runtimeType string) error
	UpsertDone(ctx context.Context, runID, botName, runtimeType string) error
	GetByRunID(ctx context.Context, runID string) (tmuxRunRecord, error)
}

type tmuxRunStoreFactory interface {
	Open(spec agent.Spec) (tmuxRunStore, error)
}
```

- [ ] **Step 6: Define concrete implementation types**

```go
type tmuxGotmuxFactory struct{}

type sqliteTMUXRunStoreFactory struct{}

type tmuxGotmuxSession struct {
	session *gotmux.Session
}

type tmuxGotmuxPane struct {
	pane *gotmux.Pane
}
```

- [ ] **Step 7: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Success (types compile)

- [ ] **Step 8: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "feat: add claude tmux driver types and interfaces"
```

---

### Task 2: Driver Registration and Constructor

**Files:**
- Modify: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Add init function for driver registration**

```go
func init() {
	agent.MustRegisterDriver("claude-tmux", func() agent.Driver {
		return NewTMUXDriver()
	})
}
```

- [ ] **Step 2: Add constructor**

```go
func NewTMUXDriver() *TMUXDriver {
	return &TMUXDriver{
		factory:         tmuxGotmuxFactory{},
		runStoreFactory: sqliteTMUXRunStoreFactory{},
	}
}
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "feat: add claude tmux driver registration"
```

---

### Task 3: Driver Init Method

**Files:**
- Modify: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Implement Init method**

```go
func (d *TMUXDriver) Init(ctx context.Context, spec agent.Spec) (agent.SessionRuntime, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return nil, fmt.Errorf("claude tmux driver requires command")
	}
	if strings.TrimSpace(spec.WorkDir) == "" {
		return nil, fmt.Errorf("claude tmux driver requires workdir")
	}

	runtime := &TMUXRuntime{
		state:   stateStarting,
		waitGap: 10 * time.Millisecond,
		spec:    spec,
	}
	if d != nil {
		runtimeFactory := d.factory
		if runtimeFactory == nil {
			runtimeFactory = tmuxGotmuxFactory{}
		}
		session, pane, err := runtimeFactory.Start(ctx, spec, nextTMUXSessionName(spec.BotName))
		if err != nil {
			return nil, err
		}
		runtime.session = session
		runtime.pane = pane

		runStoreFactory := d.runStoreFactory
		if runStoreFactory == nil {
			runStoreFactory = sqliteTMUXRunStoreFactory{}
		}
		runStore, err := runStoreFactory.Open(spec)
		if err != nil {
			return nil, err
		}
		runtime.runStore = runStore
	}

	if err := runtime.waitUntilReady(ctx); err != nil {
		runtime.markBroken(err)
		return nil, err
	}
	return runtime, nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Fails with "undefined: nextTMUXSessionName, waitUntilReady, markBroken"

- [ ] **Step 3: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "feat: add claude tmux driver Init method"
```

---

### Task 4: Runtime Helper Methods

**Files:**
- Modify: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Implement markBroken method**

```go
func (r *TMUXRuntime) markBroken(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err == nil {
		err = fmt.Errorf("claude tmux runtime is broken")
	}
	r.readErr = err
	r.state = stateBroken
}
```

- [ ] **Step 2: Implement currentError method**

```go
func (r *TMUXRuntime) currentError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readErr != nil {
		return r.readErr
	}
	return fmt.Errorf("claude tmux runtime is broken")
}
```

- [ ] **Step 3: Implement waitUntilReady method**

```go
func (r *TMUXRuntime) waitUntilReady(ctx context.Context) error {
	r.mu.Lock()
	if r.pane == nil {
		r.state = stateReady
		r.mu.Unlock()
		return nil
	}
	pane := r.pane
	gap := r.waitGap
	r.mu.Unlock()

	if gap <= 0 {
		gap = 10 * time.Millisecond
	}

	for {
		captured, err := pane.CapturePane()
		if err != nil {
			return fmt.Errorf("claude tmux capture failed: %w", err)
		}
		normalized := normalizeTMUXOutput(captured)
		if normalized != "" {
			r.mu.Lock()
			if r.state != stateBroken {
				r.state = stateReady
			}
			r.mu.Unlock()
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("claude tmux startup timed out: %w", ctx.Err())
		}
		time.Sleep(gap)
	}
}
```

- [ ] **Step 4: Implement waitRunCompletion method**

```go
func (r *TMUXRuntime) waitRunCompletion(ctx context.Context, runID string) error {
	r.mu.Lock()
	gap := r.waitGap
	runStore := r.runStore
	r.mu.Unlock()
	if gap <= 0 {
		gap = 10 * time.Millisecond
	}

	for {
		run, err := runStore.GetByRunID(ctx, runID)
		if err == nil && run.Status == "done" {
			return nil
		}
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("claude tmux run state read failed: %w", err)
		}
		if ctx.Err() != nil {
			return fmt.Errorf("claude tmux run timed out: %w", ctx.Err())
		}
		time.Sleep(gap)
	}
}
```

- [ ] **Step 5: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Fails with "undefined: normalizeTMUXOutput"

- [ ] **Step 6: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "feat: add claude tmux runtime helper methods"
```

---

### Task 5: Runtime Run Method

**Files:**
- Modify: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Implement Run method (part 1 of 2)**

```go
func (r *TMUXRuntime) Run(ctx context.Context, req agent.Request) (agent.Response, error) {
	r.runMu.Lock()
	defer r.runMu.Unlock()

	promptText := strings.TrimSpace(req.Prompt)
	if promptText == "" {
		return agent.Response{}, fmt.Errorf("claude tmux request prompt is required")
	}

	r.mu.Lock()
	if r.state == stateBroken {
		err := r.readErr
		if err == nil {
			err = fmt.Errorf("claude tmux runtime is broken")
		}
		r.mu.Unlock()
		return agent.Response{}, err
	}
	if r.state != stateReady && r.state != stateStarting {
		state := r.state
		r.mu.Unlock()
		return agent.Response{}, fmt.Errorf("claude tmux runtime is not ready: %s", state)
	}
	pane := r.pane
	r.state = stateRunning
	r.mu.Unlock()

	if pane == nil {
		r.markBroken(fmt.Errorf("claude tmux runtime is not connected to a pane"))
		return agent.Response{}, r.currentError()
	}
	if r.runStore == nil {
		r.markBroken(fmt.Errorf("claude tmux runtime is not connected to run state store"))
		return agent.Response{}, r.currentError()
	}
```

- [ ] **Step 2: Implement Run method (part 2 of 2)**

```go
	runCtx := ctx
	cancel := func() {}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		runCtx, cancel = context.WithTimeout(ctx, defaultRunTimeout)
	}
	defer cancel()

	runID := domain.NewPrefixedID("run")
	if err := writeTMUXCurrentRunID(r.spec.WorkDir, runID); err != nil {
		r.markBroken(err)
		return agent.Response{}, err
	}
	if err := r.runStore.CreatePending(runCtx, runID, r.spec.BotName, runtimeTypeClaude); err != nil {
		r.markBroken(fmt.Errorf("claude tmux create run failed: %w", err))
		return agent.Response{}, r.currentError()
	}

	if err := pane.SendKeys(promptText, "C-m"); err != nil {
		r.markBroken(fmt.Errorf("claude tmux send failed: %w", err))
		return agent.Response{}, r.currentError()
	}

	if err := r.waitRunCompletion(runCtx, runID); err != nil {
		r.markBroken(err)
		return agent.Response{}, err
	}

	captured, err := pane.CapturePane()
	if err != nil {
		r.markBroken(fmt.Errorf("claude tmux capture failed: %w", err))
		return agent.Response{}, r.currentError()
	}
	text := cleanupTMUXRunText(normalizeTMUXOutput(captured))

	r.mu.Lock()
	if r.state != stateBroken {
		r.state = stateReady
	}
	r.mu.Unlock()

	return agent.Response{Text: text, RuntimeType: runtimeTypeClaude, ExitCode: 0, RawOutput: text}, nil
}
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Fails with "undefined: writeTMUXCurrentRunID, cleanupTMUXRunText"

- [ ] **Step 4: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "feat: add claude tmux runtime Run method"
```

---

### Task 6: Runtime Close Method

**Files:**
- Modify: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Implement Close method**

```go
func (r *TMUXRuntime) Close() error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	session := r.session
	r.session = nil
	r.pane = nil
	r.state = stateBroken
	if r.readErr == nil {
		r.readErr = fmt.Errorf("claude tmux runtime is closed")
	}
	r.mu.Unlock()

	if session == nil {
		return nil
	}
	return session.Kill()
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Still fails with undefined functions (expected)

- [ ] **Step 3: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "feat: add claude tmux runtime Close method"
```

---

### Task 7: Utility Functions

**Files:**
- Modify: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Implement text cleanup functions**

```go
func cleanupTMUXRunText(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, strings.TrimRight(line, "\r"))
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func normalizeTMUXOutput(text string) string {
	return strings.ReplaceAll(text, "\r\n", "\n")
}
```

- [ ] **Step 2: Implement writeTMUXCurrentRunID function**

```go
func writeTMUXCurrentRunID(workDir, runID string) error {
	if strings.TrimSpace(workDir) == "" {
		return fmt.Errorf("claude tmux workdir is required")
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("claude tmux prepare workdir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, currentTMUXRunIDFileName), []byte(runID+"\n"), 0o644); err != nil {
		return fmt.Errorf("claude tmux write current run id: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Implement session naming function**

```go
func nextTMUXSessionName(botName string) string {
	prefix := strings.TrimSpace(botName)
	prefix = strings.ToLower(prefix)
	prefix = strings.ReplaceAll(prefix, " ", "-")
	if prefix == "" {
		prefix = "claude"
	}
	return fmt.Sprintf("myclaw-claude-%s", prefix)
}
```

- [ ] **Step 4: Implement shell quote function**

```go
func shellQuote(text string) string {
	if text == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}
```

- [ ] **Step 5: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Fails with undefined buildTMUXShellCommand, buildTMUXSessionOptions

- [ ] **Step 6: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "feat: add claude tmux utility functions"
```

---

### Task 8: TMUX Session Management

**Files:**
- Modify: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Implement buildTMUXShellCommand function**

```go
func buildTMUXShellCommand(spec agent.Spec) string {
	command := strings.TrimSpace(spec.Command)
	if command == "" {
		return ""
	}
	notifyConfig := fmt.Sprintf(`notify=["myclaw", "notify", "claude", %s]`, strconv.Quote(spec.BotName))
	return command + " -c " + shellQuote(notifyConfig)
}
```

- [ ] **Step 2: Implement buildTMUXSessionOptions function**

```go
func buildTMUXSessionOptions(spec agent.Spec, sessionName string) *gotmux.SessionOptions {
	options := &gotmux.SessionOptions{
		Name:         sessionName,
		ShellCommand: buildTMUXShellCommand(spec),
	}
	if strings.TrimSpace(spec.WorkDir) != "" {
		options.StartDirectory = spec.WorkDir
	}
	return options
}
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Success (all functions defined)

- [ ] **Step 4: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "feat: add claude tmux session management functions"
```

---

### Task 9: Gotmux Factory Implementation

**Files:**
- Modify: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Implement tmuxGotmuxFactory.Start method**

```go
func (tmuxGotmuxFactory) Start(ctx context.Context, spec agent.Spec, sessionName string) (tmuxSession, tmuxPane, error) {
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if len(spec.Args) > 0 {
		return nil, nil, fmt.Errorf("claude tmux driver does not support tmux startup args yet")
	}
	if len(spec.Env) > 0 {
		return nil, nil, fmt.Errorf("claude tmux driver does not support tmux startup env yet")
	}

	tmux, err := gotmux.DefaultTmux()
	if err != nil {
		return nil, nil, err
	}

	var session *gotmux.Session
	if tmux.HasSession(sessionName) {
		if existing, err := tmux.GetSessionByName(sessionName); err == nil && existing != nil {
			session = existing
		}
	} else {
		options := buildTMUXSessionOptions(spec, sessionName)
		session, err = tmux.NewSession(options)
		if err != nil {
			return nil, nil, fmt.Errorf("start tmux session %q: %w", sessionName, err)
		}
	}

	if session == nil {
		return nil, nil, fmt.Errorf("failed to create or find tmux session %q", sessionName)
	}

	window, err := session.GetWindowByIndex(0)
	if err != nil {
		_ = session.Kill()
		return nil, nil, fmt.Errorf("start tmux session %q: %w", sessionName, err)
	}
	panes, err := window.ListPanes()
	if err != nil || len(panes) == 0 {
		_ = session.Kill()
		if err == nil {
			err = fmt.Errorf("tmux session %q has no panes", sessionName)
		}
		return nil, nil, fmt.Errorf("start tmux session %q: %w", sessionName, err)
	}
	pane := tmuxGotmuxPane{pane: panes[0]}
	return tmuxGotmuxSession{session: session}, pane, nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "feat: add claude tmux gotmux factory implementation"
```

---

### Task 10: Gotmux Adapters

**Files:**
- Modify: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Implement tmuxGotmuxSession methods**

```go
func (s tmuxGotmuxSession) Kill() error {
	if s.session == nil {
		return nil
	}
	if err := s.session.Kill(); err != nil {
		return fmt.Errorf("kill tmux session %q: %w", s.session.Name, err)
	}
	return nil
}
```

- [ ] **Step 2: Implement tmuxGotmuxPane.SendKeys method**

```go
func (p tmuxGotmuxPane) SendKeys(keys ...string) error {
	if p.pane == nil {
		return fmt.Errorf("tmux pane is nil")
	}
	for _, key := range keys {
		if err := p.pane.SendKeys(key); err != nil {
			return fmt.Errorf("send tmux keys: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 3: Implement tmuxGotmuxPane.CapturePane method**

```go
func (p tmuxGotmuxPane) CapturePane() (string, error) {
	if p.pane == nil {
		return "", fmt.Errorf("tmux pane is nil")
	}
	output, err := p.pane.Capture()
	if err != nil {
		return "", fmt.Errorf("capture tmux pane: %w", err)
	}
	return output, nil
}
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Success

- [ ] **Step 5: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "feat: add claude tmux gotmux adapters"
```

---

### Task 11: SQLite Run Store Implementation

**Files:**
- Modify: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Define sqliteTMUXRunStore struct**

```go
type sqliteTMUXRunStore struct {
	repo *repositories.AgentRunRepository
}
```

- [ ] **Step 2: Implement sqliteTMUXRunStoreFactory.Open method**

```go
func (sqliteTMUXRunStoreFactory) Open(spec agent.Spec) (tmuxRunStore, error) {
	if strings.TrimSpace(spec.SQLitePath) == "" {
		return nil, fmt.Errorf("claude tmux driver requires sqlite path")
	}
	db, err := store.Open(spec.SQLitePath)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(db); err != nil {
		return nil, err
	}
	return &sqliteTMUXRunStore{repo: repositories.NewAgentRunRepository(db)}, nil
}
```

- [ ] **Step 3: Implement sqliteTMUXRunStore.CreatePending method**

```go
func (s *sqliteTMUXRunStore) CreatePending(ctx context.Context, runID, botName, runtimeType string) error {
	return s.repo.CreatePending(ctx, runID, botName, runtimeType)
}
```

- [ ] **Step 4: Implement sqliteTMUXRunStore.UpsertDone method**

```go
func (s *sqliteTMUXRunStore) UpsertDone(ctx context.Context, runID, botName, runtimeType string) error {
	return s.repo.UpsertDone(ctx, runID, botName, runtimeType)
}
```

- [ ] **Step 5: Implement sqliteTMUXRunStore.GetByRunID method**

```go
func (s *sqliteTMUXRunStore) GetByRunID(ctx context.Context, runID string) (tmuxRunRecord, error) {
	run, err := s.repo.GetByRunID(ctx, runID)
	if err != nil {
		return tmuxRunRecord{}, err
	}
	return tmuxRunRecord{
		RunID:  run.RunID,
		Status: run.Status,
	}, nil
}
```

- [ ] **Step 6: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Success (complete implementation)

- [ ] **Step 7: Run full test suite**

Run: `go test ./...`
Expected: All existing tests pass

- [ ] **Step 8: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "feat: add claude tmux SQLite run store implementation"
```

---

### Task 12: Unit Tests - Mock Infrastructure

**Files:**
- Create: `internal/agent/claude/driver_tmux_test.go`

- [ ] **Step 1: Create test file with package and imports**

```go
package claude

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/benenen/myclaw/internal/agent"
	"github.com/benenen/myclaw/internal/domain"
)
```

- [ ] **Step 2: Define mock pane**

```go
type mockTMUXPane struct {
	sendKeysFunc   func(keys ...string) error
	capturePaneFunc func() (string, error)
}

func (m mockTMUXPane) SendKeys(keys ...string) error {
	if m.sendKeysFunc != nil {
		return m.sendKeysFunc(keys...)
	}
	return nil
}

func (m mockTMUXPane) CapturePane() (string, error) {
	if m.capturePaneFunc != nil {
		return m.capturePaneFunc()
	}
	return "ready\n", nil
}
```

- [ ] **Step 3: Define mock session**

```go
type mockTMUXSession struct {
	killFunc func() error
}

func (m mockTMUXSession) Kill() error {
	if m.killFunc != nil {
		return m.killFunc()
	}
	return nil
}
```

- [ ] **Step 4: Define mock runtime factory**

```go
type mockTMUXRuntimeFactory struct {
	startFunc func(ctx context.Context, spec agent.Spec, sessionName string) (tmuxSession, tmuxPane, error)
}

func (m mockTMUXRuntimeFactory) Start(ctx context.Context, spec agent.Spec, sessionName string) (tmuxSession, tmuxPane, error) {
	if m.startFunc != nil {
		return m.startFunc(ctx, spec, sessionName)
	}
	return mockTMUXSession{}, mockTMUXPane{}, nil
}
```

- [ ] **Step 5: Define mock run store**

```go
type mockTMUXRunStore struct {
	createPendingFunc func(ctx context.Context, runID, botName, runtimeType string) error
	upsertDoneFunc    func(ctx context.Context, runID, botName, runtimeType string) error
	getByRunIDFunc    func(ctx context.Context, runID string) (tmuxRunRecord, error)
}

func (m mockTMUXRunStore) CreatePending(ctx context.Context, runID, botName, runtimeType string) error {
	if m.createPendingFunc != nil {
		return m.createPendingFunc(ctx, runID, botName, runtimeType)
	}
	return nil
}

func (m mockTMUXRunStore) UpsertDone(ctx context.Context, runID, botName, runtimeType string) error {
	if m.upsertDoneFunc != nil {
		return m.upsertDoneFunc(ctx, runID, botName, runtimeType)
	}
	return nil
}

func (m mockTMUXRunStore) GetByRunID(ctx context.Context, runID string) (tmuxRunRecord, error) {
	if m.getByRunIDFunc != nil {
		return m.getByRunIDFunc(ctx, runID)
	}
	return tmuxRunRecord{RunID: runID, Status: "done"}, nil
}
```

- [ ] **Step 6: Define mock run store factory**

```go
type mockTMUXRunStoreFactory struct {
	openFunc func(spec agent.Spec) (tmuxRunStore, error)
}

func (m mockTMUXRunStoreFactory) Open(spec agent.Spec) (tmuxRunStore, error) {
	if m.openFunc != nil {
		return m.openFunc(spec)
	}
	return mockTMUXRunStore{}, nil
}
```

- [ ] **Step 7: Verify compilation**

Run: `go test -c ./internal/agent/claude`
Expected: Success (test file compiles)

- [ ] **Step 8: Commit**

```bash
git add internal/agent/claude/driver_tmux_test.go
git commit -m "test: add claude tmux driver mock infrastructure"
```

---

### Task 13: Unit Tests - Init Tests

**Files:**
- Modify: `internal/agent/claude/driver_tmux_test.go`

- [ ] **Step 1: Write test for successful Init**

```go
func TestTMUXDriver_Init_Success(t *testing.T) {
	driver := &TMUXDriver{
		factory: mockTMUXRuntimeFactory{},
		runStoreFactory: mockTMUXRunStoreFactory{},
	}

	spec := agent.Spec{
		BotName:    "test-bot",
		Command:    "claude-code",
		WorkDir:    t.TempDir(),
		SQLitePath: ":memory:",
	}

	ctx := context.Background()
	runtime, err := driver.Init(ctx, spec)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if runtime == nil {
		t.Fatal("Init returned nil runtime")
	}

	defer runtime.Close()
}
```

- [ ] **Step 2: Write test for missing command**

```go
func TestTMUXDriver_Init_MissingCommand(t *testing.T) {
	driver := NewTMUXDriver()

	spec := agent.Spec{
		BotName: "test-bot",
		WorkDir: t.TempDir(),
	}

	ctx := context.Background()
	_, err := driver.Init(ctx, spec)
	if err == nil {
		t.Fatal("Init should fail with missing command")
	}
	if !strings.Contains(err.Error(), "requires command") {
		t.Errorf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 3: Write test for missing workdir**

```go
func TestTMUXDriver_Init_MissingWorkDir(t *testing.T) {
	driver := NewTMUXDriver()

	spec := agent.Spec{
		BotName: "test-bot",
		Command: "claude-code",
	}

	ctx := context.Background()
	_, err := driver.Init(ctx, spec)
	if err == nil {
		t.Fatal("Init should fail with missing workdir")
	}
	if !strings.Contains(err.Error(), "requires workdir") {
		t.Errorf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/agent/claude -run TestTMUXDriver_Init -v`
Expected: All 3 tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/agent/claude/driver_tmux_test.go
git commit -m "test: add claude tmux driver Init tests"
```

---

### Task 14: Unit Tests - Run Tests

**Files:**
- Modify: `internal/agent/claude/driver_tmux_test.go`

- [ ] **Step 1: Write test for successful Run**

```go
func TestTMUXRuntime_Run_Success(t *testing.T) {
	pane := mockTMUXPane{
		capturePaneFunc: func() (string, error) {
			return "test output\n", nil
		},
	}
	
	runtime := &TMUXRuntime{
		state:   stateReady,
		pane:    pane,
		session: mockTMUXSession{},
		waitGap: 1 * time.Millisecond,
		spec: agent.Spec{
			BotName: "test-bot",
			WorkDir: t.TempDir(),
		},
		runStore: mockTMUXRunStore{},
	}

	req := agent.Request{
		Prompt: "test prompt",
	}

	ctx := context.Background()
	resp, err := runtime.Run(ctx, req)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if resp.RuntimeType != runtimeTypeClaude {
		t.Errorf("expected runtime type %q, got %q", runtimeTypeClaude, resp.RuntimeType)
	}
	if resp.Text == "" {
		t.Error("expected non-empty response text")
	}
}
```

- [ ] **Step 2: Write test for empty prompt**

```go
func TestTMUXRuntime_Run_EmptyPrompt(t *testing.T) {
	runtime := &TMUXRuntime{
		state: stateReady,
	}

	req := agent.Request{
		Prompt: "",
	}

	ctx := context.Background()
	_, err := runtime.Run(ctx, req)
	if err == nil {
		t.Fatal("Run should fail with empty prompt")
	}
	if !strings.Contains(err.Error(), "prompt is required") {
		t.Errorf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 3: Write test for broken state**

```go
func TestTMUXRuntime_Run_BrokenState(t *testing.T) {
	runtime := &TMUXRuntime{
		state:   stateBroken,
		readErr: errors.New("test error"),
	}

	req := agent.Request{
		Prompt: "test",
	}

	ctx := context.Background()
	_, err := runtime.Run(ctx, req)
	if err == nil {
		t.Fatal("Run should fail with broken state")
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/agent/claude -run TestTMUXRuntime_Run -v`
Expected: All 3 tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/agent/claude/driver_tmux_test.go
git commit -m "test: add claude tmux runtime Run tests"
```

---

### Task 15: Unit Tests - Close and Utility Tests

**Files:**
- Modify: `internal/agent/claude/driver_tmux_test.go`

- [ ] **Step 1: Write test for Close**

```go
func TestTMUXRuntime_Close(t *testing.T) {
	killed := false
	session := mockTMUXSession{
		killFunc: func() error {
			killed = true
			return nil
		},
	}

	runtime := &TMUXRuntime{
		state:   stateReady,
		session: session,
		pane:    mockTMUXPane{},
	}

	err := runtime.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !killed {
		t.Error("session.Kill was not called")
	}
	if runtime.state != stateBroken {
		t.Errorf("expected state %q, got %q", stateBroken, runtime.state)
	}
}
```

- [ ] **Step 2: Write test for session naming**

```go
func TestNextTMUXSessionName(t *testing.T) {
	tests := []struct {
		botName string
		want    string
	}{
		{"test-bot", "myclaw-claude-test-bot"},
		{"Test Bot", "myclaw-claude-test-bot"},
		{"", "myclaw-claude-claude"},
		{"  ", "myclaw-claude-claude"},
	}

	for _, tt := range tests {
		got := nextTMUXSessionName(tt.botName)
		if got != tt.want {
			t.Errorf("nextTMUXSessionName(%q) = %q, want %q", tt.botName, got, tt.want)
		}
	}
}
```

- [ ] **Step 3: Write test for text cleanup**

```go
func TestCleanupTMUXRunText(t *testing.T) {
	input := "line1\r\n\nline2\r\n  \nline3\r"
	want := "line1\n\nline2\n\nline3"
	got := cleanupTMUXRunText(input)
	if got != want {
		t.Errorf("cleanupTMUXRunText() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 4: Write test for shell quote**

```go
func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "''"},
		{"simple", "'simple'"},
		{"with'quote", `'with'\''quote'`},
	}

	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
```

- [ ] **Step 5: Run all tests**

Run: `go test ./internal/agent/claude -v`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/agent/claude/driver_tmux_test.go
git commit -m "test: add claude tmux Close and utility tests"
```

---

### Task 16: Final Verification

**Files:**
- All files in `internal/agent/claude/`

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`
Expected: All tests pass

- [ ] **Step 2: Build the server**

Run: `go build ./cmd/server`
Expected: Success

- [ ] **Step 3: Verify driver registration**

Run: `go run ./cmd/server --help` (or check logs on startup)
Expected: Server starts without errors

- [ ] **Step 4: Run go vet**

Run: `go vet ./internal/agent/claude`
Expected: No issues

- [ ] **Step 5: Run go fmt**

Run: `go fmt ./internal/agent/claude`
Expected: No changes needed

- [ ] **Step 6: Final commit**

```bash
git add -A
git commit -m "feat: complete claude tmux driver implementation

- Implements agent.Driver interface for claude-tmux
- Manages tmux sessions via gotmux library
- Tracks run state via SQLite
- Full test coverage with mocks
- Follows codex tmux driver patterns"
```

---

## Spec Coverage Review

**Spec Requirements → Implementation Mapping:**

1. ✅ Core Components (TMUXDriver, TMUXRuntime) → Task 1
2. ✅ Driver registration as "claude-tmux" → Task 2
3. ✅ Init flow with validation → Task 3
4. ✅ Runtime state management → Task 1, 4
5. ✅ Run flow with prompt execution → Task 5
6. ✅ Close flow with session cleanup → Task 6
7. ✅ TMUX session management → Task 8, 9, 10
8. ✅ SQLite run store → Task 11
9. ✅ Utility functions (naming, quoting, cleanup) → Task 7
10. ✅ Unit tests with mocks → Tasks 12-15
11. ✅ Command: claude-code → Task 8
12. ✅ Notify: myclaw notify claude → Task 8
13. ✅ Runtime type: "claude" → Task 1

All spec requirements covered.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-21-claude-tmux-driver.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?

