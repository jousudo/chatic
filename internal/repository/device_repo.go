// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package repository

import (
	"chatic/internal/model"

	"gorm.io/gorm"
)

// DeviceRepository persists the role mapping for paired WhatsApp devices, powering the
// optional multi-account mode (see model.WhatsAppDevice).
type DeviceRepository struct {
	db *gorm.DB
}

func NewDeviceRepository(db *gorm.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

// List returns every recorded device role, newest first.
func (r *DeviceRepository) List() ([]model.WhatsAppDevice, error) {
	var devices []model.WhatsAppDevice
	err := r.db.Order("created_at asc").Find(&devices).Error
	return devices, err
}

// GetByJID looks up the role record for a device JID.
func (r *DeviceRepository) GetByJID(jid string) (*model.WhatsAppDevice, error) {
	var d model.WhatsAppDevice
	if err := r.db.Where("jid = ?", jid).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

// Upsert creates or updates the role record for a device JID.
func (r *DeviceRepository) Upsert(jid, role, ownerPN, label string) error {
	var existing model.WhatsAppDevice
	err := r.db.Where("jid = ?", jid).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return r.db.Create(&model.WhatsAppDevice{
			JID:     jid,
			Role:    role,
			OwnerPN: ownerPN,
			Label:   label,
		}).Error
	}
	if err != nil {
		return err
	}
	existing.Role = role
	existing.OwnerPN = ownerPN
	existing.Label = label
	return r.db.Save(&existing).Error
}

// DeleteByJID removes the role record for a device JID (hard delete — the row carries no
// personal chat data, only the pairing role mapping).
func (r *DeviceRepository) DeleteByJID(jid string) error {
	return r.db.Where("jid = ?", jid).Delete(&model.WhatsAppDevice{}).Error
}
