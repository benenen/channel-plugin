# Extract Common TMUX Code Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract duplicate gotmux adapter and utility code from codex and claude drivers into a shared internal/tmux package.

**Architecture:** Create internal/tmux/adapters.go with gotmux adapters (GotmuxSession, GotmuxPane, GotmuxFactory) and utility functions (NormalizeTMUXOutput, CleanupTMUXRunText, ShellQuote). Update both drivers to import and use the shared code.

**Tech Stack:** Go 1.23, gotmux library

---

## File Structure

**New Files:**
- `internal/tmux/adapters.go` - Shared gotmux adapters and utility functions

**Modified Files:**
- `internal/agent/codex/driver_tmux.go` - Remove duplicated code, import tmux package
- `internal/agent/claude/driver_tmux.go` - Remove duplicated code, import tmux package

**Reference Files:**
- `internal/agent/codex/driver_tmux.go` (lines 73-79, 346-430, 316-331, 489-494) - Code to extract
- `internal/agent/claude/driver_tmux.go` (lines 86-92, 358-442, 328-343, 501-506) - Code to extract

---

### Task 1: Create internal/tmux Package with Interfaces

**Files:**
- Create: `internal/tmux/adapters.go`

- [ ] **Step 1: Create package with imports**

```go
package tmux

import (
	"context"
	"fmt"
	"strings"

	"github.com/GianlucaP106/gotmux/gotmux"
	"github.com/benenen/myclaw/internal/agent"
)
```

- [ ] **Step 2: Define Pane interface**

```go
// Pane represents a tmux pane interface.
type Pane interface {
	SendKeys(keys ...string) error
	CapturePane() (string, error)
}
```

- [ ] **Step 3: Define Session interface**

```go
// Session represents a tmux session interface.
type Session interface {
	Kill() error
}
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./internal/tmux`
Expected: Success

- [ ] **Step 5: Commit**

```bash
git add internal/tmux/adapters.go
git commit -m "feat: create internal/tmux package with interfaces"
```

---

### Task 2: Add Gotmux Adapter Structs

**Files:**
- Modify: `internal/tmux/adapters.go`

- [ ] **Step 1: Add GotmuxSession struct**

```go
// GotmuxSession wraps a gotmux.Session and implements the Session interface.
type GotmuxSession struct {
	session *gotmux.Session
}
```

- [ ] **Step 2: Add GotmuxPane struct**

```go
// GotmuxPane wraps a gotmux.Pane and implements the Pane interface.
type GotmuxPane struct {
	pane *gotmux.Pane
}
```

- [ ] **Step 3: Add GotmuxFactory struct**

```go
// GotmuxFactory creates gotmux sessions and panes.
type GotmuxFactory struct{}
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./internal/tmux`
Expected: Success

- [ ] **Step 5: Commit**

```bash
git add internal/tmux/adapters.go
git commit -m "feat: add gotmux adapter structs"
```

---

### Task 3: Implement GotmuxSession Methods

**Files:**
- Modify: `internal/tmux/adapters.go`

- [ ] **Step 1: Implement Kill method**

```go
// Kill terminates the tmux session.
func (s GotmuxSession) Kill() error {
	if s.session == nil {
		return nil
	}
	if err := s.session.Kill(); err != nil {
		return fmt.Errorf("kill tmux session %q: %w", s.session.Name, err)
	}
	return nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/tmux`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/tmux/adapters.go
