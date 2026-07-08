// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package repository

import (
	"strings"
	"testing"

	"chatic/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestChatRepo(t *testing.T) *ChatRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("falha ao abrir banco em memória: %v", err)
	}
	if err := db.AutoMigrate(&model.Message{}); err != nil {
		t.Fatalf("falha na migração: %v", err)
	}
	return NewChatRepository(db)
}

// With an injected cipher, the content must be stored transformed in the database and
// returned in cleartext by GetRecentMessages (covers the at-rest encryption boundary).
func TestChatRepoContentCryptoRoundTrip(t *testing.T) {
	r := newTestChatRepo(t)
	// Reversible test cipher (prefix), without depending on the real vault.
	r.SetContentCrypto(
		func(s string) string { return "enc:" + s },
		func(s string) string { return strings.TrimPrefix(s, "enc:") },
	)

	const plaintext = "minha mensagem secreta"
	if err := r.SaveMessage(&model.Message{UserID: 1, Session: "default", Sender: "user", Content: plaintext, Type: "text"}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	// In the database (bypassing the repository) the value must be encrypted.
	var raw model.Message
	if err := r.db.First(&raw, "usuario_id = ?", 1).Error; err != nil {
		t.Fatalf("leitura crua: %v", err)
	}
	if raw.Content != "enc:"+plaintext {
		t.Errorf("conteúdo em repouso = %q, deveria estar cifrado", raw.Content)
	}

	// Through the repository, it must come back in cleartext.
	msgs, err := r.GetRecentMessages(1, "default", 10)
	if err != nil {
		t.Fatalf("GetRecentMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != plaintext {
		t.Errorf("GetRecentMessages devolveu %+v, esperava conteúdo em claro %q", msgs, plaintext)
	}
}

// Count/oldest/prune drive the context-summarization flow: count triggers a pass,
// GetOldestMessages feeds the summarizer, and PruneMessagesUpTo drops the folded block
// while keeping the most recent messages intact and decrypting on the way out.
func TestChatRepoSummarizePruneFlow(t *testing.T) {
	r := newTestChatRepo(t)
	r.SetContentCrypto(
		func(s string) string { return "enc:" + s },
		func(s string) string { return strings.TrimPrefix(s, "enc:") },
	)

	const uid = 7
	for i := 0; i < 6; i++ {
		if err := r.SaveMessage(&model.Message{UserID: uid, Session: "default", Sender: "user", Content: "msg", Type: "text"}); err != nil {
			t.Fatalf("SaveMessage %d: %v", i, err)
		}
	}

	n, err := r.CountMessages(uid, "default")
	if err != nil || n != 6 {
		t.Fatalf("CountMessages = %d, %v; want 6", n, err)
	}

	// Fold the oldest 4, keep the newest 2.
	old, err := r.GetOldestMessages(uid, "default", 4)
	if err != nil || len(old) != 4 {
		t.Fatalf("GetOldestMessages = %d msgs, %v; want 4", len(old), err)
	}
	if old[0].ID >= old[3].ID {
		t.Errorf("GetOldestMessages not in ascending id order: %d..%d", old[0].ID, old[3].ID)
	}
	if old[0].Content != "msg" {
		t.Errorf("GetOldestMessages content = %q, want decrypted %q", old[0].Content, "msg")
	}

	maxID := old[len(old)-1].ID
	if err := r.PruneMessagesUpTo(uid, "default", maxID); err != nil {
		t.Fatalf("PruneMessagesUpTo: %v", err)
	}
	n, err = r.CountMessages(uid, "default")
	if err != nil || n != 2 {
		t.Fatalf("after prune CountMessages = %d, %v; want 2", n, err)
	}
	// The pruned rows must be gone permanently (hard delete), not merely soft-deleted.
	var raw int64
	r.db.Unscoped().Model(&model.Message{}).Where("usuario_id = ?", uid).Count(&raw)
	if raw != 2 {
		t.Errorf("hard-delete expected 2 rows remaining, found %d (soft-deleted?)", raw)
	}
}

// Without an injected cipher (default), the repository must act as passthrough.
func TestChatRepoDefaultPassthrough(t *testing.T) {
	r := newTestChatRepo(t)
	const plaintext = "sem cifra"
	if err := r.SaveMessage(&model.Message{UserID: 2, Session: "default", Sender: "user", Content: plaintext, Type: "text"}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}
	msgs, err := r.GetRecentMessages(2, "default", 10)
	if err != nil {
		t.Fatalf("GetRecentMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != plaintext {
		t.Errorf("passthrough falhou: %+v", msgs)
	}
}
