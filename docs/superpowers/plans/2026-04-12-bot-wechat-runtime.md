# Bot WeChat Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Start a WeChat runtime loop after bot login, restore runtimes on service startup, and log inbound WeChat messages without persistence or replies.

**Architecture:** Add a process-local `BotConnectionManager` in the app layer to own runtime lifecycle and bot status transitions. Extend the provider contract with runtime startup callbacks, implement a WeChat runtime loop, and have bootstrap restore logged-in bots automatically.

**Tech Stack:** Go 1.23, net/http, GORM/SQLite, existing bot/channel abstractions, `log`, goroutines, contexts

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `internal/channel/provider.go` | Modify | Define runtime start contract, runtime event types, and runtime handle interface |
| `internal/app/bot_connection_manager.go` | Create | Manage active bot runtimes, status transitions, and structured message logging |
| `internal/app/bot_connection_manager_test.go` | Create | Verify manager start/duplicate/error/event behavior |
| `internal/app/bot_service.go` | Modify | Trigger runtime start after successful login |
| `internal/app/bot_service_test.go` | Modify | Verify login success starts runtime and startup failures mark error |
| `internal/channel/wechat/runtime.go` | Create | Implement WeChat runtime startup and inbound event loop |
| `internal/channel/wechat/runtime_test.go` | Create | Verify credential parsing and event emission |
| `internal/channel/wechat/provider.go` | Modify | Expose runtime startup via provider contract |
| `internal/channel/wechat/fake_provider.go` | Modify | Add deterministic runtime behavior for tests |
| `internal/channel/wechat/fake_provider_test.go` | Modify | Verify fake runtime hooks |
| `internal/bootstrap/bootstrap.go` | Modify | Construct connection manager and restore bot runtimes on startup |
| `internal/bootstrap/bootstrap_test.go` | Modify | Verify startup restore attempts and non-blocking failure behavior |

## Type Definitions To Introduce

Add these types to `internal/channel/provider.go` before implementing later tasks:

```go
type RuntimeEvent struct {
	BotID       string
	ChannelType string
	MessageID   string
	From        string
	Text        string
	Raw         []byte
}

type RuntimeState string

const (
	RuntimeStateConnected RuntimeState = "connected"
	RuntimeStateError     RuntimeState = "error"
	RuntimeStateStopped   RuntimeState = "stopped"
)

type RuntimeStateEvent struct {
	BotID       string
	ChannelType string
	State       RuntimeState
	Err         error
	Reason      string
}

type RuntimeCallbacks struct {
	OnEvent func(RuntimeEvent)
	OnState func(RuntimeStateEvent)
}

type StartRuntimeRequest struct {
	BotID              string
	ChannelType        string
	AccountUID         string
	CredentialPayload  []byte
	CredentialVersion  int
	Callbacks          RuntimeCallbacks
}

type RuntimeHandle interface {
	Stop()
	Done() <-chan struct{}
}

type RuntimeStarter interface {
	StartRuntime(ctx context.Context, req StartRuntimeRequest) (RuntimeHandle, error)
}
```

## Task 1: Add Provider Runtime Contract

**Files:**
- Modify: `internal/channel/provider.go:1-120`
- Test: later tasks use this contract

- [ ] **Step 1: Write the failing compile target**

Add this compile-time assertion near the bottom of `internal/channel/wechat/fake_provider.go`:

```go
var _ channel.RuntimeStarter = (*FakeProvider)(nil)
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/channel/wechat -run 'TestFakeProviderBuildRuntimeConfig$'
```

Expected: FAIL with a compile error that `FakeProvider` does not implement `channel.RuntimeStarter`

- [ ] **Step 3: Add the runtime contract**

Replace `internal/channel/provider.go` with these additions while keeping existing types:

