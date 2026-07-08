// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package queue

import (
	"log"
)

type Job struct {
	SenderNumber   string
	MessageText    string
	AudioPath      string // If audio, holds the local path of the temporary file
	DocumentPath   string // If a document, the local path of the temporary file (e.g. PDF)
	DocumentMime   string // MIME reported by WhatsApp (e.g. application/pdf)
	DocumentName   string // Original file name (for user-facing messages)
	IsGroup        bool
	GroupJID       string
	GroupTriggered bool // true if the bot was mentioned in the group (triggers the tutor in Phase 1)
	ProcessFunc    func(job Job) error
}

type QueueManager struct {
	jobQueue   chan Job
	maxWorkers int
}

var GlobalQueue *QueueManager

// InitQueue initializes the global concurrent task queue.
func InitQueue(bufferSize, maxWorkers int) {
	GlobalQueue = &QueueManager{
		jobQueue:   make(chan Job, bufferSize),
		maxWorkers: maxWorkers,
	}
	GlobalQueue.Start()
}

// Enqueue adds a new task to the FIFO queue.
func (qm *QueueManager) Enqueue(job Job) {
	qm.jobQueue <- job
}

// Start launches the background processing goroutines (workers).
func (qm *QueueManager) Start() {
	for i := 1; i <= qm.maxWorkers; i++ {
		go func(workerID int) {
			log.Printf("Queue Worker %d started and ready.", workerID)
			for job := range qm.jobQueue {
				err := job.ProcessFunc(job)
				if err != nil {
					log.Printf("Worker %d error processing job from %s: %v", workerID, job.SenderNumber, err)
				}
			}
		}(i)
	}
}
