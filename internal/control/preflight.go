package control

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

type PreflightRequest struct {
	DNS                []string `json:"dns"`
	TCP                []string `json:"tcp"`
	StoragePaths       []string `json:"storage_paths"`
	RequireObjectStore bool     `json:"require_object_store"`
	RequireQueue       bool     `json:"require_queue"`
}

type PreflightCheckResult struct {
	Type   string `json:"type"`
	Target string `json:"target"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type PreflightReport struct {
	Status    string                 `json:"status"`
	CheckedAt time.Time              `json:"checked_at"`
	Results   []PreflightCheckResult `json:"results"`
	Failed    int                    `json:"failed"`
}

func RunPreflight(req PreflightRequest, queue *Queue, hasObjectStore bool) PreflightReport {
	results := make([]PreflightCheckResult, 0, len(req.DNS)+len(req.TCP)+len(req.StoragePaths)+2)
	failed := 0

	for _, host := range req.DNS {
		if host == "" {
			continue
		}
		_, err := net.LookupHost(host)
		if err != nil {
			results = append(results, PreflightCheckResult{Type: "dns", Target: host, OK: false, Detail: err.Error()})
			failed++
			continue
		}
		results = append(results, PreflightCheckResult{Type: "dns", Target: host, OK: true})
	}

	for _, target := range req.TCP {
		if target == "" {
			continue
		}
		conn, err := net.DialTimeout("tcp", target, 1500*time.Millisecond)
		if err != nil {
			results = append(results, PreflightCheckResult{Type: "tcp", Target: target, OK: false, Detail: err.Error()})
			failed++
			continue
		}
		_ = conn.Close()
		results = append(results, PreflightCheckResult{Type: "tcp", Target: target, OK: true})
	}

	for _, basePath := range req.StoragePaths {
		if basePath == "" {
			continue
		}
		if err := os.MkdirAll(basePath, 0o755); err != nil {
			results = append(results, PreflightCheckResult{Type: "storage", Target: basePath, OK: false, Detail: err.Error()})
			failed++
			continue
		}
		probeDir := filepath.Join(basePath, ".masterchef-preflight")
		if err := os.MkdirAll(probeDir, 0o755); err != nil {
			results = append(results, PreflightCheckResult{Type: "storage", Target: basePath, OK: false, Detail: err.Error()})
			failed++
			continue
		}
		probeFile := filepath.Join(probeDir, fmt.Sprintf("probe-%d.tmp", time.Now().UTC().UnixNano()))
		if err := os.WriteFile(probeFile, []byte("ok"), 0o644); err != nil {
			results = append(results, PreflightCheckResult{Type: "storage", Target: basePath, OK: false, Detail: err.Error()})
			failed++
			continue
		}
		_ = os.Remove(probeFile)
		results = append(results, PreflightCheckResult{Type: "storage", Target: basePath, OK: true})
	}

	if req.RequireObjectStore {
		if hasObjectStore {
			results = append(results, PreflightCheckResult{Type: "object_store", Target: "configured", OK: true})
		} else {
			results = append(results, PreflightCheckResult{Type: "object_store", Target: "configured", OK: false, Detail: "object store unavailable"})
			failed++
		}
	}

	if req.RequireQueue {
		if queue == nil {
			results = append(results, PreflightCheckResult{Type: "queue", Target: "control_queue", OK: false, Detail: "queue unavailable"})
			failed++
		} else {
			st := queue.ControlStatus()
			if st.Paused {
				results = append(results, PreflightCheckResult{Type: "queue", Target: "control_queue", OK: false, Detail: "queue is paused"})
				failed++
			} else {
				results = append(results, PreflightCheckResult{Type: "queue", Target: "control_queue", OK: true})
			}
		}
	}

	status := "pass"
	if failed > 0 {
		status = "fail"
	}
	return PreflightReport{
		Status:    status,
		CheckedAt: time.Now().UTC(),
		Results:   results,
		Failed:    failed,
	}
}
