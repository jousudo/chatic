// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package repository

import (
	"chatic/internal/model"

	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// GetByNumber looks up a user by WhatsApp number.
func (r *UserRepository) GetByNumber(numero string) (*model.User, error) {
	var user model.User
	err := r.db.Where("numero_wpp = ?", numero).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByID looks up a user by primary ID.
func (r *UserRepository) GetByID(id uint) (*model.User, error) {
	var user model.User
	err := r.db.First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Create inserts a new user. If a soft-deleted record with the same PhoneNumber
// already exists, it restores that record instead of inserting a new one — without
// this the UNIQUE constraint on numero_wpp would reject the insert, since the old
// row still physically occupies that number even while "deleted".
func (r *UserRepository) Create(user *model.User) error {
	var existing model.User
	err := r.db.Unscoped().Where("numero_wpp = ?", user.PhoneNumber).First(&existing).Error
	if err == nil {
		if !existing.DeletedAt.Valid {
			return gorm.ErrDuplicatedKey
		}
		user.ID = existing.ID
		user.CreatedAt = existing.CreatedAt
		return r.db.Unscoped().Save(user).Error
	}
	return r.db.Create(user).Error
}

// Update persists changes to a user's profile.
func (r *UserRepository) Update(user *model.User) error {
	return r.db.Save(user).Error
}

// Delete removes a user using GORM's soft delete.
func (r *UserRepository) Delete(id uint) error {
	return r.db.Delete(&model.User{}, id).Error
}

// PurgeByID permanently deletes (hard delete, no soft-delete) the user's record.
// Used for the right to erasure (LGPD) — a soft-delete would leave the personal
// data physically in the database.
func (r *UserRepository) PurgeByID(id uint) error {
	return r.db.Unscoped().Delete(&model.User{}, id).Error
}

// ListAll returns all registered users.
func (r *UserRepository) ListAll() ([]model.User, error) {
	var users []model.User
	err := r.db.Order("xp desc").Find(&users).Error
	return users, err
}

// AddXP increments the user's XP score.
func (r *UserRepository) AddXP(numero string, pontos int) error {
	return r.db.Model(&model.User{}).
		Where("numero_wpp = ?", numero).
		UpdateColumn("xp", gorm.Expr("xp + ?", pontos)).Error
}
