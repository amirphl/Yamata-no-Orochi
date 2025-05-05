// Package repository provides data access layer implementations and interfaces for database operations
package repository

import (
	"context"
	"fmt"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"gorm.io/gorm"
)

// AccountTypeRepositoryImpl implements AccountTypeRepository interface
type AccountTypeRepositoryImpl struct {
	*BaseRepository[models.AccountType, models.AccountType]
}

// NewAccountTypeRepository creates a new account type repository
func NewAccountTypeRepository(db *gorm.DB) AccountTypeRepository {
	return &AccountTypeRepositoryImpl{
		BaseRepository: NewBaseRepository[models.AccountType, models.AccountType](db),
	}
}

// ByTypeName retrieves an account type by its type name
func (r *AccountTypeRepositoryImpl) ByTypeName(ctx context.Context, typeName string) (*models.AccountType, error) {
	db := r.getDB(ctx)

	var accountType models.AccountType
	err := db.Where("type_name = ?", typeName).
		Last(&accountType).Error

	if err != nil {
		if err.Error() == "record not found" { // GORM error check
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find account type by name: %w", err)
	}

	return &accountType, nil
}
