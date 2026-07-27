# asd session capabilities Implementation Plan

> **Historical note:** written against the `boo` CLI and mechanically renamed to `asd`. Where this document describes CLI surface — `ls --json`, `new -d`, `--rows`/`--cols`/`--cwd`, `kill --all`, exit code 3, `<session>.state` — asd differs; `mcps/asd` and `mcps/a2a` are authoritative.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enrich each asd-session A2A server's `description` from `<session-cwd>/asd.capabilities.json`, falling back to the session title when absent.

**Architecture:** Add three small file-reading helpers to `mcps/a2a/a2a.go` (`asdConfigDir`, `asdSessionCwd`, `asdCapabilitiesDescription`) and use them in `resolve`'s asd expansion to compute each asd server's `Description`. All best-effort; failures fall back to the title. Tested with real files via `t.Setenv` + `t.TempDir`.

**Tech Stack:** Go 1.23, stdlib (`os`, `path/filepath`, `encoding/json`, `strings`).

Spec: `docs/superpowers/specs/2026-06-22-a2a-asd-capabilities-design.md`. Branch `feat/a2a-asd-capabilities` (off main). Only `mcps/a2a/{a2a.go,a2a_test.go}` change.

---

## Task 1: capability helpers + resolve enrichment

**Files:** Modify `mcps/a2a/a2a.go`, `mcps/a2a/a2a_test.go`.

- [ ] **Step 1: Write the failing tests**

Append to `a2a_test.go` (imports `os`, `path/filepath`, `context` already present from prior tasks; `testing` present):
```go
func TestAsdConfigDir(t *testing.T) {
	t.Setenv("ASD_CONFIG", "/x/conf.toml")
	t.Setenv("XDG_CONFIG_HOME", "/y")
	if d := asdConfigDir(); d != "/x" {
		t.Fatalf("ASD_CONFIG dir = %q, want /x", d)
	}
	t.Setenv("ASD_CONFIG", "")
	if d := asdConfigDir(); d != "/y/asd" {
		t.Fatalf("XDG dir = %q, want /y/asd", d)
	}
}

func TestAsdSessionCwd(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ASD_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", tmp)
	os.MkdirAll(filepath.Join(tmp, "asd"), 0o755)
	os.WriteFile(filepath.Join(tmp, "asd", "build.state"), []byte("/home/me/proj\n"), 0o600)

	cwd, ok := asdSessionCwd("build")
	if !ok || cwd != "/home/me/proj" {
		t.Fatalf("cwd=%q ok=%v", cwd, ok)
	}
	if _, ok := asdSessionCwd("ghost"); ok {
		t.Fatal("missing .state should be !ok")
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
}

func TestResolveAsdEnrichesDescriptionFromCapabilities(t *testing.T) {
	defer stubAsd(func(args ...string) ([]byte, int, error) {
		return []byte(`[{"name":"build","title":"bash"}]`), 0, nil
	})()
	tmp := t.TempDir()
	t.Setenv("ASD_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", tmp)
	cwd := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "asd"), 0o755)
	os.WriteFile(filepath.Join(tmp, "asd", "build.state"), []byte(cwd+"\n"), 0o600)
	os.WriteFile(filepath.Join(cwd, "asd.capabilities.json"), []byte(`{"description":"go coder"}`), 0o600)

	got := resolve(context.Background(), []Source{{Kind: "asd"}})
	if len(got) != 1 || got[0].Description != "go coder" {
		t.Fatalf("enriched: %+v", got)
	}
}

func TestResolveAsdFallsBackToTitleWhenNoCapabilities(t *testing.T) {
	defer stubAsd(func(args ...string) ([]byte, int, error) {
		return []byte(`[{"name":"build","title":"the title"}]`), 0, nil
	})()
	t.Setenv("ASD_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // empty asd dir → no .state
	got := resolve(context.Background(), []Source{{Kind: "asd"}})
	if len(got) != 1 || got[0].Description != "the title" {
		t.Fatalf("fallback: %+v", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`asdConfigDir` etc. undefined): `go test ./mcps/a2a/`

- [ ] **Step 3: Add the helpers to `a2a.go`**

Add `"path/filepath"` to the import block. Add:
```go
// asdConfigDir is where asd keeps per-session restore snapshots (<session>.state).
func asdConfigDir() string {
	if c := os.Getenv("ASD_CONFIG"); c != "" {
		return filepath.Dir(c)
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "asd")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".config/asd"
	}
	return filepath.Join(home, ".config", "asd")
}

