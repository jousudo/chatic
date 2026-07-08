// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package repository

import (
	"fmt"
	"time"

	"chatic/internal/model"

	"gorm.io/gorm"
)

type GroupRepository struct {
	db *gorm.DB
}

func NewGroupRepository(db *gorm.DB) *GroupRepository {
	return &GroupRepository{db: db}
}

// GetByJID looks up a group by WhatsApp JID.
func (r *GroupRepository) GetByJID(jid string) (*model.StudyGroup, error) {
	var group model.StudyGroup
	err := r.db.Where("whatsapp_jid = ?", jid).First(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// GetByInviteCode looks up a group by invite code.
func (r *GroupRepository) GetByInviteCode(code string) (*model.StudyGroup, error) {
	var group model.StudyGroup
	err := r.db.Where("codigo_convite = ?", code).First(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// GetByID looks up a group by ID.
func (r *GroupRepository) GetByID(id uint) (*model.StudyGroup, error) {
	var group model.StudyGroup
	err := r.db.First(&group, id).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// Create inserts a new group into the database.
func (r *GroupRepository) Create(group *model.StudyGroup) error {
	return r.db.Create(group).Error
}

// Save persists changes to an existing group.
func (r *GroupRepository) Save(group *model.StudyGroup) error {
	return r.db.Save(group).Error
}

// CreateAutoGroup creates a group from a WhatsApp JID and returns it.
func (r *GroupRepository) CreateAutoGroup(jid string, creatorID uint) (*model.StudyGroup, error) {
	group := &model.StudyGroup{
		Name:        "WhatsApp Group",
		InviteCode:  fmt.Sprintf("WPP-%X", time.Now().UnixNano()%0xFFFFF),
		WhatsAppJID: jid,
		CreatorID:   creatorID,
	}
	if err := r.db.Create(group).Error; err != nil {
		return nil, err
	}
	return group, nil
}

// IsMember reports whether a user is already a member of the group.
func (r *GroupRepository) IsMember(groupID, userID uint) (bool, error) {
	var count int64
	err := r.db.Model(&model.GroupMember{}).
		Where("grupo_id = ? AND usuario_id = ?", groupID, userID).
		Count(&count).Error
	return count > 0, err
}

// AddMember adds a member to the group with the given role.
func (r *GroupRepository) AddMember(groupID, userID uint, role string) error {
	return r.db.Create(&model.GroupMember{
		GroupID:  groupID,
		UserID:   userID,
		Role:     role,
		JoinedAt: time.Now(),
	}).Error
}

// GetMember fetches a user's membership entry in a group.
func (r *GroupRepository) GetMember(groupID, userID uint) (*model.GroupMember, error) {
	var membro model.GroupMember
	err := r.db.Where("grupo_id = ? AND usuario_id = ?", groupID, userID).First(&membro).Error
	if err != nil {
		return nil, err
	}
	return &membro, nil
}

// PurgeMemberships permanently deletes all of a user's group memberships.
// Used for the right to erasure (LGPD).
func (r *GroupRepository) PurgeMemberships(usuarioID uint) error {
	return r.db.Unscoped().Where("usuario_id = ?", usuarioID).Delete(&model.GroupMember{}).Error
}
