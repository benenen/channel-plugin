package wechat

import (
	"context"
	"encoding/json"
	"fmt"
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
		return nil, fmt.Errorf("unmarshal runtime credential payload: %w", err)
	}

	if req.Callbacks.OnState != nil {
		req.Callbacks.OnState(channel.RuntimeStateEvent{
			BotID:       req.BotID,
			ChannelType: req.ChannelType,
			State:       channel.RuntimeStateConnected,
		})
	}

	go func() {
		defer close(handle.done)

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
			if req.Callbacks.OnState != nil {
				req.Callbacks.OnState(channel.RuntimeStateEvent{
					BotID:       req.BotID,
					ChannelType: req.ChannelType,
					State:       channel.RuntimeStateStopped,
					Reason:      runtimeCtx.Err().Error(),
				})
			}
		}
	}()

	_ = payload
	return handle, nil
}

var _ channel.RuntimeStarter = (*Provider)(nil)
