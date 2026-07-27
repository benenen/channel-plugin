package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	kindHTTP = "http"
	kindAsd  = "asd"
)

// Source is one config entry. kind defaults to "http".
type Source struct {
	Kind        string `json:"kind,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	AuthToken   string `json:"auth_token,omitempty"`
	WaitTimeout string `json:"wait_timeout,omitempty"` // asd source default for dispatched waits
	// Include/Exclude gate which live sessions an asd source exposes (shell
	// globs; a plain name is an exact match). Without them every session on the
	// box becomes a dispatch target, including the caller's own.
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

func (s Source) kind() string {
	if s.Kind == "" {
		return kindHTTP
	}
	return s.Kind
}

// allowsSession reports whether this source exposes the named session: an
// exclude match always wins, and an empty include list means "every name".
func (s Source) allowsSession(name string) bool {
	for _, pat := range s.Exclude {
		if matchGlob(pat, name) {
			return false
		}
	}
	if len(s.Include) == 0 {
		return true
	}
	for _, pat := range s.Include {
		if matchGlob(pat, name) {
			return true
		}
	}
	return false
}

// matchGlob is path.Match with an unparsable pattern treated as "no match";
// configWarnings reports those patterns at startup so they aren't silent.
func matchGlob(pattern, name string) bool {
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}

// visibleSession reports whether the config exposes this session at all. Each
// asd source contributes its own filtered view, so a session allowed by any of
// them is visible. A config with no asd source has nothing to filter on, and
// the roster stays informational (dispatch refuses it either way).
func visibleSession(sources []Source, name string) bool {
	filtered := false
	for _, s := range sources {
		if s.kind() != kindAsd {
			continue
		}
		filtered = true
		if s.allowsSession(name) {
			return true
		}
	}
	return !filtered
}

// configWarnings reports config mistakes that would otherwise fail silently: a
// source whose kind nothing dispatches to (the boo→asd rename made this a real
// failure mode — an unknown kind resolves to zero servers with no error), and a
// filter pattern path.Match cannot parse.
func configWarnings(sources []Source) []string {
	var out []string
	for i, s := range sources {
		if k := s.kind(); k != kindHTTP && k != kindAsd {
			out = append(out, fmt.Sprintf("source #%d (%q) has unknown kind %q and is ignored (want %q or %q)", i, s.Name, k, kindHTTP, kindAsd))
		}
		for _, pat := range append(append([]string{}, s.Include...), s.Exclude...) {
			if _, err := path.Match(pat, "probe"); err != nil {
				out = append(out, fmt.Sprintf("source #%d (%q) has invalid filter pattern %q; it will never match", i, s.Name, pat))
			}
		}
	}
	return out
}

// ResolvedServer is a dispatchable target after expanding sources.
type ResolvedServer struct {
	Name        string
	Description string
	Kind        string
	Endpoint    string
	AuthToken   string
	Session     string
	WaitTimeout string
}

func loadSources(path string) ([]Source, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read a2a config %q: %w", path, err)
	}
	var sources []Source
	if err := json.Unmarshal(data, &sources); err != nil {
		return nil, fmt.Errorf("parse a2a config %q: %w", path, err)
	}
	return sources, nil
}

// runAsd is the single exec seam for the `asd` CLI; tests stub it. stderr is
// returned because asd distinguishes a missing session by message, not by code.
var runAsd = func(ctx context.Context, args ...string) (stdout []byte, stderr []byte, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, "asd", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return out.Bytes(), errb.Bytes(), exitErr.ExitCode(), nil
	}
	if err != nil {
		return out.Bytes(), errb.Bytes(), -1, err
	}
	return out.Bytes(), errb.Bytes(), 0, nil
}

type asdSession struct {
	Name   string `json:"name"`
	Title  string `json:"title"`
	IdleMS int64  `json:"idle_ms"`
	Pid    int    `json:"pid"`
	// Live state, straight from `asd inspect --json`. Without these a reader
	// can only guess a session's state from the spinner glyph in its title.
	Status          string `json:"status"`  // asd's own word: running | idle
	Command         string `json:"command"` // what the session was started with
	AttachedClients int    `json:"attached_clients"`
}

// sizeColumn matches the SIZE column ("181x55") of `asd list`, which every
// session row has and neither the header nor the "no sessions" line does.
var sizeColumn = regexp.MustCompile(`^\d+x\d+$`)

// parseSessionNames pulls the NAME column out of `asd list` output. asd has no
// `list --json`, so the table is the only roster the CLI offers.
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

// asdDataDir is asd's data directory (`$XDG_DATA_HOME/asd`, else
// `~/.local/share/asd`), mirroring asd-proto's path contract.
func asdDataDir() string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "asd")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".local", "share", "asd")
	}
	return filepath.Join(home, ".local", "share", "asd")
}

// procCwd reads a live process's working directory. Seam: tests stub it, and
// on a platform without /proc it simply reports false and the caller falls back.
var procCwd = func(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}
	cwd, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
	if err != nil || cwd == "" {
		return "", false
	}
	return cwd, true
}

// sessionCwd resolves the directory a session is working in. The live cwd of
// its pid wins: the daemon samples sessions.tsv when it writes the file, so for
// a session started as `cd <dir> && exec ...` that snapshot predates the cd.
// The persisted list is the fallback (e.g. no /proc, or an unreadable pid).
func sessionCwd(s asdSession) (string, bool) {
	if cwd, ok := procCwd(s.Pid); ok {
		return cwd, true
	}
	return asdSessionCwd(s.Name)
}

// asdSessionCwd reads a session's working directory from asd's persisted
// session list (`<data dir>/sessions.tsv`, one `name\tcwd` line per session,
// rewritten by the daemon on every create/rename/kill).
func asdSessionCwd(session string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(asdDataDir(), "sessions.tsv"))
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		name, cwd, found := strings.Cut(strings.TrimSuffix(line, "\r"), "\t")
		if !found || name != session {
			continue
		}
		if cwd = strings.TrimSpace(cwd); cwd != "" {
			return cwd, true
		}
	}
	return "", false
}

// asdCapabilitiesDescription reads <cwd>/asd.capabilities.json and renders a
// description (with skills appended). Returns ("", false) if absent/invalid/empty.
func asdCapabilitiesDescription(cwd string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(cwd, "asd.capabilities.json"))
	if err != nil {
		return "", false
	}
	var c struct {
		Description string   `json:"description"`
		Skills      []string `json:"skills"`
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return "", false
	}
	desc := strings.TrimSpace(c.Description)
	if len(c.Skills) > 0 {
		if desc != "" {
			desc += " "
		}
		desc += "[skills: " + strings.Join(c.Skills, ", ") + "]"
	}
	if desc == "" {
		return "", false
	}
	return desc, true
}

// asdRoster returns the live asd sessions (empty on any error). `asd list`
// prints a table with no per-session detail, so each name is then resolved with
// `asd inspect --json`; a session that dies in between is skipped.
func asdRoster(ctx context.Context) []asdSession {
	stdout, _, code, err := runAsd(ctx, "list")
	if err != nil || code != 0 {
		log.Printf("a2a: asd list failed (code=%d): %v", code, err)
		return []asdSession{}
	}
	sessions := []asdSession{}
	for _, name := range parseSessionNames(stdout) {
		out, _, code, err := runAsd(ctx, "inspect", name, "--json")
		if err != nil || code != 0 {
			log.Printf("a2a: asd inspect %s failed (code=%d): %v", name, code, err)
			continue
		}
		var s asdSession
		if err := json.Unmarshal(out, &s); err != nil {
			log.Printf("a2a: parse asd inspect --json for %s: %v", name, err)
			continue
		}
		s.Name = name // `inspect` names the field "session", not "name"
		sessions = append(sessions, s)
	}
	return sessions
}

// SessionDetail is the read payload for a single asd session resource. It is
// the only channel some clients have: codex injects MCP resources but not MCP
// tools, so a2a_list is invisible there and this payload has to stand alone —
// hence the live state fields, not just the routing ones.
type SessionDetail struct {
	Name       string `json:"name"`
	Title      string `json:"title"`
	Status     string `json:"status"`   // running | idle, as asd reports it
	Attached   bool   `json:"attached"` // a client is currently attached
	Command    string `json:"command"`
	IdleMS     int64  `json:"idle_ms"`
	Cwd        string `json:"cwd"`
	Capability string `json:"capability"`
}

// enrichSession fills a session's cwd + capability from where it is running.
func enrichSession(s asdSession) SessionDetail {
	d := SessionDetail{
		Name:     s.Name,
		Title:    s.Title,
		Status:   s.Status,
		Attached: s.AttachedClients > 0,
		Command:  s.Command,
		IdleMS:   s.IdleMS,
	}
	if cwd, ok := sessionCwd(s); ok {
		d.Cwd = cwd
		if cap, ok := asdCapabilitiesDescription(cwd); ok {
			d.Capability = cap
		}
	}
	return d
}

// asdRosterDetailed returns every config-visible session enriched with cwd +
// capability, so a single roster read carries enough for routing decisions.
func asdRosterDetailed(ctx context.Context, sources []Source) []SessionDetail {
	out := []SessionDetail{}
	for _, s := range asdRoster(ctx) {
		if !visibleSession(sources, s.Name) {
			continue
		}
		out = append(out, enrichSession(s))
	}
	return out
}

// asdSessionDetail returns one live session's detail (false if the config hides
// it or it is not live). One `inspect` — reading one session never walks the
// whole roster.
func asdSessionDetail(ctx context.Context, sources []Source, name string) (SessionDetail, bool) {
	if name == "" || !visibleSession(sources, name) {
		return SessionDetail{}, false
	}
	out, _, code, err := runAsd(ctx, "inspect", name, "--json")
	if err != nil || code != 0 {
		return SessionDetail{}, false
	}
	var s asdSession
	if err := json.Unmarshal(out, &s); err != nil {
		log.Printf("a2a: parse asd inspect --json for %s: %v", name, err)
		return SessionDetail{}, false
	}
	s.Name = name // `inspect` names the field "session", not "name"
	return enrichSession(s), true
}

// resolve expands sources into live servers. http passes through; an asd source
// reads the live roster and emits one server per session. asd failures are
// logged and skipped (http sources still resolve). Duplicate names are dropped
// (first wins).
func resolve(ctx context.Context, sources []Source) []ResolvedServer {
	var out []ResolvedServer
	seen := map[string]bool{}
	add := func(rs ResolvedServer) {
		if rs.Name == "" || seen[rs.Name] {
			return
		}
		seen[rs.Name] = true
		out = append(out, rs)
	}
	for _, s := range sources {
		if s.kind() != kindHTTP {
			continue
		}
		add(ResolvedServer{Name: s.Name, Description: s.Description, Kind: kindHTTP, Endpoint: s.Endpoint, AuthToken: s.AuthToken})
	}
	for _, s := range sources {
		if s.kind() != kindAsd {
			continue
		}
		sessions := asdRoster(ctx)
		wt := s.WaitTimeout
		if wt == "" {
			wt = "60s"
		}
		for _, sess := range sessions {
			if !s.allowsSession(sess.Name) {
				continue
			}
			desc := sess.Title
			if cwd, ok := sessionCwd(sess); ok {
				if cap, ok := asdCapabilitiesDescription(cwd); ok {
					desc = cap
				}
			}
			add(ResolvedServer{Name: sess.Name, Description: desc, Kind: kindAsd, Session: sess.Name, WaitTimeout: wt})
		}
	}
	return out
}

// ---- a2a_list tool ----

type ListInput struct{}

type ServerView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Endpoint    string `json:"endpoint"`
	Kind        string `json:"kind"`
}

type ListOutput struct {
	Servers []ServerView `json:"servers" jsonschema:"the A2A servers available to dispatch subtasks to"`
}

func runList(servers []ResolvedServer) ListOutput {
	views := make([]ServerView, 0, len(servers))
	for _, s := range servers {
		views = append(views, ServerView{Name: s.Name, Description: s.Description, Endpoint: s.Endpoint, Kind: s.Kind})
	}
	return ListOutput{Servers: views}
}

// ---- A2A client ----

type a2aClient struct{ http *http.Client }

func newA2AClient(h *http.Client) *a2aClient {
	if h == nil {
		h = http.DefaultClient
	}
	return &a2aClient{http: h}
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

type a2aPart struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}
type a2aMessage struct {
	Kind      string    `json:"kind"`
	Role      string    `json:"role"`
	MessageID string    `json:"messageId"`
	Parts     []a2aPart `json:"parts"`
}
type a2aRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      string         `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}
type a2aResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *a2aClient) send(ctx context.Context, endpoint, authToken, prompt string) (string, error) {
	body := a2aRequest{
		JSONRPC: "2.0",
		ID:      newID("rpc"),
		Method:  "message/send",
		Params: map[string]any{
			"message": a2aMessage{
				Kind: "message", Role: "user", MessageID: newID("msg"),
				Parts: []a2aPart{{Kind: "text", Text: prompt}},
			},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("a2a request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("a2a endpoint returned %d", resp.StatusCode)
	}
	var rpc a2aResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		return "", fmt.Errorf("decode a2a response: %w", err)
	}
	if rpc.Error != nil {
		return "", fmt.Errorf("a2a error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}
	return extractText(rpc.Result)
}

// extractText pulls text from either a Message result or a Task result.
func extractText(raw json.RawMessage) (string, error) {
	var msg a2aMessage
	if err := json.Unmarshal(raw, &msg); err == nil && len(msg.Parts) > 0 {
		return joinText(msg.Parts), nil
	}
	var task struct {
		Status struct {
			Message a2aMessage `json:"message"`
		} `json:"status"`
		Artifacts []struct {
			Parts []a2aPart `json:"parts"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(raw, &task); err != nil {
		return "", fmt.Errorf("unrecognized a2a result: %w", err)
	}
	if len(task.Status.Message.Parts) > 0 {
		return joinText(task.Status.Message.Parts), nil
	}
	for _, a := range task.Artifacts {
		if len(a.Parts) > 0 {
			return joinText(a.Parts), nil
		}
	}
	return "", nil
}

func joinText(parts []a2aPart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Kind == "text" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// ---- a2a_dispatch tool ----

type DispatchInput struct {
	AgentName string `json:"agent_name" jsonschema:"name of the A2A server to dispatch to (from a2a_list)"`
	Prompt    string `json:"prompt" jsonschema:"the self-contained subtask to send"`
}
type DispatchOutput struct {
	Result string `json:"result"`
}

func runDispatch(ctx context.Context, sources []Source, c *a2aClient, in DispatchInput) (DispatchOutput, error) {
	if in.Prompt == "" {
		return DispatchOutput{}, fmt.Errorf("prompt is required")
	}
	var target *ResolvedServer
	for _, s := range resolve(ctx, sources) {
		if s.Name == in.AgentName {
			s := s
			target = &s
			break
		}
	}
	if target == nil {
		return DispatchOutput{}, fmt.Errorf("no such a2a server: %s", in.AgentName)
	}
	switch target.Kind {
	case kindHTTP:
		result, err := c.send(ctx, target.Endpoint, target.AuthToken, in.Prompt)
		if err != nil {
			return DispatchOutput{}, err
		}
		return DispatchOutput{Result: result}, nil
	case kindAsd:
		result, err := dispatchAsd(ctx, target.Session, in.Prompt, target.WaitTimeout)
		if err != nil {
			return DispatchOutput{}, err
		}
		return DispatchOutput{Result: result}, nil
	default:
		return DispatchOutput{}, fmt.Errorf("unknown a2a server kind: %s", target.Kind)
	}
}

// dispatchAsd types the prompt into an asd session, waits for it to settle, and
// returns the newly-produced scrollback (best-effort: a terminal is not a clean
// request/response channel).
func dispatchAsd(ctx context.Context, session, prompt, waitTimeout string) (string, error) {
	before, err := asdPeek(ctx, session)
	if err != nil {
		return "", err
	}
	// Count lines as number of '\n' characters so a trailing newline doesn't
	// produce a phantom empty element (strings.Split("a\n","\n") → ["a",""] = 2).
	beforeLines := strings.Count(before, "\n")

	if _, errb, code, err := runAsd(ctx, "send", session, "--text", prompt, "--enter"); err != nil {
		return "", fmt.Errorf("asd not available: %w", err)
	} else if e := asdDispatchErr(session, code, errb); e != nil {
		return "", e
	}

	// wait is a settle hint; timeout (exit 4) is non-fatal.
	if _, errb, code, err := runAsd(ctx, "wait", session, "--idle", "--timeout", waitTimeout); err != nil {
		return "", fmt.Errorf("asd not available: %w", err)
	} else if e := asdDispatchErr(session, code, errb); e != nil {
		return "", e
	}

	after, err := asdPeek(ctx, session)
	if err != nil {
		return "", err
	}
	afterLines := strings.Split(after, "\n")
	if beforeLines > len(afterLines) {
		beforeLines = len(afterLines)
	}
	delta := afterLines[beforeLines:]
	return trimDelta(delta, prompt), nil
}

func asdPeek(ctx context.Context, session string) (string, error) {
	out, errb, code, err := runAsd(ctx, "peek", session, "--scrollback")
	if err != nil {
		return "", fmt.Errorf("asd not available: %w", err)
	}
	if e := asdDispatchErr(session, code, errb); e != nil {
		return "", e
	}
	return string(out), nil
}

// asdDispatchErr maps one CLI call's outcome to a dispatch error. asd reports a
// missing session as a generic exit 1 with a message on stderr — and words it
// "no session named" in `wait` but "no such session" everywhere else.
func asdDispatchErr(session string, code int, stderr []byte) error {
	if code == 0 || code == 4 { // 4 = wait timeout, non-fatal
		return nil
	}
	msg := string(stderr)
	if strings.Contains(msg, "no such session") || strings.Contains(msg, "no session named") {
		return fmt.Errorf("asd session not running: %s", session)
	}
	if m := strings.TrimSpace(msg); m != "" {
		return fmt.Errorf("asd error for session %s: %s", session, m)
	}
	return fmt.Errorf("asd error (exit %d) for session %s", code, session)
}

// trimDelta cleans the raw scrollback delta:
// (a) if the first line contains the prompt string, drop it (prompt echo);
// (b) drop trailing lines that are empty or end in $ # % > after right-trimming spaces;
// (c) join remaining lines with "\n".
func trimDelta(lines []string, prompt string) string {
	// (a) drop prompt-echo first line
	if len(lines) > 0 && strings.Contains(lines[0], prompt) {
		lines = lines[1:]
	}
	// (b) drop trailing blank/shell-prompt lines
	for len(lines) > 0 {
		last := strings.TrimRight(lines[len(lines)-1], " ")
		if last == "" {
			lines = lines[:len(lines)-1]
			continue
		}
		switch last[len(last)-1] {
		case '$', '#', '%', '>':
			lines = lines[:len(lines)-1]
			continue
		}
		break
	}
	// (c) join
	return strings.Join(lines, "\n")
}
