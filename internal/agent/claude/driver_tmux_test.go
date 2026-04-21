package claude

import (
	"context"

	"github.com/benenen/myclaw/internal/agent"
)

// mockTMUXPane is a mock implementation of tmuxPane.
type mockTMUXPane struct {
	sendKeysFunc    func(keys ...string) error
	capturePaneFunc func() (string, error)
}

func (m *mockTMUXPane) SendKeys(keys ...string) error {
	if m.sendKeysFunc != nil {
		return m.sendKeysFunc(keys...)
	}
	return nil
}

func (m *mockTMUXPane) CapturePane() (string, error) {
	if m.capturePaneFunc != nil {
		return m.capturePaneFunc()
	}
	return "", nil
}

// mockTMUXSession is a mock implementation of tmuxSession.
type mockTMUXSession struct {
	killFunc func() error
}

func (m *mockTMUXSession) Kill() error {
	if m.killFunc != nil {
		return m.killFunc()
	}
	return nil
}

// mockTMUXRuntimeFactory is a mock implementation of tmuxRuntimeFactory.
type mockTMUXRuntimeFactory struct {
	startFunc func(ctx context.Context, spec agent.Spec, sessionName string) (tmuxSession, tmuxPane, error)
}

func (m *mockTMUXRuntimeFactory) Start(ctx context.Context, spec agent.Spec, sessionName string) (tmuxSession, tmuxPane, error) {
	if m.startFunc != nil {
		return m.startFunc(ctx, spec, sessionName)
	}
	return &mockTMUXSession{}, &mockTMUXPane{}, nil
}

// mockTMUXRunStore is a mock implementation of tmuxRunStore.
type mockTMUXRunStore struct {
	createPendingFunc func(ctx context.Context, runID, botName, runtimeType string) error
	upsertDoneFunc    func(ctx context.Context, runID, botName, runtimeType string) error
	getByRunIDFunc    func(ctx context.Context, runID string) (tmuxRunRecord, error)
}

func (m *mockTMUXRunStore) CreatePending(ctx context.Context, runID, botName, runtimeType string) error {
	if m.createPendingFunc != nil {
		return m.createPendingFunc(ctx, runID, botName, runtimeType)
	}
	return nil
}

func (m *mockTMUXRunStore) UpsertDone(ctx context.Context, runID, botName, runtimeType string) error {
	if m.upsertDoneFunc != nil {
		return m.upsertDoneFunc(ctx, runID, botName, runtimeType)
	}
	return nil
}

func (m *mockTMUXRunStore) GetByRunID(ctx context.Context, runID string) (tmuxRunRecord, error) {
	if m.getByRunIDFunc != nil {
		return m.getByRunIDFunc(ctx, runID)
	}
	return tmuxRunRecord{}, nil
}

// mockTMUXRunStoreFactory is a mock implementation of tmuxRunStoreFactory.
type mockTMUXRunStoreFactory struct {
	openFunc func(spec agent.Spec) (tmuxRunStore, error)
}

func (m *mockTMUXRunStoreFactory) Open(spec agent.Spec) (tmuxRunStore, error) {
	if m.openFunc != nil {
		return m.openFunc(spec)
	}
	return &mockTMUXRunStore{}, nil
}
