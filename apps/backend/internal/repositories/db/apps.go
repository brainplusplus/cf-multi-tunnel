package db

import (
	"context"

	"cf-multi-tunnel/backend/internal/models"

	"gorm.io/gorm"
)

type AppRepository struct{ db *gorm.DB }

func NewAppRepository(db *gorm.DB) *AppRepository { return &AppRepository{db: db} }

func (r *AppRepository) ListByTunnel(ctx context.Context, tunnelID string) ([]models.PublishedApp, error) {
	var apps []models.PublishedApp
	err := r.db.WithContext(ctx).Where("tunnel_id = ?", tunnelID).Order("hostname ASC").Find(&apps).Error
	return apps, err
}

func (r *AppRepository) ListEnabledByTunnel(ctx context.Context, tunnelID string) ([]models.PublishedApp, error) {
	var apps []models.PublishedApp
	err := r.db.WithContext(ctx).Where("tunnel_id = ? AND enabled = ?", tunnelID, true).
		Order("hostname ASC").Find(&apps).Error
	return apps, err
}

func (r *AppRepository) List(ctx context.Context, actor Actor) ([]models.PublishedApp, error) {
	var apps []models.PublishedApp
	q := actor.Scope(r.db.WithContext(ctx).Model(&models.PublishedApp{}), "owner_id")
	if err := q.Order("created_at DESC").Find(&apps).Error; err != nil {
		return nil, err
	}
	return apps, nil
}

func (r *AppRepository) FindByID(ctx context.Context, id string) (*models.PublishedApp, error) {
	var a models.PublishedApp
	if err := r.db.WithContext(ctx).First(&a, "id = ?", id).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &a, nil
}

func (r *AppRepository) FindByHostname(ctx context.Context, hostname string) (*models.PublishedApp, error) {
	var a models.PublishedApp
	if err := r.db.WithContext(ctx).Where("hostname = ?", hostname).First(&a).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &a, nil
}

func (r *AppRepository) Count(ctx context.Context, actor Actor) (int64, error) {
	var n int64
	q := actor.Scope(r.db.WithContext(ctx).Model(&models.PublishedApp{}), "owner_id")
	return n, q.Count(&n).Error
}

func (r *AppRepository) Create(ctx context.Context, a *models.PublishedApp) error {
	// manage_dns has a SQL default of true. GORM omits zero-value fields on
	// INSERT and then writes the default back into the struct, so an explicit
	// false would silently become true. Capture the intent first and write it
	// back afterwards, or "imported, DNS is not ours" would start managing DNS.
	manageDNS := a.ManageDNS
	if err := r.db.WithContext(ctx).Create(a).Error; err != nil {
		return err
	}
	a.ManageDNS = manageDNS
	return r.db.WithContext(ctx).Model(a).UpdateColumn("manage_dns", manageDNS).Error
}

func (r *AppRepository) Update(ctx context.Context, a *models.PublishedApp) error {
	return r.db.WithContext(ctx).Save(a).Error
}

func (r *AppRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.PublishedApp{}, "id = ?", id).Error
}

type AuditRepository struct{ db *gorm.DB }

func NewAuditRepository(db *gorm.DB) *AuditRepository { return &AuditRepository{db: db} }

func (r *AuditRepository) Create(ctx context.Context, l *models.AuditLog) error {
	return r.db.WithContext(ctx).Create(l).Error
}

func (r *AuditRepository) List(ctx context.Context, p Pagination) ([]models.AuditLog, int64, error) {
	limit, offset := p.Normalize()
	q := r.db.WithContext(ctx).Model(&models.AuditLog{})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []models.AuditLog
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}
