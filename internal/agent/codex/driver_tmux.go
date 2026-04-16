package codex

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GianlucaP106/gotmux/gotmux"
	"github.com/benenen/myclaw/internal/agent"
)

type TMUXDriver struct {
	factory tmuxRuntimeFactory
}

type TMUXRuntime struct {
	mu    sync.Mutex
	runMu sync.Mutex

	state   runtimeState
	pane    tmuxPane
	session tmuxSession
	readErr error
	waitGap time.Duration
}

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

type tmuxGotmuxFactory struct{}

type tmuxGotmuxSession struct {
	session *gotmux.Session
}

type tmuxGotmuxPane struct {
	pane *gotmux.Pane
}

func init() {
	agent.MustRegisterDriver("codex-tmux", func() agent.Driver {
		return NewTMUXDriver()
	})
}

func NewTMUXDriver() *TMUXDriver {
	return &TMUXDriver{factory: tmuxGotmuxFactory{}}
}

func (d *TMUXDriver) Init(ctx context.Context, spec agent.Spec) (agent.SessionRuntime, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return nil, fmt.Errorf("codex tmux driver requires command")
	}

	runtime := &TMUXRuntime{
		state:   stateStarting,
		waitGap: 10 * time.Millisecond,
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
	if r.state != stateReady && r.state != stateStarting {
		state := r.state
		r.mu.Unlock()
		return agent.Response{}, fmt.Errorf("codex tmux runtime is not ready: %s", state)
	}
	pane := r.pane
	r.state = stateRunning
	r.mu.Unlock()

	if pane == nil {
		r.markBroken(fmt.Errorf("codex tmux runtime is not connected to a pane"))
		return agent.Response{}, r.currentError()
	}

	runCtx := ctx
	cancel := func() {}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		runCtx, cancel = context.WithTimeout(ctx, defaultRunTimeout)
	}
	defer cancel()

	if err := pane.SendKeys(promptText, "C-m"); err != nil {
		r.markBroken(fmt.Errorf("codex tmux send failed: %w", err))
		return agent.Response{}, r.currentError()
	}

	text, err := r.waitRunCompletion(runCtx)
	if err != nil {
		r.markBroken(err)
		return agent.Response{}, err
	}

	r.mu.Lock()
	if r.state != stateBroken {
		r.state = stateReady
	}
	r.mu.Unlock()

	return agent.Response{Text: text, ExitCode: 0, RawOutput: text}, nil
}

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
			return fmt.Errorf("codex tmux capture failed: %w", err)
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
			return fmt.Errorf("codex tmux startup timed out: %w", ctx.Err())
		}
		time.Sleep(gap)
	}
}

func (r *TMUXRuntime) waitRunCompletion(ctx context.Context) (string, error) {
	r.mu.Lock()
	pane := r.pane
	gap := r.waitGap
	r.mu.Unlock()
	if gap <= 0 {
		gap = 10 * time.Millisecond
	}

	for {
		captured, err := pane.CapturePane()
		if err != nil {
			return "", fmt.Errorf("codex tmux capture failed: %w", err)
		}
		if text, err := extractTMUXRunResult(captured); err == nil {
			return text, nil
		}
		if ctx.Err() != nil {
			return "", fmt.Errorf("codex tmux run timed out: %w", ctx.Err())
		}
		time.Sleep(gap)
	}
}

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
		r.readErr = fmt.Errorf("codex tmux runtime is closed")
	}
	r.mu.Unlock()

	if session == nil {
		return nil
	}
	return session.Kill()
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

func (r *TMUXRuntime) currentError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readErr != nil {
		return r.readErr
	}
	return fmt.Errorf("codex tmux runtime is broken")
}

func extractTMUXRunResult(text string) (string, error) {
	normalized := normalizeTMUXOutput(text)
	return strings.TrimSpace(normalized), nil
}

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

func (tmuxGotmuxFactory) Start(ctx context.Context, spec agent.Spec, sessionName string) (tmuxSession, tmuxPane, error) {
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	if len(spec.Args) > 0 {
		return nil, nil, fmt.Errorf("codex tmux driver does not support tmux startup args yet")
	}
	if len(spec.Env) > 0 {
		return nil, nil, fmt.Errorf("codex tmux driver does not support tmux startup env yet")
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
		options := &gotmux.SessionOptions{Name: sessionName, ShellCommand: spec.Command}
		if strings.TrimSpace(spec.WorkDir) != "" {
			options.StartDirectory = spec.WorkDir
		}

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

func (s tmuxGotmuxSession) Kill() error {
	if s.session == nil {
		return nil
	}
	if err := s.session.Kill(); err != nil {
		return fmt.Errorf("kill tmux session %q: %w", s.session.Name, err)
	}
	return nil
}

func (p tmuxGotmuxPane) SendKeys(keys ...string) error {
	if p.pane == nil {
		return fmt.Errorf("tmux pane is nil")
	}
	for _, key := range keys {
		// if key == "C-m" {
		// 	key = "Enter"
		// }
		if err := p.pane.SendKeys(key); err != nil {
			return fmt.Errorf("send tmux keys: %w", err)
		}
	}
	return nil
}

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

func nextTMUXSessionName(botName string) string {
	prefix := strings.TrimSpace(botName)
	prefix = strings.ToLower(prefix)
	prefix = strings.ReplaceAll(prefix, " ", "-")
	if prefix == "" {
		prefix = "codex"
	}
	return fmt.Sprintf("myclaw-codex-%s", prefix)
}
