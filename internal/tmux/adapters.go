package tmux

// Pane represents a tmux pane interface for sending keys and capturing output.
type Pane interface {
	SendKeys(keys ...string) error
	CapturePane() (string, error)
}

// Session represents a tmux session interface for lifecycle management.
type Session interface {
	Kill() error
}
