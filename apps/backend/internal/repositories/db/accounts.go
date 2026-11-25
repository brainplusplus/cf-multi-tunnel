package db

import (
	"context"

	"cf-multi-tunnel/backend/internal/models"

	"gorm.io/gorm"
)

type AccountRepository struct{ db *gorm.DB }

func NewAccountRepository(db *gorm.DB) *AccountRepository { return &AccountRepository{db: db} }

func (r *AccountRepository) List(ctx context.Context, actor Actor) ([]models.CFAccount, error) {
	var accounts []models.CFAccount
	q := actor.Scope(r.db.WithContext(ctx).Model(&models.CFAccount{}), "owner_id")
	if err := q.Order("created_at DESC").Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *AccountRepository) FindByID(ctx context.Context, id string) (*models.CFAccount, error) {
	var a models.CFAccount
	if err := r.db.WithContext(ctx).First(&a, "id = ?", id).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &a, nil
}

func (r *AccountRepository) FindByOwnerAndAccountID(ctx context.Context, ownerID, accountID string) (*models.CFAccount, error) {
	var a models.CFAccount
	err := r.db.WithContext(ctx).Where("owner_id = ? AND account_id = ?", ownerID, accountID).First(&a).Error
	if err != nil {
		return nil, normalizeErr(err)
	}
	return &a, nil
}

func (r *AccountRepository) Count(ctx context.Context, actor Actor) (int64, error) {
	var n int64
	q := actor.Scope(r.db.WithContext(ctx).Model(&models.CFAccount{}), "owner_id")
	return n, q.Count(&n).Error
}

func (r *AccountRepository) Create(ctx context.Context, a *models.CFAccount) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *AccountRepository) Update(ctx context.Context, a *models.CFAccount) error {
	return r.db.WithContext(ctx).Save(a).Error
}

func (r *AccountRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tunnelIDs []string
		if err := tx.Model(&models.Tunnel{}).Where("cf_account_id = ?", id).Pluck("id", &tunnelIDs).Error; err != nil {
			return err
		}
		if len(tunnelIDs) > 0 {
			if err := tx.Where("tunnel_id IN ?", tunnelIDs).Delete(&models.PublishedApp{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("cf_account_id = ?", id).Delete(&models.Tunnel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("cf_account_id = ?", id).Delete(&models.CFZone{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.CFAccount{}, "id = ?", id).Error
	})
}

type ZoneRepository struct{ db *gorm.DB }

func NewZoneRepository(db *gorm.DB) *ZoneRepository { return &ZoneRepository{db: db} }

func (r *ZoneRepository) ListByAccount(ctx context.Context, accountID string) ([]models.CFZone, error) {
	var zones []models.CFZone
	err := r.db.WithContext(ctx).Where("cf_account_id = ?", accountID).Order("name ASC").Find(&zones).Error
	return zones, err
}

func (r *ZoneRepository) ReplaceForAccount(ctx context.Context, accountID string, zones []models.CFZone) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("cf_account_id = ?", accountID).Delete(&models.CFZone{}).Error; err != nil {
			return err
		}
		if len(zones) == 0 {
			return nil
		}
		return tx.Create(&zones).Error
	})
}
