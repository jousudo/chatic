// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"chatic/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDatabase opens the SQLite connection via GORM and runs the automatic migrations.
func InitDatabase(dbPath string) (*gorm.DB, error) {
	// Ensure the storage folder exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create storage folder: %v", err)
	}

	// GORM config with silent log level for data safety (zero-logging of queries that contain chats)
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	db, err := gorm.Open(sqlite.Open(dbPath), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SQLite database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get native SQL instance: %v", err)
	}

	// Enable WAL mode and concurrency optimizations for SQLite
	_, err = sqlDB.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		log.Printf("Warning: failed to enable WAL mode on SQLite: %v", err)
	}
	_, err = sqlDB.Exec("PRAGMA busy_timeout=5000;")
	if err != nil {
		log.Printf("Warning: failed to configure busy timeout on SQLite: %v", err)
	}

	// Limit concurrent connections to avoid SQLite write locks
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Run AutoMigrate for the registered models
	err = db.AutoMigrate(&model.User{}, &model.Message{}, &model.AdminAccount{}, &model.SystemConfig{}, &model.StudyGroup{}, &model.GroupMember{}, &model.WhatsAppDevice{})
	if err != nil {
		return nil, fmt.Errorf("automatic migrations failed: %v", err)
	}

	DB = db
	log.Printf("SQLite database connected and synchronized successfully.")
	return db, nil
}
