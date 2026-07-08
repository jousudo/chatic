// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"chatic/config"
	"chatic/internal/database"
	"chatic/internal/middleware"
	"chatic/internal/model"
	"chatic/internal/queue"
	"chatic/internal/repository"
	"chatic/internal/service"
	"chatic/internal/tutor"

	_ "github.com/glebarez/go-sqlite" // pure-Go SQLite driver (registers the name "sqlite") for the whatsmeow store; same package used by glebarez/sqlite (GORM), so registration happens only once
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// version is injected at build time by GoReleaser (-X main.version=...).
// In local/dev builds it stays "dev".
var version = "dev"

func main() {
	log.Printf("Starting Chatic — Private Multilingual Language Tutor (version %s)...", version)

	// 1. Load configuration (.env)
	cfg := config.LoadConfig()

	// 2. Initialize the main SQLite database
	db, err := database.InitDatabase(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Critical failure starting database: %v", err)
	}

	userRepo := repository.NewUserRepository(db)
	chatRepo := repository.NewChatRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	deviceRepo := repository.NewDeviceRepository(db)

	// 2b. Initialize the local master key (storage/.masterkey, 0600) and load the
	// encrypted API keys from SQLite into CurrentConfig (headless operation).
	if err := service.LoadOrCreateMachineKey("storage/.masterkey"); err != nil {
		log.Printf("Warning: failed to initialize local master key: %v", err)
	}
	service.LoadEncryptedKeysIntoConfig()

	// 2c. Encrypt the conversation content at rest (LGPD/personal data) reusing the
	// AES-GCM vault. Legacy plaintext rows remain readable (passthrough).
	chatRepo.SetContentCrypto(service.EncryptSecret, service.DecryptSecret)

	// Restrict .env permissions — it may contain manual bootstrap secrets.
	if _, err := os.Stat(".env"); err == nil {
		os.Chmod(".env", 0600)
	}

	// 3. Register the initial admin if configured
	if cfg.InitialAdminNumber != "" {
		adminUser, err := userRepo.GetByNumber(cfg.InitialAdminNumber)
		if err != nil {
			// Create the admin in the database
			adminUser = &model.User{
				PhoneNumber:    cfg.InitialAdminNumber,
				Name:           "Admin",
				IsAdmin:        true,
				OnboardingDone: true,
				FlowState:      "COMPLETE",
				Level:          "C2",
				NativeLanguage: "pt-BR",
				TargetLanguage: "en",
				Interests:      "Bot Administration",
			}
			err = userRepo.Create(adminUser)
			if err != nil {
				log.Printf("Warning: failed to register initial admin in the database: %v", err)
			} else {
				log.Printf("Initial admin (%s) registered successfully.", cfg.InitialAdminNumber)
			}
		}
	}

	// 4. Load the whitelist of registered users into fast memory
	users, err := userRepo.ListAll()
	var whitelistNumbers []string
	if err == nil {
		for _, u := range users {
			whitelistNumbers = append(whitelistNumbers, u.PhoneNumber)
		}
	}
	middleware.InitWhitelist(whitelistNumbers)
	log.Printf("Whitelist loaded into memory with %d authorized numbers.", len(whitelistNumbers))

	// 4b. FFmpeg is optional: needed only for audio. Warn clearly if it is missing.
	if service.FFmpegAvailable() {
		log.Printf("FFmpeg detected — audio features (voice messages and audio replies) ACTIVE.")
	} else {
		log.Printf("⚠️  FFmpeg NOT found on PATH — AUDIO features DISABLED.")
		log.Printf("    ✅ Works normally: the full TEXT tutor (conversation, /grammar, /word, /vocab, /quiz, links and PDF).")
		log.Printf("    ⛔ Unavailable: receiving voice messages and replying with audio (TTS).")
		log.Printf("    ➜ To enable audio, install FFmpeg — %s", service.FFmpegInstallHint())
	}

	// 5. Initialize the concurrent FIFO queue (buffer of 100 jobs, 1 sequential worker for SQLite safety)
	queue.InitQueue(100, 1)

	// 6. Initialize the whatsmeow internal session store
	containerPath := "storage/whatsmeow_store.db"
	// Restrict the WhatsApp session file permissions (owner read/write only)
	if f, err := os.OpenFile(containerPath, os.O_CREATE|os.O_RDONLY, 0600); err == nil {
		f.Close()
		os.Chmod(containerPath, 0600)
	}
	dbContainer, err := sqlstore.New(context.Background(), "sqlite", "file:"+containerPath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", waLog.Noop)
	if err != nil {
		log.Fatalf("Failed to initialize Whatsmeow repository: %v", err)
	}

	// 7. Instantiate the internal services
	factory := service.NewLLMFactory()
	engine := tutor.NewTutorEngine()
	onboarding := service.NewOnboardingService(userRepo)
	tts := service.NewTTSService("storage/tts")

	wppService := service.NewWhatsAppService(
		dbContainer,
		deviceRepo,
		userRepo,
		chatRepo,
		groupRepo,
		factory,
		engine,
		onboarding,
		tts,
	)

	// 8. Initialize and register the web admin panel routes
	adminService := service.NewWebAdminService(userRepo, chatRepo, wppService, dbContainer)
	adminService.RegisterRoutes()

	// 9. Start the admin panel web server in a goroutine
	go func() {
		log.Printf("Admin panel running at http://localhost:%s/admin", cfg.Port)
		if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
			log.Printf("Warning: failed to start web panel HTTP server: %v", err)
		}
	}()

	// 10. Start the WhatsApp connection (shared account) and, in multi-account mode,
	// also boot the already-paired personal devices (each member on their own WhatsApp).
	go wppService.Start()
	go wppService.StartPersonalDevices()

	// 11. Wait for a shutdown signal to stop the services gracefully
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down the bot gracefully...")
}
