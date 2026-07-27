package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubAsd swaps runAsd for tests that don't care about stderr; returns a restore func.
func stubAsd(fn func(args ...string) ([]byte, int, error)) func() {
	return stubAsdFull(func(args ...string) ([]byte, []byte, int, error) {
		out, code, err := fn(args...)
		return out, nil, code, err
	})
}

// stubAsdFull swaps runAsd for a fake that also controls stderr.
func stubAsdFull(fn func(args ...string) ([]byte, []byte, int, error)) func() {
	prev := runAsd
	runAsd = func(_ context.Context, args ...string) ([]byte, []byte, int, error) { return fn(args...) }
	return func() { runAsd = prev }
}

// stubProcCwd swaps the /proc cwd lookup; returns a restore func.
func stubProcCwd(fn func(pid int) (string, bool)) func() {
	prev := procCwd
	procCwd = fn
	return func() { procCwd = prev }
}

// asdSessionRow is one session in the fake CLI's roster.
type asdSessionRow struct {
	name, title string
	idleMs      int64
	status      string // "" → running
	command     string
	attached    int
}

// fakeAsdCLI mimics the real asd CLI: `list` renders the table (no JSON mode),
// `inspect --json` returns one session's detail.
func fakeAsdCLI(rows ...asdSessionRow) func(args ...string) ([]byte, int, error) {
	return func(args ...string) ([]byte, int, error) {
		switch args[0] {
		case "list":
			var b strings.Builder
			b.WriteString("NAME                 SIZE   STATUS  CLIENTS      CREATED  COMMAND\n")
			for _, r := range rows {
				fmt.Fprintf(&b, "%-18s 80x24     idle        0       1d ago  bash\n", r.name)
			}
			return []byte(b.String()), 0, nil
		case "inspect":
			for _, r := range rows {
				if r.name == args[1] {
					status := r.status
					if status == "" {
						status = "running"
					}
					return []byte(fmt.Sprintf(`{"session":%q,"title":%q,"idle_ms":%d,"status":%q,"command":%q,"attached_clients":%d}`,
						r.name, r.title, r.idleMs, status, r.command, r.attached)), 0, nil
				}
			}
			return nil, 1, nil
		}
		return nil, 0, nil
	}
}

