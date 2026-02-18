package control

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"
)

type CommandEnvelope struct {
	ID             string    `json:"id"`
	Action         string    `json:"action"`
	ConfigPath     string    `json:"config_path,omitempty"`
	Priority       string    `json:"priority,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	Checksum       string    `json:"checksum,omitempty"`
	Status         string    `json:"status"`
	Reason         string    `json:"reason,omitempty"`
	ReceivedAt     time.Time `json:"received_at"`
}

type CommandIngestStore struct {
	mu            sync.RWMutex
	nextID        int64
	accepted      map[string]*CommandEnvelope
	deadLetters   []CommandEnvelope
	byIdempotency map[string]string
	deadLimit     int
}

func NewCommandIngestStore(deadLetterLimit int) *CommandIngestStore {
	if deadLetterLimit <= 0 {
		deadLetterLimit = 5000
	}
	return &CommandIngestStore{
		accepted:      map[string]*CommandEnvelope{},
		deadLetters:   make([]CommandEnvelope, 0, deadLetterLimit),
		byIdempotency: map[string]string{},
		deadLimit:     deadLetterLimit,
	}
}

func ComputeCommandChecksum(action, configPath, priority, idempotencyKey string) string {
	parts := []string{
		strings.TrimSpace(strings.ToLower(action)),
		strings.TrimSpace(configPath),
		normalizePriority(priority),
		strings.TrimSpace(idempotencyKey),
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(h[:])
}

func (s *CommandIngestStore) RecordAccepted(input CommandEnvelope) CommandEnvelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key := strings.TrimSpace(input.IdempotencyKey); key != "" {
		if existingID, ok := s.byIdempotency[key]; ok {
			if cur, ok := s.accepted[existingID]; ok {
				return *cur
			}
		}
	}
	s.nextID++
	env := input
	env.ID = "cmd-" + itoa(s.nextID)
	env.Status = "accepted"
	env.ReceivedAt = time.Now().UTC()
	s.accepted[env.ID] = &env
	if key := strings.TrimSpace(input.IdempotencyKey); key != "" {
		s.byIdempotency[key] = env.ID
	}
	return env
}

func (s *CommandIngestStore) RecordDeadLetter(input CommandEnvelope, reason string) CommandEnvelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	env := input
	env.ID = "dlq-" + itoa(s.nextID)
	env.Status = "dead_letter"
	env.Reason = strings.TrimSpace(reason)
	env.ReceivedAt = time.Now().UTC()

	if len(s.deadLetters) >= s.deadLimit {
		copy(s.deadLetters[0:], s.deadLetters[1:])
		s.deadLetters[len(s.deadLetters)-1] = env
	} else {
		s.deadLetters = append(s.deadLetters, env)
	}
	return env
}

func (s *CommandIngestStore) DeadLetters() []CommandEnvelope {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]CommandEnvelope, len(s.deadLetters))
	copy(out, s.deadLetters)
	sort.Slice(out, func(i, j int) bool {
		return out[i].ReceivedAt.Before(out[j].ReceivedAt)
	})
	return out
}
