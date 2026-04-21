package tmux

import (
	"fmt"

	"github.com/GianlucaP106/gotmux/gotmux"
)

// Pane represents a tmux pane interface for sending keys and capturing output.
type Pane interface {
	SendKeys(keys ...string) error
	CapturePane() (string, error)
}

// Session represents a tmux session interface for lifecycle management.
type Session interface {
	Kill() error
}

// GotmuxSession wraps a gotmux Session to implement the Session interface.
type GotmuxSession struct {
	session *gotmux.Session
}

// GotmuxPane wraps a gotmux Pane to implement the Pane interface.
type GotmuxPane struct {
	pane *gotmux.Pane
}

// GotmuxFactory creates gotmux-backed Session and Pane instances.
type GotmuxFactory struct{}

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

