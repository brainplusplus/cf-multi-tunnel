package services

import (
	"context"
	"log/slog"

	"cf-multi-tunnel/backend/internal/models"
	db "cf-multi-tunnel/backend/internal/repositories/db"
)

// AuditService records who changed what. Failures are logged, never returned:
// an audit write must not roll back the user's action, but it must be visible.
type AuditService struct {
	repo *db.AuditRepository
}

func NewAuditService(repo *db.AuditRepository) *AuditService {
	return &AuditService{repo: repo}
}

type AuditEntry struct {
	ActorID    string
	ActorEmail string
	Action     string
	Entity     string
	EntityID   string
	Detail     string
	IP         string
}

func (s *AuditService) Record(ctx context.Context, e AuditEntry) {
	if err := s.repo.Create(ctx, &models.AuditLog{
		ActorID:    e.ActorID,
		ActorEmail: e.ActorEmail,
		Action:     e.Action,
		Entity:     e.Entity,
		EntityID:   e.EntityID,
		Detail:     e.Detail,
		IP:         e.IP,
	}); err != nil {
		slog.Error("failed to write audit log", "action", e.Action, "error", err)
	}
}

func (s *AuditService) List(ctx context.Context, p db.Pagination) ([]models.AuditLog, int64, error) {
	return s.repo.List(ctx, p)
}
