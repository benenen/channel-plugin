package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ---- I/O types (jsonschema-tagged for the MCP SDK) ----

type LsInput struct{}
type Session struct {
	Name     string `json:"name"`
	Attached bool   `json:"attached"`
	IdleMs   int64  `json:"idle_ms"`
	Title    string `json:"title"`
}
type LsOutput struct {
	Sessions []Session `json:"sessions" jsonschema:"the live asd sessions"`
}

type NewInput struct {
	Name    string   `json:"name,omitempty" jsonschema:"session name, [A-Za-z0-9_-]{1,64}"`
	Command []string `json:"command,omitempty" jsonschema:"command + args to run; default the user's shell"`
	Rows    int      `json:"rows,omitempty" jsonschema:"unsupported by asd (a session sizes to its attaching client); setting it is an error"`
	Cols    int      `json:"cols,omitempty" jsonschema:"unsupported by asd (a session sizes to its attaching client); setting it is an error"`
	Cwd     string   `json:"cwd,omitempty" jsonschema:"working directory; must already exist"`
}
type NewOutput struct {
	Name string `json:"name" jsonschema:"the created session name"`
}

type SendInput struct {
	Name  string `json:"name"`
	Text  string `json:"text,omitempty" jsonschema:"literal text to type (no implicit newline)"`
	Enter bool   `json:"enter,omitempty" jsonschema:"append Enter after the text"`
	Keys  string `json:"keys,omitempty" jsonschema:"comma-separated named keys e.g. Enter,C-c,Up (mutually exclusive with text)"`
}
type SendOutput struct {
	Ok bool `json:"ok"`
}

type PeekInput struct {
	Name       string `json:"name"`
	Scrollback bool   `json:"scrollback,omitempty" jsonschema:"include full scrollback history"`
}
type Cursor struct {
	Row int `json:"row"`
	Col int `json:"col"`
}
type PeekOutput struct {
	Session string `json:"session"`
	Title   string `json:"title"`
	Rows    int    `json:"rows"`
	Cols    int    `json:"cols"`
	Cursor  Cursor `json:"cursor"`
	Screen  string `json:"screen"`
}

type WaitInput struct {
	Name    string `json:"name"`
	Mode    string `json:"mode" jsonschema:"one of: text, idle"`
	Text    string `json:"text,omitempty" jsonschema:"substring to wait for (mode=text)"`
	Timeout string `json:"timeout,omitempty" jsonschema:"duration like 2s, 1m (default 30s)"`
}
type WaitOutput struct {
	Matched bool `json:"matched" jsonschema:"true if the condition was met, false on timeout"`
}

type KillInput struct {
	Name string `json:"name,omitempty"`
	All  bool   `json:"all,omitempty" jsonschema:"kill every session"`
}
type KillOutput struct {
	Ok bool `json:"ok"`
}

// ---- argv builders (pure; validation lives here) ----

// `asd list` renders a table, not JSON, and carries no per-session detail, so
// the roster is assembled as list (names) + inspect --json (one per session).
func argsForLs(LsInput) ([]string, error) { return []string{"list"}, nil }

func argsForInspect(name string) ([]string, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	return []string{"inspect", name, "--json"}, nil
}

// sizeColumn matches the SIZE column ("181x55"), which every session row has
// and neither the header nor the "no sessions" line does.
var sizeColumn = regexp.MustCompile(`^\d+x\d+$`)

// parseSessionNames pulls the NAME column out of `asd list` output.
func parseSessionNames(stdout []byte) []string {
	names := []string{}
	for _, line := range strings.Split(string(stdout), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !sizeColumn.MatchString(fields[1]) {
			continue
		}
		names = append(names, fields[0])
	}
	return names
}

