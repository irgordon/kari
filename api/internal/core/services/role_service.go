package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
)

type RoleService struct {
	repo   domain.UserRepository
	logger *slog.Logger
}

func NewRoleService(repo domain.UserRepository, logger *slog.Logger) *RoleService {
	return &RoleService{
		repo:   repo,
		logger: logger,
	}
}

// AssignRole changes a user's role while enforcing Rank-based security boundaries.
func (s *RoleService) AssignRole(ctx context.Context, actorID uuid.UUID, targetUserID uuid.UUID, newRoleID uuid.UUID) error {
	// 1. Fetch the Actor (The person performing the change)
	actor, err := s.repo.GetByID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("failed to fetch actor: %w", err)
	}

	// 2. Fetch the Target Role metadata
	targetRole, err := s.repo.GetRoleByID(ctx, newRoleID)
	if err != nil {
		return fmt.Errorf("target role not found: %w", err)
	}

	// 🛡️ 3. SLA Boundary: Rank Check (Privilege Escalation Prevention)
	// In Kari, lower numbers = higher power (0 is SuperUser).
	// An actor cannot assign a role with a rank superior to their own.
	if targetRole.Rank < actor.Role.Rank {
		s.logger.Warn("Escalation attempt blocked",
			slog.String("actor", actor.Email),
			slog.String("attempted_rank", fmt.Sprintf("%d", targetRole.Rank)))
		return errors.New("forbidden: cannot assign a role superior to your own rank")
	}

	// 🛡️ 4. Zero-Trust: "Last Admin" Protection
	// If the target user is the last Rank 0 admin, prevent them from being demoted.
	targetUser, _ := s.repo.GetByID(ctx, targetUserID)
	if targetUser.Role.Rank == 0 && targetRole.Rank > 0 {
		count, _ := s.repo.CountAdmins(ctx)
		if count <= 1 {
			return errors.New("forbidden: cannot demote the last system administrator")
		}
	}

	// 5. Execute Assignment
	return s.repo.UpdateUserRole(ctx, targetUserID, newRoleID)
}