git commit -m "feat: implement GotmuxSession.Kill method"
```

---

### Task 4: Implement GotmuxPane Methods

**Files:**
- Modify: `internal/tmux/adapters.go`

- [ ] **Step 1: Implement SendKeys method**

```go
// SendKeys sends keys to the tmux pane.
func (p GotmuxPane) SendKeys(keys ...string) error {
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

- [ ] **Step 2: Implement CapturePane method**

```go
// CapturePane captures the content of the tmux pane.
func (p GotmuxPane) CapturePane() (string, error) {
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

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/tmux`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add internal/tmux/adapters.go
git commit -m "feat: implement GotmuxPane methods"
```

---

### Task 5: Implement GotmuxFactory.Start Method

**Files:**
- Modify: `internal/tmux/adapters.go`

- [ ] **Step 1: Implement Start method (part 1)**

```go
// Start creates or connects to a tmux session and returns the session and pane.
func (GotmuxFactory) Start(ctx context.Context, spec agent.Spec, sessionName string) (Session, Pane, error) {
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if len(spec.Args) > 0 {
		return nil, nil, fmt.Errorf("tmux driver does not support tmux startup args yet")
	}
	if len(spec.Env) > 0 {
		return nil, nil, fmt.Errorf("tmux driver does not support tmux startup env yet")
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
		options := buildSessionOptions(spec, sessionName)
		session, err = tmux.NewSession(options)
		if err != nil {
			return nil, nil, fmt.Errorf("start tmux session %q: %w", sessionName, err)
		}
	}
```

- [ ] **Step 2: Implement Start method (part 2)**

```go
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
	pane := GotmuxPane{pane: panes[0]}
	return GotmuxSession{session: session}, pane, nil
}
```

- [ ] **Step 3: Add buildSessionOptions helper**

```go
func buildSessionOptions(spec agent.Spec, sessionName string) *gotmux.SessionOptions {
	options := &gotmux.SessionOptions{
		Name: sessionName,
	}
	if strings.TrimSpace(spec.WorkDir) != "" {
		options.StartDirectory = spec.WorkDir
	}
	if strings.TrimSpace(spec.Command) != "" {
		options.ShellCommand = spec.Command
	}
	return options
}
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./internal/tmux`
Expected: Success

- [ ] **Step 5: Commit**

```bash
git add internal/tmux/adapters.go
git commit -m "feat: implement GotmuxFactory.Start method"
```

---

### Task 6: Add Utility Functions

**Files:**
- Modify: `internal/tmux/adapters.go`

- [ ] **Step 1: Add NormalizeTMUXOutput function**

```go
// NormalizeTMUXOutput normalizes tmux output by replacing \r\n with \n.
func NormalizeTMUXOutput(text string) string {
	return strings.ReplaceAll(text, "\r\n", "\n")
}
```

- [ ] **Step 2: Add CleanupTMUXRunText function**

```go
// CleanupTMUXRunText cleans up tmux run text by removing empty lines and trailing \r.
func CleanupTMUXRunText(text string) string {
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
```

- [ ] **Step 3: Add ShellQuote function**

```go
// ShellQuote quotes a string for safe use in shell commands.
func ShellQuote(text string) string {
	if text == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./internal/tmux`
Expected: Success

- [ ] **Step 5: Commit**

```bash
git add internal/tmux/adapters.go
git commit -m "feat: add tmux utility functions"
```

---

### Task 7: Update Codex Driver - Add Import and Update Factory

**Files:**
- Modify: `internal/agent/codex/driver_tmux.go`

- [ ] **Step 1: Add tmux import**

Add after line 18:
```go
"github.com/benenen/myclaw/internal/tmux"
```

- [ ] **Step 2: Update NewTMUXDriver to use tmux.GotmuxFactory**

Replace line 89:
```go
factory:         tmux.GotmuxFactory{},
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/agent/codex`
Expected: Fails with "tmuxGotmuxFactory undefined" (expected, will fix in next task)

- [ ] **Step 4: Commit**

```bash
git add internal/agent/codex/driver_tmux.go
git commit -m "refactor: add tmux import to codex driver"
```

---

### Task 8: Update Codex Driver - Remove Adapter Structs and Update References

**Files:**
- Modify: `internal/agent/codex/driver_tmux.go`

- [ ] **Step 1: Delete tmuxGotmuxFactory struct**

Delete line 69:
```go
type tmuxGotmuxFactory struct{}
```

- [ ] **Step 2: Delete tmuxGotmuxSession struct**

Delete lines 73-75:
```go
type tmuxGotmuxSession struct {
	session *gotmux.Session
}
```

- [ ] **Step 3: Delete tmuxGotmuxPane struct**

Delete lines 77-79:
```go
type tmuxGotmuxPane struct {
	pane *gotmux.Pane
}
```

- [ ] **Step 4: Delete tmuxGotmuxFactory.Start method**

Delete lines 346-394 (entire Start method)

- [ ] **Step 5: Delete tmuxGotmuxSession.Kill method**

Delete lines 396-404 (entire Kill method)

- [ ] **Step 6: Delete tmuxGotmuxPane.SendKeys method**

Delete lines 406-419 (entire SendKeys method)

- [ ] **Step 7: Delete tmuxGotmuxPane.CapturePane method**

Delete lines 421-430 (entire CapturePane method)

- [ ] **Step 8: Verify compilation**

Run: `go build ./internal/agent/codex`
Expected: Success

- [ ] **Step 9: Commit**

```bash
git add internal/agent/codex/driver_tmux.go
git commit -m "refactor: remove gotmux adapters from codex driver"
```

---

### Task 9: Update Codex Driver - Replace Utility Functions

**Files:**
- Modify: `internal/agent/codex/driver_tmux.go`

- [ ] **Step 1: Update normalizeTMUXOutput call in waitUntilReady**

Replace line ~236:
```go
normalized := tmux.NormalizeTMUXOutput(captured)
```

- [ ] **Step 2: Update cleanupTMUXRunText call in Run**

Replace line ~205:
```go
text := tmux.CleanupTMUXRunText(tmux.NormalizeTMUXOutput(captured))
```

- [ ] **Step 3: Update shellQuote call in buildTMUXShellCommand**

Replace line ~486:
```go
return command + " -c " + tmux.ShellQuote(notifyConfig)
```

- [ ] **Step 4: Delete normalizeTMUXOutput function**

Delete lines 329-331:
```go
func normalizeTMUXOutput(text string) string {
	return strings.ReplaceAll(text, "\r\n", "\n")
}
```

- [ ] **Step 5: Delete cleanupTMUXRunText function**

Delete lines 316-327:
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
```

- [ ] **Step 6: Delete shellQuote function**

Delete lines 489-494:
```go
func shellQuote(text string) string {
	if text == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}
```

- [ ] **Step 7: Verify compilation**

Run: `go build ./internal/agent/codex`
Expected: Success

- [ ] **Step 8: Run codex tests**

Run: `go test ./internal/agent/codex -v`
Expected: All tests pass

- [ ] **Step 9: Commit**

```bash
git add internal/agent/codex/driver_tmux.go
git commit -m "refactor: use tmux utility functions in codex driver"
```

---

### Task 10: Update Claude Driver - Add Import and Update Factory

**Files:**
- Modify: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Add tmux import**

Add after line 18:
```go
"github.com/benenen/myclaw/internal/tmux"
```

- [ ] **Step 2: Update NewTMUXDriver to use tmux.GotmuxFactory**

Replace line 97:
```go
factory:         tmux.GotmuxFactory{},
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Fails with "tmuxGotmuxFactory undefined" (expected, will fix in next task)

- [ ] **Step 4: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "refactor: add tmux import to claude driver"
```

---

### Task 11: Update Claude Driver - Remove Adapter Structs

**Files:**
- Modify: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Delete tmuxGotmuxFactory struct**

Delete line 82:
```go
type tmuxGotmuxFactory struct{}
```

- [ ] **Step 2: Delete tmuxGotmuxSession struct**

Delete lines 86-88:
```go
type tmuxGotmuxSession struct {
	session *gotmux.Session
}
```

- [ ] **Step 3: Delete tmuxGotmuxPane struct**

Delete lines 90-92:
```go
type tmuxGotmuxPane struct {
	pane *gotmux.Pane
}
```

- [ ] **Step 4: Delete tmuxGotmuxFactory.Start method**

Delete lines 358-406 (entire Start method)

- [ ] **Step 5: Delete tmuxGotmuxSession.Kill method**

Delete lines 408-416 (entire Kill method)

- [ ] **Step 6: Delete tmuxGotmuxPane.SendKeys method**

Delete lines 418-431 (entire SendKeys method)

- [ ] **Step 7: Delete tmuxGotmuxPane.CapturePane method**

Delete lines 433-442 (entire CapturePane method)

- [ ] **Step 8: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Success

- [ ] **Step 9: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "refactor: remove gotmux adapters from claude driver"
```

---

### Task 12: Update Claude Driver - Replace Utility Functions

**Files:**
- Modify: `internal/agent/claude/driver_tmux.go`

- [ ] **Step 1: Update normalizeTMUXOutput call in waitUntilReady**

Replace line ~248:
```go
normalized := tmux.NormalizeTMUXOutput(captured)
```

- [ ] **Step 2: Update cleanupTMUXRunText call in Run**

Replace line ~217:
```go
text := tmux.CleanupTMUXRunText(tmux.NormalizeTMUXOutput(captured))
```

- [ ] **Step 3: Update shellQuote call in buildTMUXShellCommand**

Replace line ~498:
```go
return command + " -c " + tmux.ShellQuote(notifyConfig)
```

- [ ] **Step 4: Delete normalizeTMUXOutput function**

Delete lines 341-343:
```go
func normalizeTMUXOutput(text string) string {
	return strings.ReplaceAll(text, "\r\n", "\n")
}
```

- [ ] **Step 5: Delete cleanupTMUXRunText function**

Delete lines 328-339:
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
```

- [ ] **Step 6: Delete shellQuote function**

Delete lines 501-506:
```go
func shellQuote(text string) string {
	if text == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}
```

- [ ] **Step 7: Verify compilation**

Run: `go build ./internal/agent/claude`
Expected: Success

- [ ] **Step 8: Run claude tests**

Run: `go test ./internal/agent/claude -v`
Expected: All tests pass

- [ ] **Step 9: Commit**

```bash
git add internal/agent/claude/driver_tmux.go
git commit -m "refactor: use tmux utility functions in claude driver"
```

---

### Task 13: Final Verification

**Files:**
- All modified files

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`
Expected: All tests pass

- [ ] **Step 2: Build server**

Run: `go build ./cmd/server`
Expected: Success

- [ ] **Step 3: Verify no duplicate code**

Run: `grep -n "func.*SendKeys" internal/agent/codex/driver_tmux.go internal/agent/claude/driver_tmux.go`
Expected: No matches (adapters removed)

Run: `grep -n "func.*SendKeys" internal/tmux/adapters.go`
Expected: One match (in shared package)

- [ ] **Step 4: Check code reduction**

Run: `wc -l internal/agent/codex/driver_tmux.go internal/agent/claude/driver_tmux.go internal/tmux/adapters.go`
Expected: codex and claude files reduced by ~110 lines each, tmux file ~120 lines

- [ ] **Step 5: Run go fmt**

Run: `go fmt ./internal/tmux ./internal/agent/codex ./internal/agent/claude`
Expected: No changes needed

- [ ] **Step 6: Run go vet**

Run: `go vet ./internal/tmux ./internal/agent/codex ./internal/agent/claude`
Expected: No issues

- [ ] **Step 7: Final commit**

```bash
git add -A
git commit -m "refactor: extract common tmux code to internal/tmux

- Created internal/tmux package with gotmux adapters and utilities
- Removed duplicate code from codex and claude drivers
- Net reduction: ~100 lines of duplicate code
- All tests passing, no behavior changes"
```

---

## Spec Coverage Review

**Spec Requirements → Implementation Mapping:**

1. ✅ Create internal/tmux/adapters.go → Tasks 1-6
2. ✅ Interface definitions (Pane, Session) → Task 1
3. ✅ Gotmux adapters (GotmuxSession, GotmuxPane, GotmuxFactory) → Tasks 2-5
4. ✅ Utility functions (NormalizeTMUXOutput, CleanupTMUXRunText, ShellQuote) → Task 6
5. ✅ Update codex driver → Tasks 7-9
6. ✅ Update claude driver → Tasks 10-12
7. ✅ Remove duplicate code → Tasks 8-9, 11-12
8. ✅ Verify tests pass → Task 13
9. ✅ Verify compilation → Task 13

All spec requirements covered.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-21-extract-tmux-common-code.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
