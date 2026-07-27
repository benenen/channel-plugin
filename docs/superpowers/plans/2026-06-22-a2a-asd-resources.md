# a2a-mcp asd resources — Implementation Plan

> **Historical note:** written against the `boo` CLI and mechanically renamed to `asd`. Where this document describes CLI surface — `ls --json`, `new -d`, `--rows`/`--cols`/`--cwd`, `kill --all`, exit code 3, `<session>.state` — asd differs; `mcps/asd` and `mcps/a2a` are authoritative.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `mcps/a2a` expose live asd sessions as MCP **resources** (`asd://sessions` roster + `asd://session/{name}` template) so codex — whose MCP-tools path is broken — can discover sessions/capabilities via `list_mcp_resources`/`read_mcp_resource`. Tools stay unchanged. Additive, backward compatible.

**Architecture:** Add pure helpers (`asdRoster`, `asdSessionDetail`, `SessionDetail`) to `a2a.go` reusing `runAsd`/`asdSessionCwd`/`asdCapabilitiesDescription`; wire two thin mcp resource handlers in `main.go` via `AddResource`/`AddResourceTemplate`.

**Tech Stack:** Go 1.23, `modelcontextprotocol/go-sdk v0.8.0` (stdio, newline-delimited JSON), `asd` CLI via the `runAsd` seam.

**Spec:** `docs/superpowers/specs/2026-06-22-a2a-asd-resources-design.md`. Branch `feat/a2a-asd-resources` off main. Only `mcps/a2a/{a2a.go,a2a_test.go,main.go}` change. Implement in an isolated worktree; never edit `/usr/local/bin/a2a-mcp` in place.

---

## Task 1: `asdSession.IdleMS` + `asdRoster` + `asdSessionDetail` helpers

**Files:** Modify `mcps/a2a/a2a.go`, `mcps/a2a/a2a_test.go`.

- [ ] **Step 1: Write the failing tests**

Append to `a2a_test.go` (`context`, `os`, `path/filepath`, `testing` already imported):
```go
func TestAsdRosterParsesSessions(t *testing.T) {
	defer stubAsd(func(args ...string) ([]byte, int, error) {
		return []byte(`[{"name":"build","title":"a build","idle_ms":1200},{"name":"chat","title":"a chat","idle_ms":50}]`), 0, nil
	})()
	got := asdRoster(context.Background())
	if len(got) != 2 || got[0].Name != "build" || got[0].Title != "a build" || got[0].IdleMS != 1200 {
		t.Fatalf("roster: %+v", got)
	}
}

func TestAsdRosterEmptyOnFailure(t *testing.T) {
	defer stubAsd(func(args ...string) ([]byte, int, error) { return nil, 1, nil })()
	if got := asdRoster(context.Background()); len(got) != 0 {
		t.Fatalf("want empty on ls failure, got %+v", got)
	}
	defer stubAsd(func(args ...string) ([]byte, int, error) { return []byte(`{bad`), 0, nil })()
	if got := asdRoster(context.Background()); len(got) != 0 {
		t.Fatalf("want empty on bad json, got %+v", got)
	}
}

func TestAsdSessionDetailLiveWithCapability(t *testing.T) {
	defer stubAsd(func(args ...string) ([]byte, int, error) {
		return []byte(`[{"name":"build","title":"a build","idle_ms":7}]`), 0, nil
	})()
	tmp := t.TempDir()
	t.Setenv("ASD_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", tmp)
	cwd := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "asd"), 0o755)
	os.WriteFile(filepath.Join(tmp, "asd", "build.state"), []byte(cwd+"\n"), 0o600)
	os.WriteFile(filepath.Join(cwd, "asd.capabilities.json"), []byte(`{"description":"go coder"}`), 0o600)

	d, ok := asdSessionDetail(context.Background(), "build")
	if !ok || d.Name != "build" || d.Title != "a build" || d.IdleMS != 7 || d.Cwd != cwd || d.Capability != "go coder" {
		t.Fatalf("detail: %+v ok=%v", d, ok)
	}
}

func TestAsdSessionDetailUnknownIsNotOk(t *testing.T) {
	defer stubAsd(func(args ...string) ([]byte, int, error) {
		return []byte(`[{"name":"build","title":"a build"}]`), 0, nil
	})()
	if _, ok := asdSessionDetail(context.Background(), "ghost"); ok {
		t.Fatal("unknown session must be !ok")
	}
}

func TestAsdSessionDetailLiveNoCapability(t *testing.T) {
	defer stubAsd(func(args ...string) ([]byte, int, error) {
		return []byte(`[{"name":"build","title":"a build"}]`), 0, nil
	})()
	t.Setenv("ASD_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // empty asd dir → no .state
	d, ok := asdSessionDetail(context.Background(), "build")
	if !ok || d.Capability != "" || d.Cwd != "" {
		t.Fatalf("detail: %+v ok=%v (want ok, empty cap/cwd)", d, ok)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`asdRoster`/`asdSessionDetail`/`IdleMS` undefined): `cd mcps/a2a && go test ./...`

- [ ] **Step 3: Implement in `a2a.go`**

Extend `asdSession` (add the field after `Title`):
```go
type asdSession struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	IdleMS int64 `json:"idle_ms"`
}
```
Add (near the other asd helpers):
```go
// asdRoster returns the live asd sessions (empty on any error).
func asdRoster(ctx context.Context) []asdSession {
	stdout, code, err := runAsd(ctx, "ls", "--json")
	if err != nil || code != 0 {
		return nil
	}
	var sessions []asdSession
	if err := json.Unmarshal(stdout, &sessions); err != nil {
		return nil
	}
	return sessions
}

