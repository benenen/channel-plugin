package repositories

import (
	"context"
	"time"

	"github.com/benenen/myclaw/internal/domain"
	"github.com/benenen/myclaw/internal/store/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	agentRunStatusPending = "pending"
	agentRunStatusDone    = "done"
)

type AgentRun struct {
	RunID       string
	BotName     string
	RuntimeType string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
}

type AgentRunRepository struct {
	db *gorm.DB
}

func NewAgentRunRepository(db *gorm.DB) *AgentRunRepository {
	return &AgentRunRepository{db: db}
}

func (r *AgentRunRepository) CreatePending(ctx context.Context, runID, botName, runtimeType string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Create(&models.AgentRun{
		RunID:       runID,
		BotName:     botName,
		RuntimeType: runtimeType,
		Status:      agentRunStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}).Error
}

func (r *AgentRunRepository) UpsertDone(ctx context.Context, runID, botName, runtimeType string) error {
	now := time.Now().UTC()
	completedAt := now
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "run_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"bot_name":     botName,
			"runtime_type": runtimeType,
			"status":       agentRunStatusDone,
			"updated_at":   now,
			"completed_at": completedAt,
		}),
	}).Create(&models.AgentRun{
		RunID:       runID,
		BotName:     botName,
		RuntimeType: runtimeType,
		Status:      agentRunStatusDone,
		CreatedAt:   now,
		UpdatedAt:   now,
		CompletedAt: &completedAt,
	}).Error
}

func (r *AgentRunRepository) GetByRunID(ctx context.Context, runID string) (AgentRun, error) {
	var m models.AgentRun
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return AgentRun{}, domain.ErrNotFound
		}
		return AgentRun{}, err
	}
	return AgentRun{
		RunID:       m.RunID,
		BotName:     m.BotName,
		RuntimeType: m.RuntimeType,
		Status:      m.Status,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
		CompletedAt: m.CompletedAt,
	}, nil
}
