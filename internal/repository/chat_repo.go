// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package repository

import (
	"chatic/internal/model"

	"gorm.io/gorm"
)

// identityCrypto is the safe default when no cipher is injected: passthrough
// (plaintext content). This keeps the repository from ever breaking for lack of a key.
func identityCrypto(s string) string { return s }

type ChatRepository struct {
	db *gorm.DB
	// encrypt/decrypt cipher the message content at rest (LGPD). They are injected
	// via SetContentCrypto to avoid an import cycle with the service package (where
	// the master-key vault lives). Default: passthrough.
	encrypt func(string) string
	decrypt func(string) string
}

func NewChatRepository(db *gorm.DB) *ChatRepository {
	return &ChatRepository{db: db, encrypt: identityCrypto, decrypt: identityCrypto}
}

// SetContentCrypto injects the content encrypt/decrypt functions for data at rest.
// Called at boot with the AES-GCM vault. Nil functions are ignored (keeps the default).
func (r *ChatRepository) SetContentCrypto(encrypt, decrypt func(string) string) {
	if encrypt != nil {
		r.encrypt = encrypt
	}
	if decrypt != nil {
		r.decrypt = decrypt
	}
}

// SaveMessage inserts a message into the history, encrypting the content at rest.
func (r *ChatRepository) SaveMessage(msg *model.Message) error {
	msg.Content = r.encrypt(msg.Content)
	return r.db.Create(msg).Error
}

// GetRecentMessages fetches the last N messages of a user in an active session to build the LLM context.
func (r *ChatRepository) GetRecentMessages(usuarioID uint, sessao string, limit int) ([]model.Message, error) {
	var msgs []model.Message
	// Fetch messages in descending creation order, then reverse for chronological order.
	err := r.db.Where("usuario_id = ? AND sessao = ?", usuarioID, sessao).
		Order("id desc").
		Limit(limit).
		Find(&msgs).Error

	if err != nil {
		return nil, err
	}

	// Decrypt the content at rest (passthrough for legacy plaintext rows).
	for i := range msgs {
		msgs[i].Content = r.decrypt(msgs[i].Content)
	}

	// Reverse to chronological order (oldest to newest)
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	return msgs, nil
}

// CountMessages returns how many messages a user has in a given session.
// Used to decide when the history is large enough to summarize and prune.
func (r *ChatRepository) CountMessages(usuarioID uint, sessao string) (int64, error) {
	var n int64
	err := r.db.Model(&model.Message{}).
		Where("usuario_id = ? AND sessao = ?", usuarioID, sessao).
		Count(&n).Error
	return n, err
}

// GetOldestMessages fetches the oldest N messages of a session in chronological order.
// Used to feed the summarizer with the block of history about to be pruned.
func (r *ChatRepository) GetOldestMessages(usuarioID uint, sessao string, limit int) ([]model.Message, error) {
	var msgs []model.Message
	err := r.db.Where("usuario_id = ? AND sessao = ?", usuarioID, sessao).
		Order("id asc").
		Limit(limit).
		Find(&msgs).Error
	if err != nil {
		return nil, err
	}
	for i := range msgs {
		msgs[i].Content = r.decrypt(msgs[i].Content)
	}
	return msgs, nil
}

// PruneMessagesUpTo permanently (hard) deletes a session's messages with ID <= maxID.
// Called after those messages have been folded into the rolling summary, to keep the DB
// footprint bounded on free-tier hosts. This is resource pruning (the content survives,
// summarized/encrypted on the user record), NOT the LGPD erasure path — see eraseUser.
func (r *ChatRepository) PruneMessagesUpTo(usuarioID uint, sessao string, maxID uint) error {
	return r.db.Unscoped().
		Where("usuario_id = ? AND sessao = ? AND id <= ?", usuarioID, sessao, maxID).
		Delete(&model.Message{}).Error
}

// DeleteSessao deletes the history of a specific user session.
func (r *ChatRepository) DeleteSessao(usuarioID uint, sessao string) error {
	return r.db.Where("usuario_id = ? AND sessao = ?", usuarioID, sessao).Delete(&model.Message{}).Error
}

// PurgeUser permanently deletes (hard delete) all of a user's messages.
// Used for the right to erasure (LGPD): actually removes the personal content.
func (r *ChatRepository) PurgeUser(usuarioID uint) error {
	return r.db.Unscoped().Where("usuario_id = ?", usuarioID).Delete(&model.Message{}).Error
}