// SessionDetail is the read payload for a single asd session resource.
type SessionDetail struct {
	Name       string `json:"name"`
	Title      string `json:"title"`
	IdleMS     int64  `json:"idle_ms"`
	Cwd        string `json:"cwd"`
	Capability string `json:"capability"`
}

// asdSessionDetail returns one live session's detail (false if not a live session).
func asdSessionDetail(ctx context.Context, name string) (SessionDetail, bool) {
	for _, s := range asdRoster(ctx) {
		if s.Name != name {
			continue
		}
		d := SessionDetail{Name: s.Name, Title: s.Title, IdleMS: s.IdleMS}
		if cwd, ok := asdSessionCwd(name); ok {
			d.Cwd = cwd
			if cap, ok := asdCapabilitiesDescription(cwd); ok {
				d.Capability = cap
			}
		}
		return d, true
	}
	return SessionDetail{}, false
}
```
(`encoding/json` is already imported in `a2a.go`.)

- [ ] **Step 4: Run — expect PASS** + vet: `cd mcps/a2a && go test ./... && go vet ./...`

- [ ] **Step 5: Commit**
```bash
git add mcps/a2a/a2a.go mcps/a2a/a2a_test.go
git commit -m "feat(mcps/a2a): asdRoster + asdSessionDetail helpers for resource exposure"
```
(Trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.)

---

## Task 2: wire the two MCP resources in `main.go`

**Files:** Modify `mcps/a2a/main.go`. (Handlers are thin; covered by the build smoke in Task 3 — go-sdk resource plumbing isn't unit-testable without a client.)

- [ ] **Step 1: Add imports**

In `main.go` add `"encoding/json"` and `"strings"` to the import block.

- [ ] **Step 2: Register the resources** (immediately before `if err := server.Run(...)`)
```go
	server.AddResource(&mcp.Resource{
		URI:         "asd://sessions",
		Name:        "asd-sessions",
		Title:       "Live asd sessions",
		Description: "Live asd sessions you can route subtasks to. Read asd://session/<name> for one session's capabilities.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		data, _ := json.Marshal(map[string]any{"sessions": asdRoster(ctx)})
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
			{URI: "asd://sessions", MIMEType: "application/json", Text: string(data)},
		}}, nil
	})

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "asd://session/{name}",
		Name:        "asd-session",
		Title:       "asd session detail",
		Description: "Capabilities + live status of one asd session.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		name := strings.TrimPrefix(req.Params.URI, "asd://session/")
		detail, ok := asdSessionDetail(ctx, name)
		if !ok {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		data, _ := json.Marshal(detail)
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
			{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
		}}, nil
	})