// useSessionList points asd's data dir at a temp dir holding a sessions.tsv
// built from the given name→cwd entries.
func useSessionList(t *testing.T, entries map[string]string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "asd"), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for name, cwd := range entries {
		fmt.Fprintf(&b, "%s\t%s\n", name, cwd)
	}
	if err := os.WriteFile(filepath.Join(dir, "asd", "sessions.tsv"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSources(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")
	os.WriteFile(p, []byte(`[{"kind":"http","name":"w","endpoint":"http://x","auth_token":"sek"},{"kind":"asd"},{"name":"noKind","endpoint":"http://y"}]`), 0o600)
	src, err := loadSources(p)
	if err != nil || len(src) != 3 {
		t.Fatalf("sources: %+v err %v", src, err)
	}
	if src[0].kind() != "http" || src[1].kind() != "asd" || src[2].kind() != "http" {
		t.Fatalf("kinds: %q %q %q", src[0].kind(), src[1].kind(), src[2].kind())
	}
}

func TestLoadSourcesEmptyAndMissing(t *testing.T) {
	if s, err := loadSources(""); err != nil || s != nil {
		t.Fatalf("empty path -> nil,nil, got %+v %v", s, err)
	}
	if _, err := loadSources("/no/such.json"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseSessionNames(t *testing.T) {
	table := "NAME                 SIZE   STATUS  CLIENTS      CREATED  COMMAND\n" +
		"build              181x55  running        2       1d ago  make -j\n" +
		"chat               163x45     idle        0       2d ago  claude\n"
	if got := parseSessionNames([]byte(table)); len(got) != 2 || got[0] != "build" || got[1] != "chat" {
		t.Fatalf("got %v", got)
	}
	// An empty daemon prints "no sessions" — not a session row.
	if got := parseSessionNames([]byte("no sessions\n")); len(got) != 0 {
		t.Fatalf("want none, got %v", got)
	}
}

func TestResolveHTTPPassthrough(t *testing.T) {
	got := resolve(context.Background(), []Source{{Kind: "http", Name: "w", Description: "d", Endpoint: "e", AuthToken: "t"}})
	if len(got) != 1 || got[0].Kind != "http" || got[0].Endpoint != "e" || got[0].AuthToken != "t" {
		t.Fatalf("resolve http: %+v", got)
	}
}

func TestResolveAsdExpandsSessions(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir()) // no sessions.tsv → description falls back to title
	defer stubAsd(fakeAsdCLI(
		asdSessionRow{name: "build", title: "a build"},
		asdSessionRow{name: "chat", title: "a chat"},
	))()
	got := resolve(context.Background(), []Source{{Kind: "asd", WaitTimeout: "30s"}})
	if len(got) != 2 {
		t.Fatalf("want 2 asd servers, got %+v", got)
	}
	if got[0].Kind != "asd" || got[0].Name != "build" || got[0].Session != "build" || got[0].Description != "a build" || got[0].WaitTimeout != "30s" {
		t.Fatalf("asd[0]: %+v", got[0])
	}
}

func TestResolveAsdFailureKeepsHTTP(t *testing.T) {
	defer stubAsd(func(args ...string) ([]byte, int, error) { return nil, 1, nil })() // asd list fails
	got := resolve(context.Background(), []Source{{Kind: "http", Name: "w", Endpoint: "e"}, {Kind: "asd"}})
	if len(got) != 1 || got[0].Name != "w" {
		t.Fatalf("asd failure should leave only http, got %+v", got)
	}
}

func TestRunListOmitsTokenIncludesKind(t *testing.T) {
	out := runList([]ResolvedServer{{Name: "w", Description: "d", Endpoint: "e", Kind: "http", AuthToken: "SECRET"}})
	if len(out.Servers) != 1 || out.Servers[0].Kind != "http" || out.Servers[0].Endpoint != "e" {
		t.Fatalf("list: %+v", out)
	}
	// ServerView has no token field — leaking is a compile error.
}

func TestRunDispatchHTTP(t *testing.T) {
	var gotAuth, gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var req a2aRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotText = req.Params["message"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"].(string)
		w.Write([]byte(`{"result":{"kind":"message","parts":[{"kind":"text","text":"the answer"}]}}`))
	}))
	defer srv.Close()
	sources := []Source{{Kind: "http", Name: "w", Endpoint: srv.URL, AuthToken: "sek"}}
	out, err := runDispatch(context.Background(), sources, newA2AClient(srv.Client()), DispatchInput{AgentName: "w", Prompt: "hi"})
	if err != nil || out.Result != "the answer" {
		t.Fatalf("got %+v err %v", out, err)
	}
	if gotAuth != "Bearer sek" || gotText != "hi" {
		t.Fatalf("auth=%q text=%q", gotAuth, gotText)
	}
}

func TestRunDispatchUnknownAgent(t *testing.T) {
	if _, err := runDispatch(context.Background(), []Source{{Kind: "http", Name: "w", Endpoint: "http://unused"}}, newA2AClient(nil), DispatchInput{AgentName: "ghost", Prompt: "hi"}); err == nil {
		t.Fatal("expected no-such-server error")
	}
}

func TestRunDispatchEmptyPrompt(t *testing.T) {
	if _, err := runDispatch(context.Background(), []Source{{Kind: "http", Name: "w", Endpoint: "http://unused"}}, newA2AClient(nil), DispatchInput{AgentName: "w"}); err == nil {
		t.Fatal("expected empty-prompt error")
	}
}

func TestDispatchAsdDelta(t *testing.T) {
	before := "line1\nline2\n"                                // 2 history lines before
	after := "line1\nline2\necho hello\nhi there\nuser@h:~$ " // prompt echo + answer + shell prompt
	calls := 0
	defer stubAsd(func(args ...string) ([]byte, int, error) {
		calls++
		switch args[0] {
		case "peek":
			if calls == 1 {
				return []byte(before), 0, nil
			}
			return []byte(after), 0, nil
		case "send", "wait":
			return nil, 0, nil
		}
		return nil, 0, nil
	})()
	got, err := dispatchAsd(context.Background(), "build", "echo hello", "30s")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hi there" { // prompt-echo line + trailing shell prompt trimmed
		t.Fatalf("delta = %q, want %q", got, "hi there")
	}
}

