package models

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PlatformSettingsStatus represents platform settings status.
type PlatformSettingsStatus string

const (
	PlatformSettingsStatusInitialized PlatformSettingsStatus = "initialized"
	PlatformSettingsStatusInProgress  PlatformSettingsStatus = "in-progress"
	PlatformSettingsStatusActive      PlatformSettingsStatus = "active"
	PlatformSettingsStatusInactive    PlatformSettingsStatus = "inactive"
)

// Valid checks if the status is valid.
func (s PlatformSettingsStatus) Valid() bool {
	switch s {
	case PlatformSettingsStatusInitialized,
		PlatformSettingsStatusInProgress,
		PlatformSettingsStatusActive,
		PlatformSettingsStatusInactive:
		return true
	default:
		return false
	}
}

// Scan implements the sql.Scanner interface for PlatformSettingsStatus.
func (s *PlatformSettingsStatus) Scan(value any) error {
	if value == nil {
		*s = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*s = PlatformSettingsStatus(v)
	case []byte:
		*s = PlatformSettingsStatus(string(v))
	default:
		return fmt.Errorf("cannot scan %T into PlatformSettingsStatus", value)
	}

	return nil
}

// Value implements the driver.Valuer interface for PlatformSettingsStatus.
func (s PlatformSettingsStatus) Value() (driver.Value, error) {
	if !s.Valid() {
		return nil, fmt.Errorf("invalid PlatformSettingsStatus: %s", s)
	}
	return string(s), nil
}

// PlatformSettings represents platform settings with optional multimedia attachment.
type PlatformSettings struct {
	ID           uint                  `gorm:"primaryKey;autoIncrement" json:"id"`
	UUID         uuid.UUID             `gorm:"type:uuid;uniqueIndex;not null;default:gen_random_uuid()" json:"uuid"`
	CustomerID   uint                  `gorm:"not null;index" json:"customer_id"`
	Platform     string                `gorm:"type:varchar(20);not null;index" json:"platform"`
	Name         *string               `gorm:"type:varchar(255)" json:"name,omitempty"`
	Description  *string               `gorm:"type:text" json:"description,omitempty"`
	MultimediaID *uint                 `gorm:"index" json:"multimedia_id,omitempty"`
	Status       PlatformSettingsStatus `gorm:"type:varchar(20);not null;default:'initialized';index" json:"status"`
	CreatedAt    time.Time             `gorm:"not null;default:CURRENT_TIMESTAMP;index" json:"created_at"`
	UpdatedAt    time.Time             `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	Multimedia *MultimediaAsset `gorm:"foreignKey:MultimediaID;references:ID;constraint:OnDelete:SET NULL" json:"multimedia,omitempty"`
	Customer   *Customer        `gorm:"foreignKey:CustomerID;references:ID;constraint:OnDelete:CASCADE" json:"customer,omitempty"`
}

func (PlatformSettings) TableName() string { return "platform_settings" }

// BeforeCreate ensures UUID and timestamps are set.
func (p *PlatformSettings) BeforeCreate(tx *gorm.DB) error {
	if p.UUID == uuid.Nil {
		p.UUID = uuid.New()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = utils.UTCNow()
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = utils.UTCNow()
	}
	return nil
}

// PlatformSettingsFilter represents filter criteria for platform settings queries.
type PlatformSettingsFilter struct {
	ID            *uint                  `json:"id,omitempty"`
	UUID          *uuid.UUID             `json:"uuid,omitempty"`
	CustomerID    *uint                  `json:"customer_id,omitempty"`
	Platform      *string                `json:"platform,omitempty"`
	Status        *PlatformSettingsStatus `json:"status,omitempty"`
	MultimediaID  *uint                  `json:"multimedia_id,omitempty"`
	CreatedAfter  *time.Time             `json:"created_after,omitempty"`
	CreatedBefore *time.Time             `json:"created_before,omitempty"`
}
