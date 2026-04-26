package domain

import (
	"context"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPending Status = "PENDING"
	StatusRunning Status = "RUNNING"
	StatusSuccess Status = "SUCCESS"
	StatusFailed  Status = "FAILED"
)

type Deployment struct {
	ID              string            `json:"id"`
	AppID           string            `json:"app_id"`
	DomainName      string            `json:"domain_name"`
	RepoURL         string            `json:"repo_url"`
	Branch          string            `json:"branch"`
	BuildCommand    string            `json:"build_command"`
	TargetPort      int32             `json:"target_port"`
	EncryptedSSHKey string            `json:"-"`
	EnvVars         map[string]string `json:"env_vars,omitempty"`
	Status          Status            `json:"status"`
}

type DeploymentRepository interface {
	ClaimNextPending(ctx context.Context) (*Deployment, error)
	Save(ctx context.Context, deployment *Deployment) error
	AppendLog(ctx context.Context, deploymentID string, content string) error
	UpdateStatus(ctx context.Context, id string, status Status) error
}

type LogChunk struct {
	TraceID string `json:"trace_id"`
	Content string `json:"content"`
	IsEOF   bool   `json:"is_eof"`
}

type DeploymentStreamService interface {
	SubscribeToDeploymentLogs(ctx context.Context, traceID string, userID uuid.UUID) (<-chan LogChunk, error)
}
