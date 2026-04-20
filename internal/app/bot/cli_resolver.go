package bot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/benenen/myclaw/internal/agent"
	"github.com/benenen/myclaw/internal/domain"
)

var (
	ErrBotCLIConfigMissing   = errors.New("bot cli config missing")
	ErrBotCLIUnavailable     = errors.New("bot cli unavailable")
	ErrBotCLIUnsupportedMode = errors.New("bot cli mode unsupported")
)

type BotCLIResolverConfig struct {
	Timeout       time.Duration
	WorkspaceRoot string
	SQLitePath    string
}

type BotCLIResolver struct {
	bots          domain.BotRepository
	capabilities  domain.AgentCapabilityRepository
	timeout       time.Duration
	workspaceRoot string
	sqlitePath    string
}

func NewBotCLIResolver(bots domain.BotRepository, capabilities domain.AgentCapabilityRepository, cfg BotCLIResolverConfig) *BotCLIResolver {
	return &BotCLIResolver{
		bots:          bots,
		capabilities:  capabilities,
		timeout:       cfg.Timeout,
		workspaceRoot: cfg.WorkspaceRoot,
		sqlitePath:    cfg.SQLitePath,
	}
}

func (r *BotCLIResolver) Resolve(ctx context.Context, botID string) (agent.Spec, error) {
	bot, err := r.bots.GetByID(ctx, botID)
	if err != nil {
		return agent.Spec{}, err
	}
	if bot.AgentCapabilityID == "" || bot.AgentMode == "" {
		return agent.Spec{}, ErrBotCLIConfigMissing
	}
	capability, err := r.capabilities.GetByID(ctx, bot.AgentCapabilityID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return agent.Spec{}, ErrBotCLIConfigMissing
		}
		return agent.Spec{}, err
	}
	if !capability.Available {
		return agent.Spec{}, ErrBotCLIUnavailable
	}
	if !slices.Contains(capability.SupportedModes, bot.AgentMode) {
		return agent.Spec{}, ErrBotCLIUnsupportedMode
	}
	if capability.Command == "" {
		return agent.Spec{}, ErrBotCLIConfigMissing
	}
	spec := agent.Spec{
		BotID:      botID,
		BotName:    bot.Name,
		Type:       bot.AgentMode,
		Command:    capability.Command,
		Args:       append([]string(nil), capability.Args...),
		Timeout:    r.timeoutForMode(bot.AgentMode),
		SQLitePath: r.sqlitePath,
	}
	if r.workspaceRoot != "" {
		spec.WorkDir = filepath.Join(r.workspaceRoot, botID, "workspace")
		if err := os.MkdirAll(spec.WorkDir, 0o755); err != nil {
			return agent.Spec{}, err
		}
	}
	return spec, nil
}

func (r *BotCLIResolver) timeoutForMode(mode string) time.Duration {
	return r.timeout
}