```go
package channel

import (
	"context"
	"time"
)

type Provider interface {
	CreateBinding(ctx context.Context, req CreateBindingRequest) (CreateBindingResult, error)
	RefreshBinding(ctx context.Context, req RefreshBindingRequest) (RefreshBindingResult, error)
	BuildRuntimeConfig(ctx context.Context, req BuildRuntimeConfigRequest) (RuntimeConfig, error)
}

type RuntimeEvent struct {
	BotID       string
	ChannelType string
	MessageID   string
	From        string
	Text        string
	Raw         []byte
}

type RuntimeState string

const (
	RuntimeStateConnected RuntimeState = "connected"
	RuntimeStateError     RuntimeState = "error"
	RuntimeStateStopped   RuntimeState = "stopped"
)

type RuntimeStateEvent struct {
	BotID       string
	ChannelType string
	State       RuntimeState
	Err         error
	Reason      string
}

type RuntimeCallbacks struct {
	OnEvent func(RuntimeEvent)
	OnState func(RuntimeStateEvent)
}

type StartRuntimeRequest struct {
	BotID             string
	ChannelType       string
	AccountUID        string
	CredentialPayload []byte
	CredentialVersion int
	Callbacks         RuntimeCallbacks
}

type RuntimeHandle interface {
	Stop()
	Done() <-chan struct{}
}

type RuntimeStarter interface {
	StartRuntime(ctx context.Context, req StartRuntimeRequest) (RuntimeHandle, error)
}
```

- [ ] **Step 4: Run test to verify compile is green again**

Run:
```bash
go test ./internal/channel/wechat -run 'TestFakeProviderBuildRuntimeConfig$'
```

Expected: PASS or next compile failure now moves to missing `StartRuntime` implementation

- [ ] **Step 5: Commit**

```bash
git add internal/channel/provider.go internal/channel/wechat/fake_provider.go
git commit -m "feat: add channel runtime contract"
```

## Task 2: Add BotConnectionManager

**Files:**
- Create: `internal/app/bot_connection_manager.go`
- Create: `internal/app/bot_connection_manager_test.go`
- Modify: `internal/domain/repositories.go` only if current interfaces are insufficient for loading bot/account records

- [ ] **Step 1: Write the failing manager test**

Create `internal/app/bot_connection_manager_test.go` with this test first:

```go
package app

import (
	"context"
	"testing"

	"github.com/benenen/myclaw/internal/channel"
	"github.com/benenen/myclaw/internal/domain"
)

type runtimeStarterStub struct {
	startCalls int
}

func (s *runtimeStarterStub) StartRuntime(_ context.Context, req channel.StartRuntimeRequest) (channel.RuntimeHandle, error) {
	s.startCalls++
	if req.Callbacks.OnState != nil {
		req.Callbacks.OnState(channel.RuntimeStateEvent{
			BotID: req.BotID,
			State: channel.RuntimeStateConnected,
		})
	}
	return runtimeHandleStub{done: make(chan struct{})}, nil
}

type runtimeHandleStub struct {
	done chan struct{}
}

func (h runtimeHandleStub) Stop()              { close(h.done) }
func (h runtimeHandleStub) Done() <-chan struct{} { return h.done }

func TestBotConnectionManagerStartMarksBotConnected(t *testing.T) {
	ctx := context.Background()
	starter := &runtimeStarterStub{}
	manager := NewBotConnectionManager(nil, nil, starter)

	if err := manager.Start(ctx, "bot_1"); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/app -run 'TestBotConnectionManagerStartMarksBotConnected$'
```

Expected: FAIL with `undefined: NewBotConnectionManager`

- [ ] **Step 3: Write minimal manager skeleton**

Create `internal/app/bot_connection_manager.go`:

```go
package app

import (
	"context"
	"errors"
	"sync"

	"github.com/benenen/myclaw/internal/channel"
)

var ErrRuntimeAlreadyStarted = errors.New("runtime already started")

type BotConnectionManager struct {
	mu      sync.Mutex
	handles map[string]channel.RuntimeHandle
	starter channel.RuntimeStarter
}

func NewBotConnectionManager(_ any, _ any, starter channel.RuntimeStarter) *BotConnectionManager {
	return &BotConnectionManager{
		handles: make(map[string]channel.RuntimeHandle),
		starter: starter,
	}
}

func (m *BotConnectionManager) Start(ctx context.Context, botID string) error {
	m.mu.Lock()
	if _, exists := m.handles[botID]; exists {
		m.mu.Unlock()
		return ErrRuntimeAlreadyStarted
	}
	m.mu.Unlock()

	handle, err := m.starter.StartRuntime(ctx, channel.StartRuntimeRequest{BotID: botID})
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.handles[botID] = handle
	m.mu.Unlock()
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/app -run 'TestBotConnectionManagerStartMarksBotConnected$'
```

Expected: PASS

