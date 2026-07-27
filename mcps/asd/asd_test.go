package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// asdListOutput is real `asd list` output (header + right-aligned columns).
const asdListOutput = `NAME                 SIZE   STATUS  CLIENTS      CREATED  COMMAND
build              181x55  running        2       1d ago  make -j
chat               163x45     idle        0       2d ago  claude --dangerously-skip-permissions
`

func TestArgsForLs(t *testing.T) {
	got, err := argsForLs(LsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"list"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestParseSessionNamesSkipsHeaderAndNoSessions(t *testing.T) {
	if got := parseSessionNames([]byte(asdListOutput)); !reflect.DeepEqual(got, []string{"build", "chat"}) {
		t.Fatalf("got %v", got)
	}
	// An empty daemon prints "no sessions" — not a session row.
	if got := parseSessionNames([]byte("no sessions\n")); len(got) != 0 {
		t.Fatalf("want no names, got %v", got)
	}
	if got := parseSessionNames(nil); len(got) != 0 {
		t.Fatalf("want no names, got %v", got)
	}
}

func TestArgsForInspect(t *testing.T) {
	got, err := argsForInspect("build")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"inspect", "build", "--json"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if _, err := argsForInspect(""); err == nil {
		t.Fatal("expected error: name is required")
	}
}

func TestArgsForNew(t *testing.T) {
	// asd has no --cwd: the cwd is folded into the --cmd script.
	got, err := argsForNew(NewInput{Name: "build", Command: []string{"make", "-j"}, Cwd: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"new", "build", "--cmd", "cd /tmp && exec make -j"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	// no name, no command, no cwd -> bare new (asd runs $SHELL, always detached)
	got2, _ := argsForNew(NewInput{})
	if want2 := []string{"new"}; !reflect.DeepEqual(got2, want2) {
		t.Fatalf("got %v want %v", got2, want2)
	}
	// cwd without a command still needs a script to cd into it
	got3, _ := argsForNew(NewInput{Cwd: "/var/my code"})
	if want3 := []string{"new", "--cmd", `cd '/var/my code' && exec "$SHELL"`}; !reflect.DeepEqual(got3, want3) {
		t.Fatalf("got %v want %v", got3, want3)
	}
	// command args with shell metacharacters are quoted
	got4, _ := argsForNew(NewInput{Command: []string{"echo", "a b;c"}})
	if want4 := []string{"new", "--cmd", `echo 'a b;c'`}; !reflect.DeepEqual(got4, want4) {
		t.Fatalf("got %v want %v", got4, want4)
	}
	// asd cannot size a session from the CLI: say so instead of silently ignoring
	if _, err := argsForNew(NewInput{Rows: 24}); err == nil {
		t.Fatal("expected error: asd does not support rows")
	}
	if _, err := argsForNew(NewInput{Cols: 80}); err == nil {
		t.Fatal("expected error: asd does not support cols")
	}
}

func TestArgsForSend(t *testing.T) {
	got, _ := argsForSend(SendInput{Name: "build", Text: "make test", Enter: true})
	if want := []string{"send", "build", "--text", "make test", "--enter"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	keys, _ := argsForSend(SendInput{Name: "build", Keys: "C-c"})
	if want := []string{"send", "build", "--key", "C-c"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("got %v want %v", keys, want)
	}
	if _, err := argsForSend(SendInput{Name: "b", Text: "x", Keys: "C-c"}); err == nil {
		t.Fatal("expected error: text and keys are mutually exclusive")
	}
	if _, err := argsForSend(SendInput{Name: "b"}); err == nil {
		t.Fatal("expected error: one of text/keys required")
	}
}

func TestArgsForPeek(t *testing.T) {
	got, _ := argsForPeek(PeekInput{Name: "build", Scrollback: true})
	if want := []string{"peek", "build", "--json", "--scrollback"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestArgsForWait(t *testing.T) {
	got, _ := argsForWait(WaitInput{Name: "build", Mode: "text", Text: "PASS", Timeout: "2m"})
	if want := []string{"wait", "build", "--text", "PASS", "--timeout", "2m"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	idle, _ := argsForWait(WaitInput{Name: "build", Mode: "idle"})
	if want := []string{"wait", "build", "--idle"}; !reflect.DeepEqual(idle, want) {
		t.Fatalf("got %v want %v", idle, want)
	}
	if _, err := argsForWait(WaitInput{Name: "b", Mode: "text"}); err == nil {
		t.Fatal("expected error: text mode needs text")
	}
	if _, err := argsForWait(WaitInput{Name: "b", Mode: "bogus"}); err == nil {
		t.Fatal("expected error: bad mode")
	}
}

func TestValidateKill(t *testing.T) {
	if err := validateKill(KillInput{Name: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := validateKill(KillInput{All: true}); err != nil {
		t.Fatal(err)
	}
	if err := validateKill(KillInput{Name: "b", All: true}); err == nil {
		t.Fatal("expected error: name and all mutually exclusive")
	}
	if err := validateKill(KillInput{}); err == nil {
		t.Fatal("expected error: one of name/all required")
	}
}

func TestArgsForKillOne(t *testing.T) {
	if want := []string{"kill", "build"}; !reflect.DeepEqual(argsForKillOne("build"), want) {
		t.Fatalf("want %v", want)
	}
}

// stub swaps runAsd for a fixed response.
func stub(out string, code int) func() {
	return stubArgs(func(_ ...string) ([]byte, []byte, int, error) {
		return []byte(out), []byte("boom"), code, nil
	})
}

// stubArgs swaps runAsd for an argv-aware fake.
func stubArgs(fn func(args ...string) ([]byte, []byte, int, error)) func() {
	prev := runAsd
	runAsd = func(_ context.Context, args ...string) ([]byte, []byte, int, error) { return fn(args...) }
	return func() { runAsd = prev }
}

func TestRunLsListsThenInspectsEachSession(t *testing.T) {
	var calls [][]string
	defer stubArgs(func(args ...string) ([]byte, []byte, int, error) {
		calls = append(calls, args)
		if args[0] == "list" {
			return []byte(asdListOutput), nil, 0, nil
		}
		switch args[1] {
		case "build":
			return []byte(`{"session":"build","title":"make","idle_ms":1200,"attached_clients":2}`), nil, 0, nil
		default:
			return []byte(`{"session":"chat","title":"a chat","idle_ms":50,"attached_clients":0}`), nil, 0, nil
		}
	})()

	out, err := runLs(context.Background(), LsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Sessions) != 2 {
		t.Fatalf("want 2 sessions, got %+v", out.Sessions)
	}
	b := out.Sessions[0]
	if b.Name != "build" || b.Title != "make" || b.IdleMs != 1200 || !b.Attached {
		t.Fatalf("build: %+v", b)
	}
	c := out.Sessions[1]
	if c.Name != "chat" || c.Attached {
		t.Fatalf("chat: %+v", c)
	}
	if len(calls) != 3 || calls[0][0] != "list" || calls[1][0] != "inspect" {
		t.Fatalf("calls: %v", calls)
	}
}

// A session that dies between `list` and `inspect` must not fail the whole listing.
func TestRunLsSkipsSessionsThatVanish(t *testing.T) {
	defer stubArgs(func(args ...string) ([]byte, []byte, int, error) {
		if args[0] == "list" {
			return []byte(asdListOutput), nil, 0, nil
		}
		if args[1] == "build" {
			return nil, []byte("Error: inspect failed (2): no such session 'build'"), 1, nil
		}
		return []byte(`{"session":"chat","title":"a chat"}`), nil, 0, nil
	})()
	out, err := runLs(context.Background(), LsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Sessions) != 1 || out.Sessions[0].Name != "chat" {
		t.Fatalf("want only chat, got %+v", out.Sessions)
	}
}

func TestRunNewReturnsName(t *testing.T) {
	defer stub("build\n", 0)()
	out, err := runNew(context.Background(), NewInput{Name: "build", Command: []string{"bash"}})
	if err != nil || out.Name != "build" {
		t.Fatalf("got %+v err %v", out, err)
	}
}

func TestRunPeekParses(t *testing.T) {
	defer stub(`{"session":"build","title":"t","rows":24,"cols":80,"cursor":{"row":1,"col":2},"screen":"hello"}`, 0)()
	out, err := runPeek(context.Background(), PeekInput{Name: "build"})
	if err != nil || out.Screen != "hello" || out.Cursor.Col != 2 {
		t.Fatalf("got %+v err %v", out, err)
	}
}

// asd reports a missing session as exit 1 + a stderr message, not a distinct code.
func TestNoSuchSessionMapsToError(t *testing.T) {
	defer stubArgs(func(_ ...string) ([]byte, []byte, int, error) {
		return nil, []byte("Error: peek failed (2): no such session 'ghost'"), 1, nil
	})()
	_, err := runPeek(context.Background(), PeekInput{Name: "ghost"})
	if err == nil {
		t.Fatal("expected no-such-session error")
	}
	if !strings.Contains(err.Error(), "no such session") || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("unhelpful error: %v", err)
	}
}

// `asd wait` words a missing session differently from every other subcommand
// ("no session named" vs "no such session"); both must map to the same error.
func TestWaitOnMissingSessionMapsToNoSuchSession(t *testing.T) {
	defer stubArgs(func(_ ...string) ([]byte, []byte, int, error) {
		return nil, []byte("Error: wait: no session named 'ghost'"), 1, nil
	})()
	_, err := runWait(context.Background(), WaitInput{Name: "ghost", Mode: "idle"})
	if err == nil || !strings.Contains(err.Error(), "no such session") {
		t.Fatalf("want no-such-session error, got %v", err)
	}
}

func TestOtherFailuresSurfaceStderr(t *testing.T) {
	defer stubArgs(func(_ ...string) ([]byte, []byte, int, error) {
		return nil, []byte("Error: daemon not running"), 1, nil
	})()
	_, err := runPeek(context.Background(), PeekInput{Name: "build"})
	if err == nil || !strings.Contains(err.Error(), "daemon not running") {
		t.Fatalf("want stderr in error, got %v", err)
	}
}

func TestWaitTimeoutIsNotError(t *testing.T) {
	defer stub("", 4)()
	out, err := runWait(context.Background(), WaitInput{Name: "build", Mode: "idle"})
	if err != nil {
		t.Fatalf("timeout should not be an error: %v", err)
	}
	if out.Matched {
		t.Fatal("expected Matched=false on timeout")
	}
}

func TestWaitMatchedTrue(t *testing.T) {
	defer stub("", 0)()
	out, err := runWait(context.Background(), WaitInput{Name: "build", Mode: "text", Text: "PASS"})
	if err != nil || !out.Matched {
		t.Fatalf("got %+v err %v", out, err)
	}
}

func TestRunKillOne(t *testing.T) {
	var calls [][]string
	defer stubArgs(func(args ...string) ([]byte, []byte, int, error) {
		calls = append(calls, args)
		return nil, nil, 0, nil
	})()
	out, err := runKill(context.Background(), KillInput{Name: "build"})
	if err != nil || !out.Ok {
		t.Fatalf("got %+v err %v", out, err)
	}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], []string{"kill", "build"}) {
		t.Fatalf("calls: %v", calls)
	}
}

// asd has no `kill --all`: the tool lists sessions and kills each one.
func TestRunKillAllFansOut(t *testing.T) {
	var killed []string
	defer stubArgs(func(args ...string) ([]byte, []byte, int, error) {
		if args[0] == "list" {
			return []byte(asdListOutput), nil, 0, nil
		}
		killed = append(killed, args[1])
		return nil, nil, 0, nil
	})()
	out, err := runKill(context.Background(), KillInput{All: true})
	if err != nil || !out.Ok {
		t.Fatalf("got %+v err %v", out, err)
	}
	if !reflect.DeepEqual(killed, []string{"build", "chat"}) {
		t.Fatalf("killed: %v", killed)
	}
}

// A session that exits on its own between `list` and `kill` is not a failure.
func TestRunKillAllToleratesVanishedSessions(t *testing.T) {
	defer stubArgs(func(args ...string) ([]byte, []byte, int, error) {
		if args[0] == "list" {
			return []byte(asdListOutput), nil, 0, nil
		}
		if args[1] == "build" {
			return nil, []byte("Error: kill failed (2): no such session 'build'"), 1, nil
		}
		return nil, nil, 0, nil
	})()
	out, err := runKill(context.Background(), KillInput{All: true})
	if err != nil || !out.Ok {
		t.Fatalf("vanished session should not fail kill --all: %+v %v", out, err)
	}
}

func TestSendValidationErrorSkipsExec(t *testing.T) {
	called := false
	defer stubArgs(func(_ ...string) ([]byte, []byte, int, error) {
		called = true
		return nil, nil, 0, nil
	})()
	if _, err := runSend(context.Background(), SendInput{Name: "b"}); err == nil {
		t.Fatal("expected validation error")
	}
	if called {
		t.Fatal("runAsd must not be called when validation fails")
	}
}
