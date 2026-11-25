package db

import (
	"errors"

	"gorm.io/gorm"
)

// ErrNotFound is the single not-found sentinel the service layer translates to
// HTTP 404. Leaking gorm.ErrRecordNotFound would couple handlers to GORM.
var ErrNotFound = errors.New("not found")

func normalizeErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// Actor is the authenticated caller used for tenant scoping. An admin bypasses
// the owner filter; everyone else only sees rows they own.
type Actor struct {
	UserID string
	Admin  bool
}

func (a Actor) Scope(db *gorm.DB, ownerColumn string) *gorm.DB {
	if a.Admin {
		return db
	}
	return db.Where(ownerColumn+" = ?", a.UserID)
}

func (a Actor) CanAccess(ownerID string) bool {
	return a.Admin || a.UserID == ownerID
}

type Pagination struct {
	Page     int
	PageSize int
}

func (p Pagination) Normalize() (limit, offset int) {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	return p.PageSize, (p.Page - 1) * p.PageSize
}