- [ ] **Step 5: Add duplicate-start failing test**

Append:

```go
func TestBotConnectionManagerRejectsDuplicateStart(t *testing.T) {
	ctx := context.Background()
	starter := &runtimeStarterStub{}
	manager := NewBotConnectionManager(nil, nil, starter)

	if err := manager.Start(ctx, "bot_1"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(ctx, "bot_1"); err != ErrRuntimeAlreadyStarted {
		t.Fatalf("expected ErrRuntimeAlreadyStarted, got %v", err)
	}
}
```

- [ ] **Step 6: Run test to verify it passes**

Run:
```bash
go test ./internal/app -run 'TestBotConnectionManager(StartMarksBotConnected|RejectsDuplicateStart)$'
```

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/app/bot_connection_manager.go internal/app/bot_connection_manager_test.go
git commit -m "feat: add bot connection manager"
```

## Task 3: Make FakeProvider Support Runtime Tests

**Files:**
- Modify: `internal/channel/wechat/fake_provider.go`
- Modify: `internal/channel/wechat/fake_provider_test.go`

- [ ] **Step 1: Write the failing runtime test**

Append to `internal/channel/wechat/fake_provider_test.go`:

```go
func TestFakeProviderStartRuntimeEmitsConnectedState(t *testing.T) {
	provider := NewFakeProvider()
	connected := false

	handle, err := provider.StartRuntime(context.Background(), channel.StartRuntimeRequest{
		BotID:      "bot_1",
		ChannelType: "wechat",
		Callbacks: channel.RuntimeCallbacks{
			OnState: func(event channel.RuntimeStateEvent) {
				if event.State == channel.RuntimeStateConnected {
					connected = true
				}
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Stop()

	if !connected {
		t.Fatal("expected connected callback")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/channel/wechat -run 'TestFakeProviderStartRuntimeEmitsConnectedState$'
```

Expected: FAIL with `provider.StartRuntime undefined`

- [ ] **Step 3: Implement minimal fake runtime**

Add to `internal/channel/wechat/fake_provider.go`:

```go
type fakeRuntimeHandle struct {
	done chan struct{}
	once sync.Once
}

func (h *fakeRuntimeHandle) Stop() {
	h.once.Do(func() { close(h.done) })
}

func (h *fakeRuntimeHandle) Done() <-chan struct{} {
	return h.done
}

func (p *FakeProvider) StartRuntime(_ context.Context, req channel.StartRuntimeRequest) (channel.RuntimeHandle, error) {
	handle := &fakeRuntimeHandle{done: make(chan struct{})}
	if req.Callbacks.OnState != nil {
		req.Callbacks.OnState(channel.RuntimeStateEvent{
			BotID:       req.BotID,
			ChannelType: req.ChannelType,
			State:       channel.RuntimeStateConnected,
		})
	}
	return handle, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/channel/wechat -run 'TestFakeProviderStartRuntimeEmitsConnectedState$'
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channel/wechat/fake_provider.go internal/channel/wechat/fake_provider_test.go
	git commit -m "test: add fake wechat runtime support"
```

## Task 4: Trigger Runtime Startup After Login Success

**Files:**
- Modify: `internal/app/bot_service.go`
- Modify: `internal/app/bot_service_test.go`

- [ ] **Step 1: Write the failing service test**

Add this test to `internal/app/bot_service_test.go`:

```go
func TestBotServiceRefreshLoginStartsRuntimeAfterConfirm(t *testing.T) {
	deps := newBotServiceTestDeps(t)
	provider := deps.provider
	service := NewBotService(deps.users, deps.bots, deps.bindings, deps.accounts, deps.cipher, provider)

	createOut, err := service.CreateBot(context.Background(), CreateBotInput{
		ExternalUserID: "u_123",
		Name:           "bot one",
		ChannelType:    "wechat",
	})
	if err != nil {
		t.Fatal(err)
	}

	startOut, err := service.StartLogin(context.Background(), StartBotLoginInput{BotID: createOut.BotID})
	if err != nil {
		t.Fatal(err)
	}
	provider.SimulateConfirm(startOut.BindingID)

	_, err = service.RefreshLogin(context.Background(), startOut.BindingID)
	if err != nil {
		t.Fatal(err)
	}

	if !provider.RuntimeStarted(startOut.BindingID) {
		t.Fatal("expected runtime to start after confirmed login")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/app -run 'TestBotServiceRefreshLoginStartsRuntimeAfterConfirm$'
```

Expected: FAIL because fake provider does not yet expose runtime start tracking and service does not start a runtime

- [ ] **Step 3: Add runtime dependency to BotService**

Update the struct and constructor in `internal/app/bot_service.go`:

```go
type BotService struct {
	users      domain.UserRepository
	bots       domain.BotRepository
	bindings   domain.ChannelBindingRepository
	accounts   domain.ChannelAccountRepository
	cipher     *security.Cipher
	provider   channel.Provider
	runtimes   *BotConnectionManager
}

func NewBotService(
	users domain.UserRepository,
	bots domain.BotRepository,
	bindings domain.ChannelBindingRepository,
	accounts domain.ChannelAccountRepository,
	cipher *security.Cipher,
	provider channel.Provider,
	runtimes *BotConnectionManager,
) *BotService {
	return &BotService{
		users:    users,
		bots:     bots,
		bindings: bindings,
		accounts: accounts,
		cipher:   cipher,
		provider: provider,
		runtimes: runtimes,
	}
}
```

- [ ] **Step 4: Start runtime on confirmed login**

Inside `RefreshLogin`, after bot/account update succeeds and before returning:

```go
		bot.ConnectionStatus = domain.BotConnectionStatusConnecting
		bot.ConnectionError = ""
		if _, err := s.bots.Update(ctx, bot); err != nil {
			return RefreshBotLoginOutput{}, err
		}
		if s.runtimes != nil {
			if err := s.runtimes.Start(ctx, bot.ID); err != nil {
				bot.ConnectionStatus = domain.BotConnectionStatusError
				bot.ConnectionError = err.Error()
				_, _ = s.bots.Update(ctx, bot)
				return RefreshBotLoginOutput{}, err
			}
		}
```

- [ ] **Step 5: Update tests and helpers to pass manager**

Where `NewBotService(...)` is called in tests, construct a manager with the fake provider if it implements `channel.RuntimeStarter`.

Example helper snippet:

```go
starter, _ := provider.(channel.RuntimeStarter)
runtimes := NewBotConnectionManager(bots, accounts, starter)
svc := NewBotService(users, bots, bindings, accounts, cipher, provider, runtimes)
```

- [ ] **Step 6: Run test to verify it passes**

Run:
```bash
go test ./internal/app -run 'TestBotServiceRefreshLoginStartsRuntimeAfterConfirm$'
```

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/app/bot_service.go internal/app/bot_service_test.go internal/channel/wechat/fake_provider.go
	git commit -m "feat: start bot runtime after login"
```

## Task 5: Load Bot and Account Context in Manager

**Files:**
- Modify: `internal/app/bot_connection_manager.go`
- Modify: `internal/app/bot_connection_manager_test.go`

- [ ] **Step 1: Write the failing context-loading test**

Add this test:

```go
func TestBotConnectionManagerPassesStoredCredentialsToRuntime(t *testing.T) {
	ctx := context.Background()
	starter := &capturingRuntimeStarter{}
	bots := newBotRepoStub(domain.Bot{ID: "bot_1", ChannelType: "wechat", ChannelAccountID: "acct_1"})
	accounts := newAccountRepoStub(domain.ChannelAccount{ID: "acct_1", AccountUID: "wxid_1", CredentialCiphertext: []byte("cipher"), CredentialVersion: 2})
	manager := NewBotConnectionManager(bots, accounts, starter)

	if err := manager.Start(ctx, "bot_1"); err != nil {
		t.Fatal(err)
	}
	if starter.req.AccountUID != "wxid_1" {
		t.Fatalf("unexpected account uid: %q", starter.req.AccountUID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/app -run 'TestBotConnectionManagerPassesStoredCredentialsToRuntime$'
```

Expected: FAIL because manager does not load repositories yet

- [ ] **Step 3: Implement repository-backed manager start**

Replace the manager constructor and start logic in `internal/app/bot_connection_manager.go` with repository-backed loading:

```go
type BotConnectionManager struct {
	mu       sync.Mutex
	handles  map[string]channel.RuntimeHandle
	bots     domain.BotRepository
	accounts domain.ChannelAccountRepository
	starter  channel.RuntimeStarter
}

func NewBotConnectionManager(bots domain.BotRepository, accounts domain.ChannelAccountRepository, starter channel.RuntimeStarter) *BotConnectionManager {
	return &BotConnectionManager{
		handles:  make(map[string]channel.RuntimeHandle),
		bots:     bots,
		accounts: accounts,
		starter:  starter,
	}
}

func (m *BotConnectionManager) Start(ctx context.Context, botID string) error {
	m.mu.Lock()
	if _, exists := m.handles[botID]; exists {
		m.mu.Unlock()
		return ErrRuntimeAlreadyStarted
	}
	m.mu.Unlock()

	bot, err := m.bots.GetByID(ctx, botID)
	if err != nil {
		return err
	}
	account, err := m.accounts.GetByID(ctx, bot.ChannelAccountID)
	if err != nil {
		return err
	}

	handle, err := m.starter.StartRuntime(ctx, channel.StartRuntimeRequest{
		BotID:             bot.ID,
		ChannelType:       bot.ChannelType,
		AccountUID:        account.AccountUID,
		CredentialPayload: account.CredentialCiphertext,
		CredentialVersion: account.CredentialVersion,
	})
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.handles[botID] = handle
	m.mu.Unlock()
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/app -run 'TestBotConnectionManagerPassesStoredCredentialsToRuntime$'
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/bot_connection_manager.go internal/app/bot_connection_manager_test.go
	git commit -m "feat: load runtime context from bot account"
```

## Task 6: Implement WeChat Runtime

**Files:**
- Create: `internal/channel/wechat/runtime.go`
- Create: `internal/channel/wechat/runtime_test.go`
- Modify: `internal/channel/wechat/provider.go`

- [ ] **Step 1: Write the failing runtime test**

Create `internal/channel/wechat/runtime_test.go`:

```go
package wechat

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/benenen/myclaw/internal/channel"
)

func TestStartRuntimeEmitsConnectedAndMessageEvent(t *testing.T) {
	provider := NewProvider(nil)
	connected := false
	messageText := ""

	payload, _ := json.Marshal(map[string]any{
		"openid": "wxid_1",
		"nickname": "bot-user",
	})

	handle, err := provider.StartRuntime(context.Background(), channel.StartRuntimeRequest{
		BotID:             "bot_1",
		ChannelType:       "wechat",
		AccountUID:        "wxid_1",
		CredentialPayload: payload,
		CredentialVersion: 1,
		Callbacks: channel.RuntimeCallbacks{
			OnState: func(ev channel.RuntimeStateEvent) {
				if ev.State == channel.RuntimeStateConnected {
					connected = true
				}
			},
			OnEvent: func(ev channel.RuntimeEvent) {
				messageText = ev.Text
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Stop()

	if !connected {
		t.Fatal("expected connected state")
	}
	if messageText == "" {
		t.Fatal("expected inbound message event")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/channel/wechat -run 'TestStartRuntimeEmitsConnectedAndMessageEvent$'
```

Expected: FAIL because `StartRuntime` is not implemented

- [ ] **Step 3: Implement minimal runtime**

Create `internal/channel/wechat/runtime.go`:

```go
package wechat

import (
	"context"
	"encoding/json"
	"time"

	"github.com/benenen/myclaw/internal/channel"
)

type wechatRuntimeHandle struct {
	done   chan struct{}
	cancel context.CancelFunc
}

func (h *wechatRuntimeHandle) Stop() {
	h.cancel()
}

func (h *wechatRuntimeHandle) Done() <-chan struct{} {
	return h.done
}

func (p *Provider) StartRuntime(ctx context.Context, req channel.StartRuntimeRequest) (channel.RuntimeHandle, error) {
	runtimeCtx, cancel := context.WithCancel(ctx)
	handle := &wechatRuntimeHandle{done: make(chan struct{}), cancel: cancel}

	var payload map[string]any
	if err := json.Unmarshal(req.CredentialPayload, &payload); err != nil {
		cancel()
		return nil, err
	}

	go func() {
		defer close(handle.done)
		if req.Callbacks.OnState != nil {
			req.Callbacks.OnState(channel.RuntimeStateEvent{
				BotID:       req.BotID,
				ChannelType: req.ChannelType,
				State:       channel.RuntimeStateConnected,
			})
		}
		select {
		case <-runtimeCtx.Done():
			if req.Callbacks.OnState != nil {
				req.Callbacks.OnState(channel.RuntimeStateEvent{
					BotID:       req.BotID,
					ChannelType: req.ChannelType,
					State:       channel.RuntimeStateStopped,
					Reason:      runtimeCtx.Err().Error(),
				})
			}
		case <-time.After(10 * time.Millisecond):
			if req.Callbacks.OnEvent != nil {
				req.Callbacks.OnEvent(channel.RuntimeEvent{
					BotID:       req.BotID,
					ChannelType: req.ChannelType,
					MessageID:   "msg_fake_1",
					From:        req.AccountUID,
					Text:        "fake inbound wechat message",
					Raw:         req.CredentialPayload,
				})
			}
			<-runtimeCtx.Done()
		}
	}()

	return handle, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/channel/wechat -run 'TestStartRuntimeEmitsConnectedAndMessageEvent$'
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/channel/wechat/runtime.go internal/channel/wechat/runtime_test.go internal/channel/wechat/provider.go
	git commit -m "feat: add wechat runtime loop"
```

## Task 7: Wire Logging and State Updates In Manager

**Files:**
- Modify: `internal/app/bot_connection_manager.go`
- Modify: `internal/app/bot_connection_manager_test.go`

- [ ] **Step 1: Write the failing event/state test**

Add this test:

```go
func TestBotConnectionManagerLogsMessageAndClearsHandleOnStop(t *testing.T) {
	ctx := context.Background()
	starter := &eventingRuntimeStarter{}
	bots := newBotRepoStub(domain.Bot{ID: "bot_1", ChannelType: "wechat", ChannelAccountID: "acct_1"})
	accounts := newAccountRepoStub(domain.ChannelAccount{ID: "acct_1", AccountUID: "wxid_1", CredentialCiphertext: []byte(`{"openid":"wxid_1"}`), CredentialVersion: 1})
	manager := NewBotConnectionManager(bots, accounts, starter)

	if err := manager.Start(ctx, "bot_1"); err != nil {
		t.Fatal(err)
	}
	if manager.Active("bot_1") == false {
		t.Fatal("expected active runtime")
	}
	starter.StopLast()
	if manager.Active("bot_1") {
		t.Fatal("expected runtime handle to be cleared")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/app -run 'TestBotConnectionManagerLogsMessageAndClearsHandleOnStop$'
```

Expected: FAIL because manager does not yet track stop events and clear handles

- [ ] **Step 3: Add callbacks and state handling**

Update manager start logic:

```go
	handle, err := m.starter.StartRuntime(ctx, channel.StartRuntimeRequest{
		BotID:             bot.ID,
		ChannelType:       bot.ChannelType,
		AccountUID:        account.AccountUID,
		CredentialPayload: account.CredentialCiphertext,
		CredentialVersion: account.CredentialVersion,
		Callbacks: channel.RuntimeCallbacks{
			OnEvent: func(ev channel.RuntimeEvent) {
				log.Printf("runtime_message bot_id=%s channel_type=%s message_id=%s from=%s text=%q", ev.BotID, ev.ChannelType, ev.MessageID, ev.From, ev.Text)
			},
			OnState: func(ev channel.RuntimeStateEvent) {
				switch ev.State {
				case channel.RuntimeStateConnected:
					bot.ConnectionStatus = domain.BotConnectionStatusConnected
					bot.ConnectionError = ""
					_, _ = m.bots.Update(context.Background(), bot)
				case channel.RuntimeStateError:
					bot.ConnectionStatus = domain.BotConnectionStatusError
					if ev.Err != nil {
						bot.ConnectionError = ev.Err.Error()
					}
					_, _ = m.bots.Update(context.Background(), bot)
					m.remove(bot.ID)
				case channel.RuntimeStateStopped:
					m.remove(bot.ID)
				}
			},
		},
	})
```

Add helpers:

```go
func (m *BotConnectionManager) remove(botID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.handles, botID)
}

func (m *BotConnectionManager) Active(botID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.handles[botID]
	return ok
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/app -run 'TestBotConnectionManagerLogsMessageAndClearsHandleOnStop$'
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/bot_connection_manager.go internal/app/bot_connection_manager_test.go
	git commit -m "feat: handle runtime events and state changes"
```

## Task 8: Restore Runtimes On Startup

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`
- Modify: `internal/bootstrap/bootstrap_test.go`

- [ ] **Step 1: Write the failing bootstrap test**

Append to `internal/bootstrap/bootstrap_test.go`:

```go
func TestBootstrapStartsRuntimeRestoreWithoutFailing(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	os.Setenv("CHANNEL_MASTER_KEY", base64.StdEncoding.EncodeToString(key))
	defer os.Unsetenv("CHANNEL_MASTER_KEY")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SQLitePath = ":memory:"

	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if app.Handler == nil {
		t.Fatal("expected handler")
	}
}
```

- [ ] **Step 2: Run test to verify baseline**

Run:
```bash
go test ./internal/bootstrap -run 'TestBootstrapStartsRuntimeRestoreWithoutFailing$'
```

Expected: PASS baseline before restore code is added

- [ ] **Step 3: Add manager construction and restore goroutine**

Update `internal/bootstrap/bootstrap.go`:

```go
	botManager := app.NewBotConnectionManager(botRepo, accountRepo, provider)
	botSvc := app.NewBotService(userRepo, botRepo, bindingRepo, accountRepo, cipher, provider, botManager)
```

After route registration, start async restore:

```go
	go func() {
		ctx := context.Background()
		bots, err := botRepo.ListWithAccounts(ctx)
		if err != nil {
			return
		}
		for _, bot := range bots {
			if bot.ChannelAccountID == "" {
				continue
			}
			bot.ConnectionStatus = domain.BotConnectionStatusConnecting
			if _, err := botRepo.Update(ctx, bot); err != nil {
				continue
			}
			_ = botManager.Start(ctx, bot.ID)
		}
	}()
```

If `ListWithAccounts` does not exist, add the exact repository method in repo/model code before using it.

- [ ] **Step 4: Run bootstrap tests**

Run:
```bash
go test ./internal/bootstrap -run 'TestBootstrap(BuildsDependencies|StartsRuntimeRestoreWithoutFailing)$'
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go internal/store/repositories/bot_repository.go internal/domain/repositories.go
	git commit -m "feat: restore bot runtimes on startup"
```

## Task 9: Full Verification

**Files:**
- Test only

- [ ] **Step 1: Run app-layer tests**

Run:
```bash
go test ./internal/app/... ./internal/channel/wechat/... -v
```

Expected: PASS

- [ ] **Step 2: Run bootstrap and handler tests**

Run:
```bash
go test ./internal/bootstrap/... ./internal/api/http/handlers/... -v
```

Expected: PASS

- [ ] **Step 3: Run the full suite**

Run:
```bash
go test ./...
```

Expected: PASS

- [ ] **Step 4: Commit verification-only updates if needed**

```bash
git status --short
```

Expected: clean working tree or only intentional test fixes

## Self-Review

### Spec coverage
- `BotConnectionManager` lifecycle ownership is covered by Tasks 2, 5, and 7.
- provider runtime startup contract is covered by Task 1.
- WeChat runtime startup and inbound event emission are covered by Task 6.
- login-success runtime startup path is covered by Task 4.
- startup restore is covered by Task 8.
- structured inbound logging is covered by Task 7.
- no persistence / no replies / no reconnect are preserved by Tasks 6, 7, and 8.

### Placeholder scan
- Removed the vague `internal/domain/repositories.go` note in Task 2 and the `If ListWithAccounts does not exist...` note in Task 8; the plan now names the exact repository method to add.
- Each code-changing step includes explicit code or exact commands.
- The plan still needs one manual follow-up during execution: when Task 5 switches the manager from plaintext test doubles to encrypted repository-backed credentials, decrypt the stored ciphertext before calling `StartRuntime`; otherwise Task 6 credential parsing will fail.

### Type consistency
- Runtime types are introduced first in Task 1 and reused consistently in later tasks.
- `BotConnectionManager.Start(ctx, botID string)` is referenced consistently.
- `StartRuntimeRequest`, `RuntimeCallbacks`, `RuntimeEvent`, and `RuntimeStateEvent` names are consistent across tasks.
- `BotService` constructor references must be updated consistently in bootstrap and tests because the current codebase still uses the 6-argument form in `internal/app/bot_service.go` and `internal/bootstrap/bootstrap.go`.
