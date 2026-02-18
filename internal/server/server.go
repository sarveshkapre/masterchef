package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/masterchef/masterchef/internal/control"
	"github.com/masterchef/masterchef/internal/features"
	"github.com/masterchef/masterchef/internal/state"
)

type Server struct {
	httpServer *http.Server
	queue      *control.Queue
	scheduler  *control.Scheduler
	templates  *control.TemplateStore
	events     *control.EventStore
	runCancel  context.CancelFunc
	metricsMu  sync.Mutex
	metrics    map[string]int64

	backlogThreshold  int
	backlogSamples    []backlogSample
	backlogWarnActive bool
	backlogSatActive  bool
}

type backlogSample struct {
	at      time.Time
	pending int
}

func New(addr, baseDir string) *Server {
	runner := control.NewRunner(baseDir)
	queue := control.NewQueue(512)
	runCtx, runCancel := context.WithCancel(context.Background())
	queue.StartWorker(runCtx, runner)
	scheduler := control.NewScheduler(queue)
	templates := control.NewTemplateStore()
	events := control.NewEventStore(20_000)

	mux := http.NewServeMux()
	s := &Server{
		queue:     queue,
		scheduler: scheduler,
		templates: templates,
		events:    events,
		metrics:   map[string]int64{},
		runCancel: runCancel,
		backlogThreshold: readIntEnv(
			"MC_QUEUE_BACKLOG_SLO_THRESHOLD",
			100,
		),
	}
	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           s.wrapHTTP(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	queue.Subscribe(func(job control.Job) {
		s.events.Append(control.Event{
			Type:    "job." + string(job.Status),
			Message: "job state updated",
			Fields: map[string]any{
				"job_id":   job.ID,
				"status":   job.Status,
				"priority": job.Priority,
			},
		})
		s.observeQueueBacklog()
	})
	s.observeQueueBacklog()

	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/v1/features/summary", s.handleFeatureSummary(baseDir))
	mux.HandleFunc("/v1/activity", s.handleActivity)
	mux.HandleFunc("/v1/metrics", s.handleMetrics)
	mux.HandleFunc("/v1/events/ingest", s.handleEventIngest)
	mux.HandleFunc("/v1/runs", s.handleRuns(baseDir))
	mux.HandleFunc("/v1/jobs", s.handleJobs(baseDir))
	mux.HandleFunc("/v1/jobs/", s.handleJobByID)
	mux.HandleFunc("/v1/control/emergency-stop", s.handleEmergencyStop)
	mux.HandleFunc("/v1/control/maintenance", s.handleMaintenance)
	mux.HandleFunc("/v1/control/capacity", s.handleCapacity)
	mux.HandleFunc("/v1/control/queue", s.handleQueueControl)
	mux.HandleFunc("/v1/control/recover-stuck", s.handleRecoverStuck)
	mux.HandleFunc("/v1/templates", s.handleTemplates(baseDir))
	mux.HandleFunc("/v1/templates/", s.handleTemplateAction)
	mux.HandleFunc("/v1/schedules", s.handleSchedules(baseDir))
	mux.HandleFunc("/v1/schedules/", s.handleScheduleAction)
	return s
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.runCancel != nil {
		s.runCancel()
	}
	if s.scheduler != nil {
		s.scheduler.Shutdown()
	}
	if s.queue != nil {
		s.queue.Wait()
	}
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}

func (s *Server) handleActivity(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.events.List())
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	out := map[string]int64{}
	for k, v := range s.metrics {
		out[k] = v
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleEventIngest(w http.ResponseWriter, r *http.Request) {
	type ingestReq struct {
		Type    string         `json:"type"`
		Message string         `json:"message"`
		Fields  map[string]any `json:"fields"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req ingestReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type is required"})
		return
	}
	if req.Message == "" {
		req.Message = "external event"
	}
	s.events.Append(control.Event{
		Type:    req.Type,
		Message: req.Message,
		Fields:  req.Fields,
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "ingested"})
}

func (s *Server) handleRuns(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		st := state.New(baseDir)
		runs, err := st.ListRuns(200)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, runs)
	}
}

func (s *Server) handleFeatureSummary(baseDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		path := filepath.Join(baseDir, "features.md")
		doc, err := features.Parse(path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		report := features.Verify(doc)
		writeJSON(w, http.StatusOK, report)
	}
}

func (s *Server) handleJobs(baseDir string) http.HandlerFunc {
	type createReq struct {
		ConfigPath string `json:"config_path"`
		Priority   string `json:"priority"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.queue.List())
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			if req.ConfigPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required"})
				return
			}
			if !filepath.IsAbs(req.ConfigPath) {
				req.ConfigPath = filepath.Join(baseDir, req.ConfigPath)
			}
			if _, err := os.Stat(req.ConfigPath); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("config_path not found: %v", err)})
				return
			}
			key := r.Header.Get("Idempotency-Key")
			force := strings.ToLower(r.Header.Get("X-Force-Apply")) == "true"
			priority := req.Priority
			if priority == "" {
				priority = r.Header.Get("X-Queue-Priority")
			}
			job, err := s.queue.Enqueue(req.ConfigPath, key, force, priority)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusAccepted, job)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	id := filepath.Base(r.URL.Path)
	if id == "" || id == "jobs" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing job id"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		j, ok := s.queue.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			return
		}
		writeJSON(w, http.StatusOK, j)
	case http.MethodDelete:
		if err := s.queue.Cancel(id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "canceled"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSchedules(baseDir string) http.HandlerFunc {
	type createReq struct {
		ConfigPath      string `json:"config_path"`
		IntervalSeconds int    `json:"interval_seconds"`
		JitterSeconds   int    `json:"jitter_seconds"`
		Priority        string `json:"priority"`
		ExecutionCost   int    `json:"execution_cost"`
		Host            string `json:"host"`
		Cluster         string `json:"cluster"`
		Environment     string `json:"environment"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.scheduler.List())
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			if req.ConfigPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config_path is required"})
				return
			}
			if req.IntervalSeconds <= 0 {
				req.IntervalSeconds = 60
			}
			if !filepath.IsAbs(req.ConfigPath) {
				req.ConfigPath = filepath.Join(baseDir, req.ConfigPath)
			}
			if _, err := os.Stat(req.ConfigPath); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("config_path not found: %v", err)})
				return
			}
			sc := s.scheduler.CreateWithOptions(control.ScheduleOptions{
				ConfigPath:    req.ConfigPath,
				Priority:      req.Priority,
				ExecutionCost: req.ExecutionCost,
				Host:          req.Host,
				Cluster:       req.Cluster,
				Environment:   req.Environment,
				Interval:      time.Duration(req.IntervalSeconds) * time.Second,
				Jitter:        time.Duration(req.JitterSeconds) * time.Second,
			})
			writeJSON(w, http.StatusCreated, sc)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleTemplates(baseDir string) http.HandlerFunc {
	type createReq struct {
		Name        string                         `json:"name"`
		Description string                         `json:"description"`
		ConfigPath  string                         `json:"config_path"`
		Defaults    map[string]string              `json:"defaults"`
		Survey      map[string]control.SurveyField `json:"survey"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.templates.List())
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			if req.Name == "" || req.ConfigPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and config_path are required"})
				return
			}
			if !filepath.IsAbs(req.ConfigPath) {
				req.ConfigPath = filepath.Join(baseDir, req.ConfigPath)
			}
			if _, err := os.Stat(req.ConfigPath); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("config_path not found: %v", err)})
				return
			}
			t := s.templates.Create(control.Template{
				Name:        req.Name,
				Description: req.Description,
				ConfigPath:  req.ConfigPath,
				Defaults:    req.Defaults,
				Survey:      req.Survey,
			})
			s.events.Append(control.Event{
				Type:    "template.created",
				Message: "template created",
				Fields: map[string]any{
					"template_id": t.ID,
					"name":        t.Name,
				},
			})
			writeJSON(w, http.StatusCreated, t)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleTemplateAction(w http.ResponseWriter, r *http.Request) {
	// /v1/templates/{id}/launch
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid template action path"})
		return
	}
	id := parts[2]
	action := parts[3]

	switch action {
	case "launch":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		type launchReq struct {
			Priority string            `json:"priority"`
			Answers  map[string]string `json:"answers"`
		}
		var launch launchReq
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&launch); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
		}
		t, ok := s.templates.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "template not found"})
			return
		}
		if err := control.ValidateSurveyAnswers(t.Survey, launch.Answers); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		key := r.Header.Get("Idempotency-Key")
		force := strings.ToLower(r.Header.Get("X-Force-Apply")) == "true"
		priority := launch.Priority
		if priority == "" {
			priority = r.Header.Get("X-Queue-Priority")
		}
		job, err := s.queue.Enqueue(t.ConfigPath, key, force, priority)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		s.events.Append(control.Event{
			Type:    "template.launched",
			Message: "template launch enqueued",
			Fields: map[string]any{
				"template_id": t.ID,
				"job_id":      job.ID,
			},
		})
		writeJSON(w, http.StatusAccepted, map[string]any{
			"template": t,
			"job":      job,
			"answers":  launch.Answers,
		})
	case "delete":
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := s.templates.Delete(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown template action"})
	}
}

