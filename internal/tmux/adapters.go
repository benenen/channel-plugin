package tmux

import gotmux "github.com/jubnzv/go-tmux"

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