// shellSafe is the set of characters `sh` treats literally, so an argument made
// only of these needs no quoting.
var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuote renders s as a single `sh` word.
func shellQuote(s string) string {
	if s != "" && shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// newScript builds the `--cmd` script: asd has no --cwd, so a requested working
// directory becomes a `cd` in front of the command (exec keeps the session's pid
// on the real process, which is what `inspect` reports).
func newScript(command []string, cwd string) string {
	words := make([]string, 0, len(command))
	for _, c := range command {
		words = append(words, shellQuote(c))
	}
	cmd := strings.Join(words, " ")
	if cwd == "" {
		return cmd
	}
	if cmd == "" {
		cmd = `exec "$SHELL"`
	} else {
		cmd = "exec " + cmd
	}
	return "cd " + shellQuote(cwd) + " && " + cmd
}

func argsForNew(in NewInput) ([]string, error) {
	// asd sizes a session from its attaching client; there is no CLI knob.
	if in.Rows > 0 || in.Cols > 0 {
		return nil, fmt.Errorf("asd cannot set rows/cols on a new session (it sizes to the attaching client)")
	}
	args := []string{"new"}
	if in.Name != "" {
		args = append(args, in.Name)
	}
	// asd sessions are always detached; no -d flag.
	if script := newScript(in.Command, in.Cwd); script != "" {
		args = append(args, "--cmd", script)
	}
	return args, nil
}

func argsForSend(in SendInput) ([]string, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	hasText, hasKeys := in.Text != "", in.Keys != ""
	if hasText == hasKeys {
		return nil, fmt.Errorf("exactly one of text or keys is required")
	}
	args := []string{"send", in.Name}
	if hasText {
		args = append(args, "--text", in.Text)
		if in.Enter {
			args = append(args, "--enter")
		}
	} else {
		args = append(args, "--key", in.Keys)
	}
	return args, nil
}

func argsForPeek(in PeekInput) ([]string, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	args := []string{"peek", in.Name, "--json"}
	if in.Scrollback {
		args = append(args, "--scrollback")
	}
	return args, nil
}

func argsForWait(in WaitInput) ([]string, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	args := []string{"wait", in.Name}
	switch in.Mode {
	case "text":
		if in.Text == "" {
			return nil, fmt.Errorf("mode=text requires text")
		}
		args = append(args, "--text", in.Text)
	case "idle":
		args = append(args, "--idle")
	default:
		return nil, fmt.Errorf("mode must be text or idle")
	}
	if in.Timeout != "" {
		args = append(args, "--timeout", in.Timeout)
	}
	return args, nil
}

func validateKill(in KillInput) error {
	if (in.Name != "") == in.All {
		return fmt.Errorf("exactly one of name or all is required")
	}
	return nil
}

// asd has no `kill --all`; runKill fans out over the roster instead.
func argsForKillOne(name string) []string { return []string{"kill", name} }

// runAsd is the single exec seam; handlers call it, tests stub it.
var runAsd = func(ctx context.Context, args ...string) (stdout []byte, stderr []byte, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, "asd", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return out.Bytes(), errb.Bytes(), exitErr.ExitCode(), nil
	}
	if err != nil {
		return out.Bytes(), errb.Bytes(), -1, fmt.Errorf("asd not available: %w", err)
	}
	return out.Bytes(), errb.Bytes(), 0, nil
}

// asdError maps a non-success asd exit code to a tool error. Returns nil for 0.
// Exit 4 (wait timeout) is handled by runWait directly and not passed here.
// asd signals a missing session as exit 1 with "no such session" on stderr
// rather than a dedicated exit code, so the message is what we match on.
func asdError(name string, exitCode int, stderr []byte) error {
	if exitCode == 0 {
		return nil
	}
	msg := strings.TrimSpace(string(stderr))
	if isNoSuchSession(stderr) {
		return fmt.Errorf("no such session: %s", name)
	}
	if msg == "" {
		msg = fmt.Sprintf("asd exited %d", exitCode)
	}
	return fmt.Errorf("asd error: %s", msg)
}

// isNoSuchSession recognises both wordings asd uses: every subcommand says
// "no such session" except `wait`, which says "no session named".
func isNoSuchSession(stderr []byte) bool {
	s := string(stderr)
	return strings.Contains(s, "no such session") || strings.Contains(s, "no session named")
}

// inspectOutput is the subset of `asd inspect --json` the roster needs.
type inspectOutput struct {
	Session         string `json:"session"`
	Title           string `json:"title"`
	IdleMs          int64  `json:"idle_ms"`
	AttachedClients int    `json:"attached_clients"`
}

// listSessionNames returns the live session names from `asd list`.
func listSessionNames(ctx context.Context) ([]string, error) {
	args, err := argsForLs(LsInput{})
	if err != nil {
		return nil, err
	}
	out, errb, code, err := runAsd(ctx, args...)
	if err != nil {
		return nil, err
	}
	if e := asdError("", code, errb); e != nil {
		return nil, e
	}
	return parseSessionNames(out), nil
}

