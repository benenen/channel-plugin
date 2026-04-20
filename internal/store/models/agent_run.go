package models

import "time"

type AgentRun struct {
	RunID       string     `gorm:"column:run_id;primaryKey"`
	BotName     string     `gorm:"column:bot_name;not null"`
	RuntimeType string     `gorm:"column:runtime_type;not null"`
	Status      string     `gorm:"not null"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
	CompletedAt *time.Time `gorm:"column:completed_at"`
}

func (AgentRun) TableName() string {
	return "agent_runs"
}
