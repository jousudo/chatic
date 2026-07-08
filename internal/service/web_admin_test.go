// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"chatic/config"
	"chatic/internal/middleware"
	"chatic/internal/model"
	"chatic/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Falha ao abrir banco de dados em memória para teste: %v", err)
	}

	err = db.AutoMigrate(&model.User{}, &model.Message{}, &model.AdminAccount{}, &model.SystemConfig{}, &model.StudyGroup{}, &model.GroupMember{})
	if err != nil {
		t.Fatalf("Falha na migração de teste: %v", err)
	}

	// Initialize the whitelist with an empty instance
	middleware.InitWhitelist([]string{})

	// Initialize the global config for testing
	config.CurrentConfig = &config.Config{
		Port:               "3030",
		DatabasePath:       "test.db",
		PrimaryLLMProvider: "gemini",
		GeminiAPIKey:       "test_key",
		LLMTimeoutSeconds:  10,
	}

	return db
}

func TestGetStatusHandler(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	chatRepo := repository.NewChatRepository(db)

	adminService := &WebAdminService{
		userRepo: userRepo,
		chatRepo: chatRepo,
	}

	req, err := http.NewRequest("GET", "/admin/api/status", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(adminService.handleGetStatus)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Código de status incorreto: obteve %v quer %v", status, http.StatusOK)
	}

	var resp map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("Falha ao unmarshal resposta: %v", err)
	}

	if resp["total_users"].(float64) != 0 {
		t.Errorf("Número de usuários incorreto: obteve %v quer 0", resp["total_users"])
	}
}

func TestAddUserHandler(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	chatRepo := repository.NewChatRepository(db)

	adminService := &WebAdminService{
		userRepo: userRepo,
		chatRepo: chatRepo,
	}

	body := map[string]interface{}{
		"nome":          "João Silva",
		"numero_wpp":    "5511988888888",
		"is_admin":      false,
		"nivel":         "B1",
		"idioma_nativo": "pt-BR",
		"idioma_alvo":   "en",
		"interesses":    "Esportes",
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", "/admin/api/users/add", bytes.NewBuffer(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Method = http.MethodPost

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(adminService.handlePostAddUser)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusCreated {
		t.Errorf("Código de status incorreto: obteve %v quer %v. Resposta: %s", status, http.StatusCreated, rr.Body.String())
	}

	// Check that it was added to the database
	user, err := userRepo.GetByNumber("5511988888888")
	if err != nil {
		t.Fatalf("Usuário não foi inserido no banco: %v", err)
	}
	if user.Name != "João Silva" {
		t.Errorf("Nome incorreto: obteve %s quer João Silva", user.Name)
	}

	// Check that the in-memory whitelist was updated
	if !middleware.Instance.Check("5511988888888") {
		t.Errorf("Número não adicionado na whitelist em memória")
	}
}

// TestHandleGetAccountQRImage checks that a pending pairing's QR is rendered as a PNG
// locally when there is a code, 202 while waiting for the first code, and 404 once the
// pairing is gone (completed/expired).
func TestHandleGetAccountQRImage(t *testing.T) {
	wpp := &WhatsAppService{pendingQR: map[string]string{}}
	s := &WebAdminService{wppService: wpp}

	// Waiting for the first code → 202.
	wpp.setPendingQR("p1", "")
	rec0 := httptest.NewRecorder()
	s.handleGetAccountQRImage(rec0, httptest.NewRequest(http.MethodGet, "/admin/api/accounts/qr.png?id=p1", nil))
	if rec0.Code != http.StatusAccepted {
		t.Fatalf("waiting: status = %d, quer 202", rec0.Code)
	}

	// A code is available → PNG.
	wpp.setPendingQR("p1", "2@abc.def,ghi,jkl")
	rec := httptest.NewRecorder()
	s.handleGetAccountQRImage(rec, httptest.NewRequest(http.MethodGet, "/admin/api/accounts/qr.png?id=p1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quer 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, quer image/png", ct)
	}
	body := rec.Body.Bytes()
	// PNG signature: 0x89 'P' 'N' 'G'.
	if len(body) < 8 || body[0] != 0x89 || string(body[1:4]) != "PNG" {
		t.Errorf("corpo não é um PNG válido (primeiros bytes: % x)", body[:min(8, len(body))])
	}

	// Unknown/finished pairing → 404.
	rec2 := httptest.NewRecorder()
	s.handleGetAccountQRImage(rec2, httptest.NewRequest(http.MethodGet, "/admin/api/accounts/qr.png?id=gone", nil))
	if rec2.Code != http.StatusNotFound {
		t.Errorf("sem QR: status = %d, quer 404", rec2.Code)
	}
}
