// Package repository provides data access layer implementations and interfaces for database operations
package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// BaseRepository provides common repository functionality with transaction support
type BaseRepository[T any, F any] struct {
	DB *gorm.DB
}

// NewBaseRepository creates a new base repository instance
func NewBaseRepository[T any, F any](db *gorm.DB) *BaseRepository[T, F] {
	return &BaseRepository[T, F]{
		DB: db,
	}
}

// getDB returns the appropriate database connection (with or without transaction)
func (r *BaseRepository[T, F]) getDB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(TxContextKey).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return r.DB
}

// getDBForWrite returns database connection with transaction for write operations
func (r *BaseRepository[T, F]) getDBForWrite(ctx context.Context) (*gorm.DB, bool, error) {
	if tx, ok := ctx.Value(TxContextKey).(*gorm.DB); ok && tx != nil {
		return tx, false, nil // Transaction already exists, don't commit
	}

	// Start new transaction for write operation
	tx := r.DB.Begin()
	if tx.Error != nil {
		return nil, false, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	return tx, true, nil // New transaction, should commit
}

// ByID retrieves an entity by its ID
func (r *BaseRepository[T, F]) ByID(ctx context.Context, id uint) (*T, error) {
	db := r.getDB(ctx)

	var entity T
	err := db.Last(&entity, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find entity by ID %d: %w", id, err)
	}

	return &entity, nil
}

// ByFilter retrieves entities based on filter criteria
// func (r *BaseRepository[T, F]) ByFilter(ctx context.Context, filter F) ([]*T, error) {
// 	db := r.getDB(ctx)

// 	var entities []*T
// 	query := r.applyFilter(db, filter)

// 	err := query.Find(&entities).Error
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to find entities by filter: %w", err)
// 	}

// 	return entities, nil
// }

// Save inserts a new entity
func (r *BaseRepository[T, F]) Save(ctx context.Context, entity *T) error {
	db, shouldCommit, err := r.getDBForWrite(ctx)
	if err != nil {
		return err
	}

	if shouldCommit {
		defer func() {
			if err != nil {
				db.Rollback()
			} else {
				db.Commit()
			}
		}()
	}

	err = db.Create(entity).Error
	if err != nil {
		return fmt.Errorf("failed to save entity: %w", err)
	}

	return nil
}

// SaveBatch inserts multiple entities in a single transaction
func (r *BaseRepository[T, F]) SaveBatch(ctx context.Context, entities []*T) error {
	if len(entities) == 0 {
		return nil
	}

	db, shouldCommit, err := r.getDBForWrite(ctx)
	if err != nil {
		return err
	}

	if shouldCommit {
		defer func() {
			if err != nil {
				db.Rollback()
			} else {
				db.Commit()
			}
		}()
	}

	err = db.CreateInBatches(entities, 100).Error // Batch size of 100
	if err != nil {
		return fmt.Errorf("failed to save batch entities: %w", err)
	}

	return nil
}

// Count returns the number of entities matching the filter
// func (r *BaseRepository[T, F]) Count(ctx context.Context, filter F) (int64, error) {
// 	db := r.getDB(ctx)

// 	var count int64
// 	var entity T
// 	query := r.applyFilter(db.Model(&entity), filter)

// 	err := query.Count(&count).Error
// 	if err != nil {
// 		return 0, fmt.Errorf("failed to count entities: %w", err)
// 	}

// 	return count, nil
// }

// Exists checks if any entity matching the filter exists
// func (r *BaseRepository[T, F]) Exists(ctx context.Context, filter F) (bool, error) {
// 	count, err := r.Count(ctx, filter)
// 	if err != nil {
// 		return false, err
// 	}

// 	return count > 0, nil
// }

// applyFilter applies filter conditions to the GORM query
// func (r *BaseRepository[T, F]) applyFilter(db *gorm.DB, filter F) *gorm.DB {
// 	filterValue := reflect.ValueOf(filter)
// 	filterType := reflect.TypeOf(filter)

// 	// Handle pointer to struct
// 	if filterType.Kind() == reflect.Ptr {
// 		if filterValue.IsNil() {
// 			return db
// 		}
// 		filterValue = filterValue.Elem()
// 		filterType = filterType.Elem()
// 	}

// 	// Apply filters based on field names and values
// 	for i := 0; i < filterValue.NumField(); i++ {
// 		field := filterValue.Field(i)
// 		fieldType := filterType.Field(i)

// 		if !field.IsValid() || field.IsZero() {
// 			continue
// 		}

// 		// Handle pointer fields
// 		if field.Kind() == reflect.Ptr {
// 			if field.IsNil() {
// 				continue
// 			}
// 			field = field.Elem()
// 		}

// 		fieldName := fieldType.Name
// 		dbColumnName := r.getDBColumnName(fieldName)

// 		// Apply specific filter logic based on field type and name
// 		switch fieldName {
// 		case "CreatedAfter":
// 			if t, ok := field.Interface().(time.Time); ok {
// 				db = db.Where("created_at >= ?", t)
// 			}
// 		case "CreatedBefore":
// 			if t, ok := field.Interface().(time.Time); ok {
// 				db = db.Where("created_at <= ?", t)
// 			}
// 		case "LastLoginAfter":
// 			if t, ok := field.Interface().(time.Time); ok {
// 				db = db.Where("last_login_at >= ?", t)
// 			}
// 		case "LastLoginBefore":
// 			if t, ok := field.Interface().(time.Time); ok {
// 				db = db.Where("last_login_at <= ?", t)
// 			}
// 		case "ExpiresAfter":
// 			if t, ok := field.Interface().(time.Time); ok {
// 				db = db.Where("expires_at >= ?", t)
// 			}
// 		case "ExpiresBefore":
// 			if t, ok := field.Interface().(time.Time); ok {
// 				db = db.Where("expires_at <= ?", t)
// 			}
// 		case "AccessedAfter":
// 			if t, ok := field.Interface().(time.Time); ok {
// 				db = db.Where("last_accessed_at >= ?", t)
// 			}
// 		case "AccessedBefore":
// 			if t, ok := field.Interface().(time.Time); ok {
// 				db = db.Where("last_accessed_at <= ?", t)
// 			}
// 		case "IsActive":
// 			if field.Kind() == reflect.Bool {
// 				// Special handling for active filter in OTP verifications
// 				if field.Bool() {
// 					// Check if we're dealing with OTP table by looking for status field
// 					var testEntity T
// 					entityType := reflect.TypeOf(testEntity)
// 					if entityType.Kind() == reflect.Ptr {
// 						entityType = entityType.Elem()
// 					}
// 					if entityType.Name() == "OTPVerification" {
// 						db = db.Where("status = ? AND expires_at > ?", "pending", time.Now())
// 					} else {
// 						db = db.Where(fmt.Sprintf("%s = ?", dbColumnName), field.Interface())
// 					}
// 				} else {
// 					db = db.Where(fmt.Sprintf("%s = ?", dbColumnName), field.Interface())
// 				}
// 			} else {
// 				db = db.Where(fmt.Sprintf("%s = ?", dbColumnName), field.Interface())
// 			}
// 		case "IsExpired":
// 			if field.Kind() == reflect.Bool && field.Bool() {
// 				db = db.Where("expires_at <= ?", time.Now())
// 			}
// 		case "AccountTypeName":
// 			// Join with account_types table for type name filtering
// 			db = db.Joins("JOIN account_types ON customers.account_type_id = account_types.id").
// 				Where("account_types.type_name = ?", field.Interface())
// 		default:
// 			// Standard equality filter
// 			db = db.Where(fmt.Sprintf("%s = ?", dbColumnName), field.Interface())
// 		}
// 	}

// 	return db
// }

// getDBColumnName converts Go field name to database column name (snake_case)
// func (r *BaseRepository[T, F]) getDBColumnName(fieldName string) string {
// 	// Common abbreviations that should be treated as single units
// 	abbreviations := map[string]string{
// 		"ID":   "id",
// 		"URL":  "url",
// 		"API":  "api",
// 		"HTTP": "http",
// 		"JSON": "json",
// 		"XML":  "xml",
// 		"HTML": "html",
// 		"CSS":  "css",
// 		"SQL":  "sql",
// 		"JWT":  "jwt",
// 		"OTP":  "otp",
// 	}

// 	// Check if the entire field name is an abbreviation
// 	if abbr, exists := abbreviations[fieldName]; exists {
// 		return abbr
// 	}

// 	// Convert PascalCase to snake_case, handling abbreviations
// 	var result []rune
// 	for i := 0; i < len(fieldName); i++ {
// 		r := rune(fieldName[i])

// 		// Check if we have a potential abbreviation (2+ consecutive uppercase letters)
// 		if r >= 'A' && r <= 'Z' {
// 			// Look ahead to see if we have consecutive uppercase letters
// 			abbrEnd := i
// 			for j := i + 1; j < len(fieldName) && fieldName[j] >= 'A' && fieldName[j] <= 'Z'; j++ {
// 				abbrEnd = j
// 			}

// 			// If we found consecutive uppercase letters, treat as abbreviation
// 			if abbrEnd > i {
// 				abbr := fieldName[i : abbrEnd+1]
// 				if abbrLower, exists := abbreviations[abbr]; exists {
// 					// Add underscore before abbreviation if not at start
// 					if i > 0 {
// 						result = append(result, '_')
// 					}
// 					result = append(result, []rune(abbrLower)...)
// 					i = abbrEnd // Skip the processed characters
// 					continue
// 				}
// 			}

// 			// Single uppercase letter - add underscore and convert to lowercase
// 			if i > 0 {
// 				result = append(result, '_')
// 			}
// 			result = append(result, r+32) // Convert to lowercase
// 		} else {
// 			// Lowercase letter - just add it
// 			result = append(result, r)
// 		}
// 	}

// 	return string(result)
// }

// WithTransaction executes a function within a database transaction
func WithTransaction(ctx context.Context, db *gorm.DB, fn func(context.Context) error) (err error) {
	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			err = fmt.Errorf("panic in transaction: %v", r)
		}
	}()

	ctx = context.WithValue(ctx, TxContextKey, tx)

	if err := fn(ctx); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