// asd reports a missing session as exit 1 + a stderr message, not a distinct code.
func TestDispatchAsdSessionMissing(t *testing.T) {
	defer stubAsdFull(func(args ...string) ([]byte, []byte, int, error) {
		return nil, []byte("Error: peek failed (2): no such session 'ghost'"), 1, nil
	})()
	_, err := dispatchAsd(context.Background(), "ghost", "hi", "5s")
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("want session-not-running error, got %v", err)
	}
}

func TestDispatchAsdTimeoutStillReturns(t *testing.T) {
	calls := 0
	defer stubAsd(func(args ...string) ([]byte, int, error) {
		calls++
		switch args[0] {
		case "peek":
			if calls == 1 {
				return []byte("a\n"), 0, nil
			}
			return []byte("a\npartial output\n"), 0, nil
		case "wait":
			return nil, 4, nil // timeout, non-fatal
		}
		return nil, 0, nil
	})()
	got, err := dispatchAsd(context.Background(), "build", "x", "1s")
	if err != nil {
		t.Fatalf("timeout should not error: %v", err)
	}
	if got != "partial output" {
		t.Fatalf("got %q", got)
	}
}

func TestRunDispatchRoutesAsd(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cli := fakeAsdCLI(asdSessionRow{name: "build", title: "t"})
	peeks := 0
	defer stubAsd(func(args ...string) ([]byte, int, error) {
		switch args[0] {
		case "peek":
			peeks++
			if peeks == 1 {
				return []byte("a\n"), 0, nil
			}
			return []byte("a\nprompt\nresult-text\n"), 0, nil
		case "send", "wait":
			return nil, 0, nil
		}
		return cli(args...)
	})()
	out, err := runDispatch(context.Background(), []Source{{Kind: "asd"}}, newA2AClient(nil), DispatchInput{AgentName: "build", Prompt: "prompt"})
	if err != nil || out.Result == "" {
		t.Fatalf("asd route: out=%+v err=%v", out, err)
	}
}

// ---- Task-2-era http result-shape tests (kept; adapted to new []Source signature) ----

func TestRunDispatchTaskResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":{"kind":"task","status":{"message":{"parts":[{"kind":"text","text":"task-answer"}]}}}}`))
	}))
	defer srv.Close()
	sources := []Source{{Kind: "http", Name: "w", Endpoint: srv.URL}}
	out, err := runDispatch(context.Background(), sources, newA2AClient(srv.Client()), DispatchInput{AgentName: "w", Prompt: "hi"})
	if err != nil || out.Result != "task-answer" {
		t.Fatalf("got %+v err %v", out, err)
	}
}

func TestRunDispatchArtifactResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":{"kind":"task","artifacts":[{"parts":[{"kind":"text","text":"artifact-answer"}]}]}}`))
	}))
	defer srv.Close()
	sources := []Source{{Kind: "http", Name: "w", Endpoint: srv.URL}}
	out, err := runDispatch(context.Background(), sources, newA2AClient(srv.Client()), DispatchInput{AgentName: "w", Prompt: "hi"})
	if err != nil || out.Result != "artifact-answer" {
		t.Fatalf("got %+v err %v", out, err)
	}
}

func TestRunDispatchJSONRPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":{"code":-32000,"message":"boom"}}`))
	}))
	defer srv.Close()
	sources := []Source{{Kind: "http", Name: "w", Endpoint: srv.URL}}
	if _, err := runDispatch(context.Background(), sources, newA2AClient(srv.Client()), DispatchInput{AgentName: "w", Prompt: "hi"}); err == nil {
		t.Fatal("expected a2a json-rpc error")
	}
}

func TestRunDispatchNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	sources := []Source{{Kind: "http", Name: "w", Endpoint: srv.URL}}
	if _, err := runDispatch(context.Background(), sources, newA2AClient(srv.Client()), DispatchInput{AgentName: "w", Prompt: "hi"}); err == nil {
		t.Fatal("expected non-2xx error")
	}
}