// asdSessionCwd reads the session's saved working directory from its snapshot.
func asdSessionCwd(session string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(asdConfigDir(), session+".state"))
	if err != nil {
		return "", false
	}
	cwd := strings.TrimSpace(string(data))
	if cwd == "" {
		return "", false
	}
	return cwd, true
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
```

- [ ] **Step 4: Use it in `resolve`'s asd expansion**

In `resolve`, inside the per-session loop of the asd source, replace the `Description: sess.Title` with the enriched value. The loop currently does roughly:
```go
for _, sess := range sessions {
	add(ResolvedServer{Name: sess.Name, Description: sess.Title, Kind: kindAsd, Session: sess.Name, WaitTimeout: wt})
}
```
Change to:
```go
for _, sess := range sessions {
	desc := sess.Title
	if cwd, ok := asdSessionCwd(sess.Name); ok {
		if cap, ok := asdCapabilitiesDescription(cwd); ok {
			desc = cap
		}
	}
	add(ResolvedServer{Name: sess.Name, Description: desc, Kind: kindAsd, Session: sess.Name, WaitTimeout: wt})
}
```

- [ ] **Step 5: Run — expect PASS** + build/vet + no blast radius

```bash
cd /root/workspace/master/myclaw/mcps/a2a && go test ./... -v && go vet ./... && go build ./...
cd /root/workspace/master/myclaw && go build ./mcps/echo/... ./mcps/ping/... ./mcps/asd/... ./mcps/a2a/... && go test ./mcps/echo/... ./mcps/ping/... ./mcps/asd/... ./mcps/a2a/...
```

- [ ] **Step 6: Commit** (explicit paths; rm any stray `mcps/a2a/a2a` binary first)

```bash
cd /root/workspace/master/myclaw
git add mcps/a2a/a2a.go mcps/a2a/a2a_test.go
git commit -m "feat(mcps/a2a): enrich asd session description from asd.capabilities.json"
```
(Trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.) `git show --stat HEAD` → only the 2 files.

---

## Task 2: real-asd smoke

**Files:** none (verification; fix-forward into `mcps/a2a` if the real `.state`/path handling differs).

- [ ] **Step 1: Build + a real session with a capabilities file**

```bash
cd /root/workspace/master/myclaw/mcps/a2a && go build -o /tmp/a2a-mcp .
CAPDIR=$(mktemp -d); printf '{"description":"smoke capability panel","skills":["alpha"]}' > "$CAPDIR/asd.capabilities.json"
asd new capsmoke -d --cwd "$CAPDIR" -- bash
sleep 1
cat ~/.config/asd/capsmoke.state   # should print $CAPDIR
```

- [ ] **Step 2: Confirm `a2a_list` shows the capability description**

Drive `/tmp/a2a-mcp --config <[{"kind":"asd"}]>` via the MCP handshake (newline-delimited JSON, go-sdk v0.8.0) and `tools/call a2a_list`; assert the `capsmoke` server's `description` == `"smoke capability panel [skills: alpha]"` (NOT the bash title). If a clean handshake is impractical, validate the helper path directly:
```bash
# what resolve() would compute:
CWD=$(cat ~/.config/asd/capsmoke.state); cat "$CWD/asd.capabilities.json"
```
Then create a SECOND session with NO capabilities file in its cwd and confirm `a2a_list` falls back to its title.
Paste the `a2a_list` (or direct) evidence. If the real `.state` content has trailing data beyond the cwd line or a different shape than "one path line", fix `asdSessionCwd` (take the first line), add a test, and re-run `go test ./mcps/a2a/...`.

- [ ] **Step 3: Cleanup + optional fix-forward commit**

```bash
asd kill capsmoke   # + the second session
# only if Step 2 required a fix:
git add mcps/a2a/a2a.go mcps/a2a/a2a_test.go && git commit -m "fix(mcps/a2a): align asd .state cwd parsing with real asd"
```

---

## Self-Review

**Spec coverage:** cwd via `<asdConfigDir>/<session>.state` (`asdConfigDir` + `asdSessionCwd`, Task 1 tests) ✓; capabilities from `<cwd>/asd.capabilities.json` folded into description + skills (`asdCapabilitiesDescription`) ✓; fallback to title on any miss (`TestResolveAsdFallsBackToTitleWhenNoCapabilities`) ✓; resolve enrichment ✓; no schema change (description only) ✓; real-asd smoke (Task 2) ✓. Out-of-scope (watching, AgentCard, structured skills field, auto-routing) absent.

**Placeholder scan:** Task 2's handshake step has a concrete fallback (validate the helper path directly) when no MCP-handshake helper exists; no "TBD".

**Type consistency:** `asdConfigDir() string`, `asdSessionCwd(string)(string,bool)`, `asdCapabilitiesDescription(string)(string,bool)` consistent between helpers, the `resolve` call site, and the tests. `resolve`/`ResolvedServer`/`stubAsd` unchanged from prior tasks.
