package db

import (
	"context"

	"cf-multi-tunnel/backend/internal/models"

	"gorm.io/gorm"
)

type TunnelRepository struct{ db *gorm.DB }

func NewTunnelRepository(db *gorm.DB) *TunnelRepository { return &TunnelRepository{db: db} }

func (r *TunnelRepository) List(ctx context.Context, actor Actor, accountID, location string) ([]models.Tunnel, error) {
	var tunnels []models.Tunnel
	q := actor.Scope(r.db.WithContext(ctx).Model(&models.Tunnel{}), "owner_id")
	if accountID != "" {
		q = q.Where("cf_account_id = ?", accountID)
	}
	if location != "" {
		q = q.Where("location = ?", location)
	}
	if err := q.Order("created_at DESC").Find(&tunnels).Error; err != nil {
		return nil, err
	}
	if err := r.fillAppCounts(ctx, tunnels); err != nil {
		return nil, err
	}
	return tunnels, nil
}

// fillAppCounts attaches the number of published apps per tunnel in one grouped
// query. The tunnels list must answer "how much is routed through this tunnel"
// without the UI issuing one request per row.
func (r *TunnelRepository) fillAppCounts(ctx context.Context, tunnels []models.Tunnel) error {
	if len(tunnels) == 0 {
		return nil
	}
	ids := make([]string, 0, len(tunnels))
	for i := range tunnels {
		ids = append(ids, tunnels[i].ID)
	}

	type row struct {
		TunnelID string
		N        int64
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Model(&models.PublishedApp{}).
		Select("tunnel_id, COUNT(*) AS n").
		Where("tunnel_id IN ?", ids).
		Group("tunnel_id").
		Scan(&rows).Error; err != nil {
		return err
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.TunnelID] = row.N
	}
	for i := range tunnels {
		tunnels[i].AppsCount = counts[tunnels[i].ID]
	}
	return nil
}

func (r *TunnelRepository) FindByID(ctx context.Context, id string) (*models.Tunnel, error) {
	var t models.Tunnel
	if err := r.db.WithContext(ctx).First(&t, "id = ?", id).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &t, nil
}

func (r *TunnelRepository) FindByRemoteID(ctx context.Context, accountID, tunnelID string) (*models.Tunnel, error) {
	var t models.Tunnel
	err := r.db.WithContext(ctx).Where("cf_account_id = ? AND tunnel_id = ?", accountID, tunnelID).First(&t).Error
	if err != nil {
		return nil, normalizeErr(err)
	}
	return &t, nil
}

func (r *TunnelRepository) ListByAccount(ctx context.Context, accountID string) ([]models.Tunnel, error) {
	var tunnels []models.Tunnel
	err := r.db.WithContext(ctx).Where("cf_account_id = ?", accountID).Order("created_at DESC").Find(&tunnels).Error
	return tunnels, err
}

func (r *TunnelRepository) ListManaged(ctx context.Context) ([]models.Tunnel, error) {
	var tunnels []models.Tunnel
	err := r.db.WithContext(ctx).
		Where("location = ? AND connector_mode = ? AND desired_running = ?", "local", "docker", true).
		Find(&tunnels).Error
	return tunnels, err
}

func (r *TunnelRepository) CountByLocation(ctx context.Context, actor Actor, location string) (int64, error) {
	var n int64
	q := actor.Scope(r.db.WithContext(ctx).Model(&models.Tunnel{}), "owner_id")
	return n, q.Where("location = ?", location).Count(&n).Error
}

func (r *TunnelRepository) CountByLocationStatus(ctx context.Context, actor Actor, location, status string) (int64, error) {
	var n int64
	q := actor.Scope(r.db.WithContext(ctx).Model(&models.Tunnel{}), "owner_id")
	return n, q.Where("location = ? AND status = ?", location, status).Count(&n).Error
}

func (r *TunnelRepository) Count(ctx context.Context, actor Actor) (int64, error) {
	var n int64
	q := actor.Scope(r.db.WithContext(ctx).Model(&models.Tunnel{}), "owner_id")
	return n, q.Count(&n).Error
}

func (r *TunnelRepository) CountByStatus(ctx context.Context, actor Actor, status string) (int64, error) {
	var n int64
	q := actor.Scope(r.db.WithContext(ctx).Model(&models.Tunnel{}), "owner_id")
	return n, q.Where("status = ?", status).Count(&n).Error
}

func (r *TunnelRepository) Create(ctx context.Context, t *models.Tunnel) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *TunnelRepository) Update(ctx context.Context, t *models.Tunnel) error {
	return r.db.WithContext(ctx).Save(t).Error
}

func (r *TunnelRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tunnel_id = ?", id).Delete(&models.PublishedApp{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Tunnel{}, "id = ?", id).Error
	})
}