// asd keeps its persisted session list in the XDG data dir, not the config dir.
func TestAsdDataDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/y")
	if d := asdDataDir(); d != "/y/asd" {
		t.Fatalf("XDG dir = %q, want /y/asd", d)
	}
	t.Setenv("XDG_DATA_HOME", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local", "share", "asd")
	if d := asdDataDir(); d != want {
		t.Fatalf("home fallback = %q, want %q", d, want)
	}
}

func TestAsdSessionCwd(t *testing.T) {
	useSessionList(t, map[string]string{"build": "/home/me/proj"})

	cwd, ok := asdSessionCwd("build")
	if !ok || cwd != "/home/me/proj" {
		t.Fatalf("cwd=%q ok=%v", cwd, ok)
	}
	if _, ok := asdSessionCwd("ghost"); ok {
		t.Fatal("session absent from the list should be !ok")
	}
}

// The daemon writes an empty cwd field when it cannot read the session's cwd.
func TestAsdSessionCwdEmptyFieldIsNotOk(t *testing.T) {
	useSessionList(t, map[string]string{"build": ""})
	if _, ok := asdSessionCwd("build"); ok {
		t.Fatal("empty cwd should be !ok")
	}
}

func TestAsdSessionCwdMissingFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir()) // no sessions.tsv at all
	if _, ok := asdSessionCwd("build"); ok {
		t.Fatal("missing sessions.tsv should be !ok")
	}
}

// The daemon samples a session's cwd when it writes sessions.tsv — for a
// session created as `cd <dir> && exec ...` that snapshot predates the cd and
// is wrong. The live cwd of the session's pid wins when it is readable.
func TestSessionCwdPrefersLivePidOverSessionList(t *testing.T) {
	useSessionList(t, map[string]string{"build": "/stale/snapshot"})
	defer stubProcCwd(func(pid int) (string, bool) {
		if pid == 4242 {
			return "/live/dir", true
		}
		return "", false
	})()
	if cwd, ok := sessionCwd(asdSession{Name: "build", Pid: 4242}); !ok || cwd != "/live/dir" {
		t.Fatalf("cwd=%q ok=%v, want /live/dir", cwd, ok)
	}
}

func TestSessionCwdFallsBackToSessionList(t *testing.T) {
	useSessionList(t, map[string]string{"build": "/from/list"})
	defer stubProcCwd(func(int) (string, bool) { return "", false })()
	if cwd, ok := sessionCwd(asdSession{Name: "build", Pid: 4242}); !ok || cwd != "/from/list" {
		t.Fatalf("cwd=%q ok=%v, want /from/list", cwd, ok)
	}
}

func TestSessionCwdUnknownIsNotOk(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	defer stubProcCwd(func(int) (string, bool) { return "", false })()
	if _, ok := sessionCwd(asdSession{Name: "build"}); ok {
		t.Fatal("no live pid and no session list should be !ok")
	}
}

// procCwd reads a real process's cwd through /proc.
func TestProcCwdReadsOwnProcess(t *testing.T) {
	want, err := os.Getwd()
	if err != nil {
		t.Skip(err)
	}
	got, ok := procCwd(os.Getpid())
	if !ok || got != want {
		t.Fatalf("procCwd(self) = %q ok=%v, want %q", got, ok, want)
	}
	if _, ok := procCwd(0); ok {
		t.Fatal("pid 0 must be !ok")
	}
}

func TestAsdCapabilitiesDescription(t *testing.T) {
	cwd := t.TempDir()
	os.WriteFile(filepath.Join(cwd, "asd.capabilities.json"),
		[]byte(`{"description":"coding agent","skills":["go","testing"]}`), 0o600)
	got, ok := asdCapabilitiesDescription(cwd)
	if !ok || got != "coding agent [skills: go, testing]" {
		t.Fatalf("got %q ok=%v", got, ok)
	}

	cwd2 := t.TempDir()
	os.WriteFile(filepath.Join(cwd2, "asd.capabilities.json"), []byte(`{"description":"plain"}`), 0o600)
	if got, ok := asdCapabilitiesDescription(cwd2); !ok || got != "plain" {
		t.Fatalf("plain: got %q ok=%v", got, ok)
	}

	if _, ok := asdCapabilitiesDescription(t.TempDir()); ok {
		t.Fatal("missing file should be !ok")
	}
	bad := t.TempDir()
	os.WriteFile(filepath.Join(bad, "asd.capabilities.json"), []byte(`{not json`), 0o600)
	if _, ok := asdCapabilitiesDescription(bad); ok {
		t.Fatal("invalid json should be !ok")
	}
	// Empty object: present but no description/skills → ("", false).
	empty := t.TempDir()
	os.WriteFile(filepath.Join(empty, "asd.capabilities.json"), []byte(`{}`), 0o600)
	if got, ok := asdCapabilitiesDescription(empty); ok || got != "" {
		t.Fatalf("empty object: got %q ok=%v, want (\"\", false)", got, ok)
	}
}

