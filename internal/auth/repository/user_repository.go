package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"vpsmonitoring-backend/internal/auth/models"
)

// UserRepository defines database operations for the User entity
type UserRepository interface {
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	FindByID(ctx context.Context, id int64) (*models.User, error)
	FindAllByRole(ctx context.Context, role string) ([]models.User, error)
	Create(ctx context.Context, user *models.User) error
	Update(ctx context.Context, user *models.User) error
	EmailExists(ctx context.Context, email string, excludeID int64) (bool, error)
	CountByRole(ctx context.Context, role string) (int64, error)
}

type gormUserRepository struct {
	db *gorm.DB
}

// NewUserRepository initializes and returns a GORM-based UserRepository
func NewUserRepository(db *gorm.DB) UserRepository {
	return &gormUserRepository{db: db}
}

func (r *gormUserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	result := r.db.WithContext(ctx).Where("LOWER(email) = ?", email).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &user, nil
}

func (r *gormUserRepository) FindByID(ctx context.Context, id int64) (*models.User, error) {
	var user models.User
	result := r.db.WithContext(ctx).First(&user, id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &user, nil
}

func (r *gormUserRepository) FindAllByRole(ctx context.Context, role string) ([]models.User, error) {
	var users []models.User
	result := r.db.WithContext(ctx).Where("role = ?", role).Order("id asc").Find(&users)
	if result.Error != nil {
		return nil, result.Error
	}
	return users, nil
}

func (r *gormUserRepository) Create(ctx context.Context, user *models.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *gormUserRepository) Update(ctx context.Context, user *models.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

func (r *gormUserRepository) EmailExists(ctx context.Context, email string, excludeID int64) (bool, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&models.User{}).Where("LOWER(email) = ?", email)
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *gormUserRepository) CountByRole(ctx context.Context, role string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.User{}).Where("role = ?", role).Count(&count).Error
	return count, err
}