```
(Verify `req.Params.URI` is the correct accessor for `*mcp.ReadResourceRequest` in go-sdk v0.8.0 — `ReadResourceRequest = ServerRequest[*ReadResourceParams]`; `ReadResourceParams.URI`. The `mcp/server_example_test.go` ReadResource example shows the shape.)

- [ ] **Step 3: Build + vet** (whole repo + a2a module)
```bash
cd /root/workspace/master/myclaw/mcps/a2a && go build ./... && go vet ./...
cd /root/workspace/master/myclaw && go build ./mcps/a2a/...
```
Expected: clean.

- [ ] **Step 4: Commit**
```bash
git add mcps/a2a/main.go
git commit -m "feat(mcps/a2a): expose asd sessions as MCP resources (roster + per-session template)"
```

---

## Task 3: build smoke — resources visible + readable over MCP

**Files:** none (verification; fix-forward into Task 1/2 files if a shape is wrong).

- [ ] **Step 1: Build the probe binary**
```bash
cd /root/workspace/master/myclaw/mcps/a2a && go build -o /tmp/a2a-mcp-res .
```

- [ ] **Step 2: Drive the MCP handshake** (newline-delimited JSON; keep stdin open with a trailing sleep so the server flushes before EOF)
```bash
{ printf '%s\n' \
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"p","version":"0"}}}' \
'{"jsonrpc":"2.0","method":"notifications/initialized"}' \
'{"jsonrpc":"2.0","id":2,"method":"resources/list","params":{}}' \
'{"jsonrpc":"2.0","id":3,"method":"resources/templates/list","params":{}}' \
'{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"asd://sessions"}}'; sleep 3; } \
| timeout 8 /tmp/a2a-mcp-res --config /root/.myclaw/a2a-servers.json 2>/dev/null
```
Assert: id=2 lists `asd://sessions`; id=3 lists the `asd://session/{name}` template; id=4 returns `{"sessions":[…]}`. Then read one live session by name (pick a name from id=4) via another `resources/read {"uri":"asd://session/<name>"}` and assert it returns `{"name":…,"capability":…}`. Paste evidence.

- [ ] **Step 3: Cleanup** `rm -f /tmp/a2a-mcp-res`. No commit (verification only) unless a fix was needed.

---

## Self-Review

**Spec coverage:** roster resource `asd://sessions` (Task 2 + smoke) ✓; per-session template `asd://session/{name}` with dynamic capability read (Task 1 `asdSessionDetail` + Task 2) ✓; cheap roster vs per-session capability split ✓; reuse of `runAsd`/`asdSessionCwd`/`asdCapabilitiesDescription` ✓; tools unchanged (no edits to AddTool) ✓; best-effort empty-on-failure roster + `ResourceNotFoundError` for unknown session ✓; build smoke ✓. Out-of-scope (tool removal, HTTP-as-resource, dynamic concrete registration, new dispatch) untouched.

**Placeholder scan:** every code step is complete; the one verify-note (`req.Params.URI` accessor) names the exact type chain and the example file. No TBD.

**Type consistency:** `asdSession{Name,Title,IdleMS}`, `SessionDetail{Name,Title,IdleMS,Cwd,Capability}`, `asdRoster(ctx)[]asdSession`, `asdSessionDetail(ctx,name)(SessionDetail,bool)` are consistent between a2a.go, the tests, and the main.go handlers. `runAsd`/`asdSessionCwd`/`asdCapabilitiesDescription` reused unchanged.