func TestResolveAsdEnrichesDescriptionFromCapabilities(t *testing.T) {
	defer stubAsd(fakeAsdCLI(asdSessionRow{name: "build", title: "bash"}))()
	cwd := t.TempDir()
	useSessionList(t, map[string]string{"build": cwd})
	os.WriteFile(filepath.Join(cwd, "asd.capabilities.json"), []byte(`{"description":"go coder"}`), 0o600)

	got := resolve(context.Background(), []Source{{Kind: "asd"}})
	if len(got) != 1 || got[0].Description != "go coder" {
		t.Fatalf("enriched: %+v", got)
	}
}

func TestResolveAsdFallsBackToTitleWhenNoCapabilities(t *testing.T) {
	defer stubAsd(fakeAsdCLI(asdSessionRow{name: "build", title: "the title"}))()
	t.Setenv("XDG_DATA_HOME", t.TempDir()) // no sessions.tsv → no cwd
	got := resolve(context.Background(), []Source{{Kind: "asd"}})
	if len(got) != 1 || got[0].Description != "the title" {
		t.Fatalf("fallback: %+v", got)
	}
}

func TestAsdRosterParsesSessions(t *testing.T) {
	defer stubAsd(fakeAsdCLI(
		asdSessionRow{name: "build", title: "a build", idleMs: 1200},
		asdSessionRow{name: "chat", title: "a chat", idleMs: 50},
	))()
	got := asdRoster(context.Background())
	if len(got) != 2 || got[0].Name != "build" || got[0].Title != "a build" || got[0].IdleMS != 1200 {
		t.Fatalf("roster: %+v", got)
	}
}

func TestAsdRosterEmptyOnFailure(t *testing.T) {
	restore := stubAsd(func(args ...string) ([]byte, int, error) { return nil, 1, nil })
	if got := asdRoster(context.Background()); got == nil || len(got) != 0 {
		t.Fatalf("want non-nil empty on list failure, got %#v", got)
	}
	restore()

	restore = stubAsd(func(args ...string) ([]byte, int, error) { return nil, 0, errors.New("exec fail") })
	if got := asdRoster(context.Background()); got == nil || len(got) != 0 {
		t.Fatalf("want non-nil empty on exec error, got %#v", got)
	}
	restore()
}

// A session that dies between `list` and `inspect` is dropped, not fatal.
func TestAsdRosterSkipsSessionsThatVanish(t *testing.T) {
	defer stubAsd(func(args ...string) ([]byte, int, error) {
		if args[0] == "list" {
			return fakeAsdCLI(asdSessionRow{name: "build"}, asdSessionRow{name: "chat", title: "a chat"})(args...)
		}
		if args[1] == "build" {
			return nil, 1, nil
		}
		return []byte(`{"session":"chat","title":"a chat"}`), 0, nil
	})()
	got := asdRoster(context.Background())
	if len(got) != 1 || got[0].Name != "chat" {
		t.Fatalf("roster: %+v", got)
	}
}

func TestAsdSessionDetailLiveWithCapability(t *testing.T) {
	defer stubAsd(fakeAsdCLI(asdSessionRow{name: "build", title: "a build", idleMs: 7}))()
	cwd := t.TempDir()
	useSessionList(t, map[string]string{"build": cwd})
	os.WriteFile(filepath.Join(cwd, "asd.capabilities.json"), []byte(`{"description":"go coder"}`), 0o600)

	d, ok := asdSessionDetail(context.Background(), nil, "build")
	if !ok || d.Name != "build" || d.Title != "a build" || d.IdleMS != 7 || d.Cwd != cwd || d.Capability != "go coder" {
		t.Fatalf("detail: %+v ok=%v", d, ok)
	}
}

