// Package models contains domain entities and business models for the authentication system
package models

import (
	"time"

	"github.com/google/uuid"
)

type Admin struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UUID         uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uk_admins_uuid;index:idx_admins_uuid" json:"uuid"`
	Username     string    `gorm:"size:255;not null;uniqueIndex:uk_admins_username;index:idx_admins_username" json:"username"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`

	IsActive    *bool      `gorm:"default:true;index:idx_admins_is_active" json:"is_active"`
	CreatedAt   time.Time  `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC');index:idx_admins_created_at" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')" json:"updated_at"`
	LastLoginAt *time.Time `gorm:"index:idx_admins_last_login_at" json:"last_login_at,omitempty"`
}

func (Admin) TableName() string {
	return "admins"
}

// AdminFilter represents filter criteria for admin queries
type AdminFilter struct {
	ID              *uint
	UUID            *uuid.UUID
	Username        *string
	IsActive        *bool
	CreatedAfter    *time.Time
	CreatedBefore   *time.Time
	LastLoginAfter  *time.Time
	LastLoginBefore *time.Time
}