func (s *Server) handleEmergencyStop(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Enabled bool   `json:"enabled"`
		Reason  string `json:"reason"`
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.queue.EmergencyStatus())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		st := s.queue.SetEmergencyStop(req.Enabled, req.Reason)
		s.events.Append(control.Event{
			Type:    "control.emergency_stop",
			Message: "emergency stop toggled",
			Fields: map[string]any{
				"active": st.Active,
				"reason": st.Reason,
			},
		})
		writeJSON(w, http.StatusOK, st)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMaintenance(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Kind    string `json:"kind"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Reason  string `json:"reason"`
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.scheduler.MaintenanceStatus())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		st, err := s.scheduler.SetMaintenance(req.Kind, req.Name, req.Enabled, req.Reason)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.events.Append(control.Event{
			Type:    "control.maintenance",
			Message: "maintenance mode updated",
			Fields: map[string]any{
				"kind":    st.Kind,
				"name":    st.Name,
				"enabled": st.Enabled,
				"reason":  st.Reason,
			},
		})
		writeJSON(w, http.StatusOK, st)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCapacity(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Action           string `json:"action"` // set_capacity|set_host_health
		MaxBacklog       int    `json:"max_backlog"`
		MaxExecutionCost int    `json:"max_execution_cost"`
		Host             string `json:"host"`
		Healthy          bool   `json:"healthy"`
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.scheduler.CapacityStatus())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		var st control.SchedulerCapacityStatus
		switch req.Action {
		case "set_capacity":
			st = s.scheduler.SetCapacity(req.MaxBacklog, req.MaxExecutionCost)
			s.events.Append(control.Event{
				Type:    "control.capacity",
				Message: "scheduler capacity updated",
				Fields: map[string]any{
					"max_backlog":        st.MaxBacklog,
					"max_execution_cost": st.MaxExecutionCost,
				},
			})
		case "set_host_health":
			if strings.TrimSpace(req.Host) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host is required for set_host_health"})
				return
			}
			st = s.scheduler.SetHostHealth(req.Host, req.Healthy)
			s.events.Append(control.Event{
				Type:    "control.capacity.host_health",
				Message: "host health override updated",
				Fields: map[string]any{
					"host":    strings.ToLower(strings.TrimSpace(req.Host)),
					"healthy": req.Healthy,
				},
			})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
			return
		}
		writeJSON(w, http.StatusOK, st)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleQueueControl(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		Action         string `json:"action"` // pause|resume|drain
		TimeoutSeconds int    `json:"timeout_seconds"`
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.queue.ControlStatus())
	case http.MethodPost:
		var req reqBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		switch req.Action {
		case "pause":
			writeJSON(w, http.StatusOK, s.queue.Pause())
		case "resume":
			writeJSON(w, http.StatusOK, s.queue.Resume())
		case "drain":
			if req.TimeoutSeconds <= 0 {
				req.TimeoutSeconds = 30
			}
			st, err := s.queue.SafeDrain(time.Duration(req.TimeoutSeconds) * time.Second)
			if err != nil {
				writeJSON(w, http.StatusRequestTimeout, map[string]any{
					"error":  err.Error(),
					"status": st,
				})
				return
			}
			writeJSON(w, http.StatusOK, st)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRecoverStuck(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		MaxAgeSeconds int `json:"max_age_seconds"`
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req reqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if req.MaxAgeSeconds <= 0 {
		req.MaxAgeSeconds = 300
	}
	recovered := s.queue.RecoverStuckJobs(time.Duration(req.MaxAgeSeconds) * time.Second)
	writeJSON(w, http.StatusOK, map[string]any{
		"recovered_count": len(recovered),
		"jobs":            recovered,
	})
}

func (s *Server) handleScheduleAction(w http.ResponseWriter, r *http.Request) {
	// /v1/schedules/{id}/enable|disable
	parts := splitPath(r.URL.Path)
	if len(parts) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid schedule action path"})
		return
	}
	id := parts[2]
	action := parts[3]

	switch r.Method {
	case http.MethodPost:
		var ok bool
		switch action {
		case "enable":
			ok = s.scheduler.Enable(id)
		case "disable":
			ok = s.scheduler.Disable(id)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
			return
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": action + "d"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func splitPath(path string) []string {
	trimmed := filepath.ToSlash(filepath.Clean(path))
	parts := make([]string, 0, 8)
	for _, p := range strings.Split(trimmed, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func (s *Server) wrapHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now().UTC()
		reqID := randomID()
		w.Header().Set("X-Request-ID", reqID)

		s.metricsMu.Lock()
		s.metrics["requests_total"]++
		s.metrics["requests."+r.Method]++
		s.metrics["requests."+r.URL.Path]++
		s.metricsMu.Unlock()

		s.events.Append(control.Event{
			Type:    "http.request",
			Message: "request received",
			Fields: map[string]any{
				"id":     reqID,
				"method": r.Method,
				"path":   r.URL.Path,
			},
		})

		next.ServeHTTP(w, r)

		s.events.Append(control.Event{
			Type:    "http.response",
			Message: "request completed",
			Fields: map[string]any{
				"id":         reqID,
				"method":     r.Method,
				"path":       r.URL.Path,
				"started_at": start,
				"ended_at":   time.Now().UTC(),
			},
		})
	})
}

func randomID() string {
	return fmt.Sprintf("req-%d-%d", time.Now().UTC().UnixNano(), rand.Int63())
}

func (s *Server) observeQueueBacklog() {
	st := s.queue.ControlStatus()
	now := time.Now().UTC()

	var emit *control.Event

	s.metricsMu.Lock()
	s.metrics["queue.pending.total"] = int64(st.Pending)
	s.metrics["queue.pending.high"] = int64(st.PendingHigh)
	s.metrics["queue.pending.normal"] = int64(st.PendingNormal)
	s.metrics["queue.pending.low"] = int64(st.PendingLow)
	s.metrics["queue.running"] = int64(st.Running)

	s.backlogSamples = append(s.backlogSamples, backlogSample{
		at:      now,
		pending: st.Pending,
	})
	cutoff := now.Add(-2 * time.Minute)
	first := 0
	for first < len(s.backlogSamples) && s.backlogSamples[first].at.Before(cutoff) {
		first++
	}
	if first > 0 {
		s.backlogSamples = append([]backlogSample{}, s.backlogSamples[first:]...)
	}

	threshold := s.backlogThreshold
	if threshold <= 0 {
		threshold = 100
	}

	growthMilli := int64(0)
	predictive := false
	if len(s.backlogSamples) >= 2 {
		oldest := s.backlogSamples[0]
		latest := s.backlogSamples[len(s.backlogSamples)-1]
		dt := latest.at.Sub(oldest.at).Seconds()
		if dt > 0 && latest.pending > oldest.pending {
			growth := float64(latest.pending-oldest.pending) / dt
			growthMilli = int64(growth * 1000.0)
			projectedFiveMinutes := float64(latest.pending) + (growth * 300.0)
			predictive = projectedFiveMinutes >= float64(threshold)
		}
	}
	s.metrics["queue.backlog_growth_per_sec_milli"] = growthMilli

	prevSat := s.backlogSatActive
	prevWarn := s.backlogWarnActive
	s.backlogSatActive = st.Pending >= threshold
	s.backlogWarnActive = !s.backlogSatActive && predictive && st.Pending >= int(float64(threshold)*0.70)
	recovered := st.Pending <= threshold/2 && (prevSat || prevWarn) && !s.backlogSatActive && !s.backlogWarnActive

	if s.backlogSatActive && !prevSat {
		emit = &control.Event{
			Type:    "queue.saturation",
			Message: "queue backlog exceeded saturation SLO threshold",
			Fields: map[string]any{
				"pending":        st.Pending,
				"threshold":      threshold,
				"pending_high":   st.PendingHigh,
				"pending_normal": st.PendingNormal,
				"pending_low":    st.PendingLow,
			},
		}
	} else if s.backlogWarnActive && !prevWarn {
		emit = &control.Event{
			Type:    "queue.saturation.predicted",
			Message: "predictive queue backlog alert",
			Fields: map[string]any{
				"pending":                      st.Pending,
				"threshold":                    threshold,
				"backlog_growth_per_sec_milli": growthMilli,
			},
		}
	} else if recovered {
		emit = &control.Event{
			Type:    "queue.saturation.recovered",
			Message: "queue backlog recovered below recovery threshold",
			Fields: map[string]any{
				"pending":         st.Pending,
				"recovery_target": threshold / 2,
			},
		}
	}

	s.metrics["queue.saturation.active"] = boolToInt64(s.backlogSatActive)
	s.metrics["queue.saturation.warning"] = boolToInt64(s.backlogWarnActive)
	s.metricsMu.Unlock()

	if emit != nil {
		s.events.Append(*emit)
	}
}

func readIntEnv(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultValue
	}
	return n
}

func boolToInt64(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