func TestAsdSessionDetailUnknownIsNotOk(t *testing.T) {
	defer stubAsd(fakeAsdCLI(asdSessionRow{name: "build", title: "a build"}))()
	if _, ok := asdSessionDetail(context.Background(), nil, "ghost"); ok {
		t.Fatal("unknown session must be !ok")
	}
}

func TestAsdSessionDetailLiveNoCapability(t *testing.T) {
	defer stubAsd(fakeAsdCLI(asdSessionRow{name: "build", title: "a build"}))()
	t.Setenv("XDG_DATA_HOME", t.TempDir()) // no sessions.tsv → no cwd
	d, ok := asdSessionDetail(context.Background(), nil, "build")
	if !ok || d.Capability != "" || d.Cwd != "" {
		t.Fatalf("detail: %+v ok=%v (want ok, empty cap/cwd)", d, ok)
	}
}

func TestAsdRosterDetailedEnrichesCapability(t *testing.T) {
	defer stubAsd(fakeAsdCLI(asdSessionRow{name: "build", title: "a build", idleMs: 5}))()
	cwd := t.TempDir()
	useSessionList(t, map[string]string{"build": cwd})
	os.WriteFile(filepath.Join(cwd, "asd.capabilities.json"), []byte(`{"description":"go coder"}`), 0o600)

	got := asdRosterDetailed(context.Background(), nil)
	if len(got) != 1 || got[0].Name != "build" || got[0].Capability != "go coder" || got[0].Cwd != cwd {
		t.Fatalf("detailed roster: %+v", got)
	}
}

func TestSourceAllowsSession(t *testing.T) {
	cases := []struct {
		name    string
		src     Source
		session string
		want    bool
	}{
		{"no filter allows all", Source{}, "build", true},
		{"include exact match", Source{Include: []string{"build"}}, "build", true},
		{"include misses", Source{Include: []string{"build"}}, "chat", false},
		{"include glob", Source{Include: []string{"bot-*"}}, "bot-1", true},
		{"exclude wins over include", Source{Include: []string{"*"}, Exclude: []string{"root"}}, "root", false},
		{"exclude glob", Source{Exclude: []string{"priv-*"}}, "priv-a", false},
		{"exclude leaves others", Source{Exclude: []string{"priv-*"}}, "build", true},
		{"malformed pattern never matches", Source{Include: []string{"["}}, "[", false},
	}
	for _, c := range cases {
		if got := c.src.allowsSession(c.session); got != c.want {
			t.Errorf("%s: allowsSession(%q) = %v, want %v", c.name, c.session, got, c.want)
		}
	}
}

func TestResolveAsdAppliesFilter(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	defer stubAsd(fakeAsdCLI(
		asdSessionRow{name: "build", title: "a build"},
		asdSessionRow{name: "root", title: "a root"},
	))()
	got := resolve(context.Background(), []Source{{Kind: "asd", Exclude: []string{"root"}}})
	if len(got) != 1 || got[0].Name != "build" {
		t.Fatalf("excluded session must not resolve: %+v", got)
	}
}

func TestVisibleSession(t *testing.T) {
	asd := []Source{{Kind: "asd", Include: []string{"build"}}}
	if !visibleSession(asd, "build") || visibleSession(asd, "root") {
		t.Fatal("asd source must gate visibility by its filter")
	}
	// Two asd sources: allowed by either one is enough.
	two := []Source{{Kind: "asd", Include: []string{"build"}}, {Kind: "asd", Include: []string{"root"}}}
	if !visibleSession(two, "root") {
		t.Fatal("a session allowed by any asd source is visible")
	}
	// No asd source: nothing to filter on, so the roster stays informational.
	if !visibleSession([]Source{{Kind: "http", Name: "w"}}, "anything") {
		t.Fatal("without an asd source every session stays visible")
	}
}

func TestAsdRosterDetailedAppliesFilter(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	defer stubAsd(fakeAsdCLI(
		asdSessionRow{name: "build", title: "a build"},
		asdSessionRow{name: "root", title: "a root"},
	))()
	got := asdRosterDetailed(context.Background(), []Source{{Kind: "asd", Exclude: []string{"root"}}})
	if len(got) != 1 || got[0].Name != "build" {
		t.Fatalf("filtered roster: %+v", got)
	}
}