// runLs assembles the roster: names from `asd list`, per-session detail from
// `asd inspect --json`. A session that dies between the two calls is skipped
// rather than failing the whole listing.
func runLs(ctx context.Context, in LsInput) (LsOutput, error) {
	names, err := listSessionNames(ctx)
	if err != nil {
		return LsOutput{}, err
	}
	sessions := []Session{}
	for _, name := range names {
		args, err := argsForInspect(name)
		if err != nil {
			return LsOutput{}, err
		}
		out, errb, code, err := runAsd(ctx, args...)
		if err != nil {
			return LsOutput{}, err
		}
		if code != 0 {
			if isNoSuchSession(errb) {
				continue
			}
			return LsOutput{}, asdError(name, code, errb)
		}
		var d inspectOutput
		if err := json.Unmarshal(out, &d); err != nil {
			return LsOutput{}, fmt.Errorf("parse inspect --json for %s: %w", name, err)
		}
		sessions = append(sessions, Session{
			Name:     name,
			Attached: d.AttachedClients > 0,
			IdleMs:   d.IdleMs,
			Title:    d.Title,
		})
	}
	return LsOutput{Sessions: sessions}, nil
}

func runNew(ctx context.Context, in NewInput) (NewOutput, error) {
	args, err := argsForNew(in)
	if err != nil {
		return NewOutput{}, err
	}
	out, errb, code, err := runAsd(ctx, args...)
	if err != nil {
		return NewOutput{}, err
	}
	if e := asdError(in.Name, code, errb); e != nil {
		return NewOutput{}, e
	}
	return NewOutput{Name: strings.TrimSpace(string(out))}, nil
}

func runSend(ctx context.Context, in SendInput) (SendOutput, error) {
	args, err := argsForSend(in)
	if err != nil {
		return SendOutput{}, err
	}
	_, errb, code, err := runAsd(ctx, args...)
	if err != nil {
		return SendOutput{}, err
	}
	if e := asdError(in.Name, code, errb); e != nil {
		return SendOutput{}, e
	}
	return SendOutput{Ok: true}, nil
}

func runPeek(ctx context.Context, in PeekInput) (PeekOutput, error) {
	args, err := argsForPeek(in)
	if err != nil {
		return PeekOutput{}, err
	}
	out, errb, code, err := runAsd(ctx, args...)
	if err != nil {
		return PeekOutput{}, err
	}
	if e := asdError(in.Name, code, errb); e != nil {
		return PeekOutput{}, e
	}
	var po PeekOutput
	if err := json.Unmarshal(out, &po); err != nil {
		return PeekOutput{}, fmt.Errorf("parse peek --json: %w", err)
	}
	return po, nil
}

func runWait(ctx context.Context, in WaitInput) (WaitOutput, error) {
	args, err := argsForWait(in)
	if err != nil {
		return WaitOutput{}, err
	}
	_, errb, code, err := runAsd(ctx, args...)
	if err != nil {
		return WaitOutput{}, err
	}
	if code == 4 { // timeout: a normal result, not an error
		return WaitOutput{Matched: false}, nil
	}
	if e := asdError(in.Name, code, errb); e != nil {
		return WaitOutput{}, e
	}
	return WaitOutput{Matched: true}, nil
}

// runKill ends one session, or every session: asd has no `kill --all`, so the
// roster is listed and killed one by one. A session that exits on its own in
// between is not a failure.
func runKill(ctx context.Context, in KillInput) (KillOutput, error) {
	if err := validateKill(in); err != nil {
		return KillOutput{}, err
	}
	names := []string{in.Name}
	if in.All {
		var err error
		if names, err = listSessionNames(ctx); err != nil {
			return KillOutput{}, err
		}
	}
	for _, name := range names {
		_, errb, code, err := runAsd(ctx, argsForKillOne(name)...)
		if err != nil {
			return KillOutput{}, err
		}
		if code == 0 {
			continue
		}
		if in.All && isNoSuchSession(errb) {
			continue
		}
		return KillOutput{}, asdError(name, code, errb)
	}
	return KillOutput{Ok: true}, nil
}
