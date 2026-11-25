package db

import (
	"context"
	"time"

	"cf-multi-tunnel/backend/internal/models"

	"gorm.io/gorm"
)

type SettingRepository struct{ db *gorm.DB }

func NewSettingRepository(db *gorm.DB) *SettingRepository { return &SettingRepository{db: db} }

func (r *SettingRepository) Get(ctx context.Context, key string) (string, error) {
	var s models.Setting
	if err := r.db.WithContext(ctx).First(&s, "key = ?", key).Error; err != nil {
		return "", normalizeErr(err)
	}
	return s.Value, nil
}

func (r *SettingRepository) Set(ctx context.Context, key, value string) error {
	return r.db.WithContext(ctx).Save(&models.Setting{
		Key:       key,
		Value:     value,
		UpdatedAt: time.Now().UTC(),
	}).Error
}

func (r *SettingRepository) Delete(ctx context.Context, key string) error {
	return r.db.WithContext(ctx).Delete(&models.Setting{}, "key = ?", key).Error
}