func TestAsdSessionDetailFilteredIsNotOk(t *testing.T) {
	defer stubAsd(fakeAsdCLI(asdSessionRow{name: "root", title: "a root"}))()
	if _, ok := asdSessionDetail(context.Background(), []Source{{Kind: "asd", Exclude: []string{"root"}}}, "root"); ok {
		t.Fatal("an excluded session must not be readable as a resource")
	}
}

// One session's detail is one `inspect` call — not a full roster walk.
func TestAsdSessionDetailUsesSingleInspect(t *testing.T) {
	var calls [][]string
	real := fakeAsdCLI(asdSessionRow{name: "build", title: "a build", idleMs: 7})
	defer stubAsd(func(args ...string) ([]byte, int, error) {
		calls = append(calls, args)
		return real(args...)
	})()
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	d, ok := asdSessionDetail(context.Background(), nil, "build")
	if !ok || d.Name != "build" || d.Title != "a build" || d.IdleMS != 7 {
		t.Fatalf("detail: %+v ok=%v", d, ok)
	}
	if len(calls) != 1 || calls[0][0] != "inspect" || calls[0][1] != "build" {
		t.Fatalf("want a single `inspect build` call, got %v", calls)
	}
}

func TestConfigWarnings(t *testing.T) {
	got := configWarnings([]Source{
		{Kind: "http", Name: "w"},
		{Kind: "asd"},
		{Kind: "boo", Name: "stale"},
		{Kind: "asd", Include: []string{"["}},
	})
	if len(got) != 2 {
		t.Fatalf("want one unknown-kind + one bad-pattern warning, got %v", got)
	}
	if !strings.Contains(got[0], "boo") || !strings.Contains(got[0], "stale") {
		t.Fatalf("unknown-kind warning should name the kind and source: %q", got[0])
	}
	if !strings.Contains(got[1], "[") {
		t.Fatalf("bad-pattern warning should quote the pattern: %q", got[1])
	}
}

func TestConfigWarningsCleanConfig(t *testing.T) {
	if got := configWarnings([]Source{{Kind: "http", Name: "w"}, {Kind: "asd", Exclude: []string{"root", "priv-*"}}}); len(got) != 0 {
		t.Fatalf("a valid config warns about nothing, got %v", got)
	}
}

// A resource read carries the session's live state, not just its routing
// fields: codex clients see MCP resources but no MCP tools, so this payload is
// all they get.
func TestSessionDetailCarriesLiveState(t *testing.T) {
	defer stubAsd(fakeAsdCLI(asdSessionRow{
		name: "build", title: "a build", idleMs: 42,
		status: "idle", command: "claude --resume", attached: 2,
	}))()
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	d, ok := asdSessionDetail(context.Background(), nil, "build")
	if !ok {
		t.Fatal("want ok")
	}
	if d.Status != "idle" || !d.Attached || d.Command != "claude --resume" || d.IdleMS != 42 {
		t.Fatalf("live state lost: %+v", d)
	}
}

func TestSessionDetailUnattached(t *testing.T) {
	defer stubAsd(fakeAsdCLI(asdSessionRow{name: "build", status: "running", attached: 0}))()
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	d, _ := asdSessionDetail(context.Background(), nil, "build")
	if d.Attached || d.Status != "running" {
		t.Fatalf("attached=%v status=%q, want false/running", d.Attached, d.Status)
	}
}

func TestRosterDetailedCarriesLiveState(t *testing.T) {
	defer stubAsd(fakeAsdCLI(
		asdSessionRow{name: "build", status: "running", attached: 1, command: "make"},
		asdSessionRow{name: "chat", status: "idle", command: "bash"},
	))()
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	got := asdRosterDetailed(context.Background(), nil)
	if len(got) != 2 {
		t.Fatalf("roster: %+v", got)
	}
	if got[0].Status != "running" || !got[0].Attached || got[0].Command != "make" {
		t.Fatalf("build: %+v", got[0])
	}
	if got[1].Status != "idle" || got[1].Attached {
		t.Fatalf("chat: %+v", got[1])
	}
}
