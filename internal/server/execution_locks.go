package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/masterchef/masterchef/internal/control"
)

func (s *Server) handleExecutionLocks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		includeHistory := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_history")), "true")
		writeJSON(w, http.StatusOK, s.executionLocks.List(includeHistory))
	case http.MethodPost:
		var req control.ExecutionLockAcquireInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		lock, err := s.executionLocks.Acquire(req)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		s.recordEvent(control.Event{
			Type:    "execution.lock.acquired",
			Message: "execution lock acquired",
			Fields: map[string]any{
				"lock_key": lock.Key,
				"holder":   lock.Holder,
			},
		}, true)
		writeJSON(w, http.StatusCreated, lock)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleExecutionLockRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req control.ExecutionLockReleaseInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	lock, ok := s.executionLocks.Release(req)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "execution lock not found"})
		return
	}
	writeJSON(w, http.StatusOK, lock)
}

func (s *Server) handleExecutionLockCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	expired := s.executionLocks.CleanupExpired()
	writeJSON(w, http.StatusOK, map[string]any{
		"expired_count": len(expired),
		"expired":       expired,
	})
}

func (s *Server) enqueueJobWithOptionalLock(configPath, idempotencyKey string, force bool, priority, lockKey string, lockTTLSeconds int, lockOwner string) (*control.Job, error) {
	lockKey = strings.TrimSpace(lockKey)
	if lockKey == "" {
		return s.queue.Enqueue(configPath, idempotencyKey, force, priority)
	}
	owner := strings.TrimSpace(lockOwner)
	if owner == "" {
		owner = strings.TrimSpace(idempotencyKey)
	}
	if owner == "" {
		owner = "control-plane"
	}
	if _, err := s.executionLocks.Acquire(control.ExecutionLockAcquireInput{
		Key:        lockKey,
		Holder:     owner,
		TTLSeconds: lockTTLSeconds,
	}); err != nil {
		return nil, err
	}
	job, err := s.queue.Enqueue(configPath, idempotencyKey, force, priority)
	if err != nil {
		_, _ = s.executionLocks.Release(control.ExecutionLockReleaseInput{Key: lockKey})
		return nil, err
	}
	if _, err := s.executionLocks.BindJob(lockKey, job.ID); err != nil {
		_, _ = s.executionLocks.Release(control.ExecutionLockReleaseInput{Key: lockKey})
		_ = s.queue.Cancel(job.ID)
		return nil, err
	}
	return job, nil
}
