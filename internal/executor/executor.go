package executor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/masterchef/masterchef/internal/config"
	"github.com/masterchef/masterchef/internal/planner"
	"github.com/masterchef/masterchef/internal/provider"
	"github.com/masterchef/masterchef/internal/state"
)

type Executor struct {
	stepTimeout       time.Duration
	baseDir           string
	registry          *provider.Registry
	transportHandlers map[string]transportApplyFunc
}

type transportApplyFunc func(step planner.Step, r config.Resource) (bool, bool, string, error)

type filebucketSnapshot struct {
	Eligible bool
	Path     string
	Content  []byte
	Checksum string
}

func New(baseDir string) *Executor {
	e := &Executor{
		stepTimeout: 30 * time.Second,
		baseDir:     baseDir,
		registry:    provider.NewBuiltinRegistry(),
	}
	e.registerBuiltinTransports()
	return e
}

func NewWithRegistry(stepTimeout time.Duration, reg *provider.Registry) *Executor {
	if stepTimeout <= 0 {
		stepTimeout = 30 * time.Second
	}
	if reg == nil {
		reg = provider.NewBuiltinRegistry()
	}
	e := &Executor{
		stepTimeout: stepTimeout,
		registry:    reg,
	}
	e.registerBuiltinTransports()
	return e
}

func (e *Executor) Apply(p *planner.Plan) (state.RunRecord, error) {
	run := state.RunRecord{
		ID:        time.Now().UTC().Format("20060102T150405.000000000"),
		StartedAt: time.Now().UTC(),
		Status:    state.RunSucceeded,
		Results:   make([]state.ResourceRun, 0, len(p.Steps)),
	}

	policy := p.Execution
	strategy := strings.ToLower(strings.TrimSpace(policy.Strategy))
	if strategy == "" {
		strategy = "linear"
	}
	steps := p.Steps
	switch strategy {
	case "serial":
		steps = serialOrderedSteps(p.Steps, policy.Serial, policy.FailureDomain)
	}
	refreshSources := buildRefreshSourceIndex(steps)
	changedByResource := map[string]bool{}
	notifiedHandlers := map[string]struct{}{}

	failedSteps := 0
	executedSteps := 0
	shouldStop := func() bool {
		if failedSteps == 0 {
			return false
		}
		if policy.AnyErrorsFatal {
			return true
		}
		if strategy == "linear" || strategy == "serial" {
			return true
		}
		if policy.MaxFailPercentage > 0 && executedSteps > 0 {
			failPct := (failedSteps * 100) / executedSteps
			if failPct > policy.MaxFailPercentage {
				return true
			}
		}
		return false
	}

	for _, step := range steps {
		triggeredSources := refreshTriggeredSources(step.Resource, refreshSources, changedByResource)
		if step.Resource.RefreshOnly && len(triggeredSources) == 0 {
			run.Results = append(run.Results, state.ResourceRun{
				ResourceID: step.Resource.ID,
				Type:       step.Resource.Type,
				Host:       step.Resource.Host,
				Skipped:    true,
				Message:    "refresh-only resource not triggered",
			})
			changedByResource[step.Resource.ID] = false
			executedSteps++
			continue
		}
		if len(triggeredSources) > 0 && step.Resource.Type == "command" && strings.TrimSpace(step.Resource.RefreshCommand) != "" {
			step.Resource.Command = strings.TrimSpace(step.Resource.RefreshCommand)
			step.Resource.Creates = ""
			step.Resource.OnlyIf = ""
			step.Resource.Unless = ""
		}
		res, failed := e.executeStep(step)
		if len(triggeredSources) > 0 {
			res.Message = appendAuditMessage(res.Message, "refresh triggered by: "+strings.Join(triggeredSources, ", "))
		}
		run.Results = append(run.Results, res)
		changedByResource[step.Resource.ID] = res.Changed
		if res.Changed && !res.Skipped {
			for _, handlerID := range step.Resource.NotifyHandlers {
				handlerID = strings.TrimSpace(handlerID)
				if handlerID == "" {
					continue
				}
				notifiedHandlers[handlerID] = struct{}{}
			}
		}
		executedSteps++
		if failed {
			failedSteps++
			run.Status = state.RunFailed
			if shouldStop() {
				break
			}
		}
	}
	if run.Status == state.RunSucceeded && len(notifiedHandlers) > 0 {
		handlerIDs := make([]string, 0, len(notifiedHandlers))
		for id := range notifiedHandlers {
			handlerIDs = append(handlerIDs, id)
		}
		sort.Strings(handlerIDs)
		for _, id := range handlerIDs {
			handlerStep, ok := p.Handlers[id]
			if !ok {
				run.Status = state.RunFailed
				run.Results = append(run.Results, state.ResourceRun{
					ResourceID: id,
					Type:       "handler",
					Message:    "notified handler not found in plan",
				})
				break
			}
			res, failed := e.executeStep(handlerStep)
			res.Message = appendAuditMessage(res.Message, "handler executed")
			run.Results = append(run.Results, res)
			if failed {
				run.Status = state.RunFailed
				break
			}
		}
	}

	run.EndedAt = time.Now().UTC()
	if run.Status == "" {
		run.Status = state.RunSucceeded
	}
	return run, nil
}

func (e *Executor) executeStep(step planner.Step) (state.ResourceRun, bool) {
	filebucket := e.captureFilebucketSnapshot(step)
	attempts := 1
	if step.Resource.Retries > 0 {
		attempts = step.Resource.Retries + 1
	}
	baseDelay := time.Duration(step.Resource.RetryDelaySeconds) * time.Second
	backoff := normalizeRetryBackoff(step.Resource.RetryBackoff)
	jitterMax := time.Duration(step.Resource.RetryJitterSecs) * time.Second
	untilContains := strings.TrimSpace(step.Resource.UntilContains)

	var last state.ResourceRun
	var failed bool
	for attempt := 1; attempt <= attempts; attempt++ {
		last, failed = e.executeSingleStep(step)
		if !failed && untilContains != "" && !strings.Contains(last.Message, untilContains) {
			failed = true
			if strings.TrimSpace(last.Message) == "" {
				last.Message = "until_contains condition not met: " + untilContains
			} else {
				last.Message = strings.TrimSpace(last.Message) + "; until_contains condition not met: " + untilContains
			}
		}
		if !failed {
			if filebucket.Eligible && !last.Skipped && last.Changed {
				if msg, err := e.persistFilebucketBackup(step, filebucket); err != nil {
					last.Message = appendAuditMessage(last.Message, "filebucket backup failed: "+err.Error())
				} else {
					last.Message = appendAuditMessage(last.Message, msg)
				}
			}
			if attempt > 1 {
				last.Message = strings.TrimSpace(last.Message + " (succeeded after " + strconv.Itoa(attempt) + " attempts)")
			}
			return last, false
		}
		if attempt < attempts {
			delay := retryDelayForAttempt(baseDelay, attempt, backoff) + retryJitterForAttempt(step.Resource.ID, attempt, jitterMax)
			if delay > 0 {
				time.Sleep(delay)
			}
		}
	}
	if attempts > 1 {
		last.Message = strings.TrimSpace(last.Message + " (failed after " + strconv.Itoa(attempts) + " attempts)")
	}
	return last, true
}

func (e *Executor) executeSingleStep(step planner.Step) (state.ResourceRun, bool) {
	r := step.Resource
	handler, ok := e.transportHandlers[strings.ToLower(strings.TrimSpace(step.Host.Transport))]
	if !ok {
		return state.ResourceRun{
			ResourceID: r.ID,
			Type:       r.Type,
			Host:       r.Host,
			Message:    "unsupported transport: " + step.Host.Transport,
		}, true
	}

	res := state.ResourceRun{
		ResourceID: r.ID,
		Type:       r.Type,
		Host:       r.Host,
	}

	preparedResource, audit, prepErr := prepareResourceForExecution(step.Host, r)
	if prepErr != nil {
		res.Message = prepErr.Error()
		return res, true
	}

	changed, skipped, msg, err := handler(step, preparedResource)
	res.Changed = changed
	res.Skipped = skipped
	res.Message = appendAuditMessage(msg, audit)
	recordPath, recordErr := e.maybeRecordSession(step, preparedResource, msg, err)
	if recordErr != nil {
		res.Message = appendAuditMessage(res.Message, "session record error: "+recordErr.Error())
	} else if recordPath != "" {
		res.Message = appendAuditMessage(res.Message, "session record: "+recordPath)
	}

	if err != nil && strings.TrimSpace(r.RescueCommand) != "" {
		hookMsg, hookChanged, hookErr := e.runCommandHook(step, handler, r, "rescue", r.RescueCommand)
		res.Message = appendAuditMessage(res.Message, hookMsg)
		res.Changed = res.Changed || hookChanged
		if hookErr == nil {
			err = nil
			res.Skipped = false
		} else {
			err = fmt.Errorf("%w; rescue hook failed: %v", err, hookErr)
		}
	}

	if strings.TrimSpace(r.AlwaysCommand) != "" {
		hookMsg, hookChanged, hookErr := e.runCommandHook(step, handler, r, "always", r.AlwaysCommand)
		res.Message = appendAuditMessage(res.Message, hookMsg)
		res.Changed = res.Changed || hookChanged
		if hookErr != nil {
			if err != nil {
				err = fmt.Errorf("%w; always hook failed: %v", err, hookErr)
			} else {
				err = fmt.Errorf("always hook failed: %w", hookErr)
			}
		}
	}

	if err == nil {
		return res, false
	}
	if strings.TrimSpace(res.Message) == "" {
		res.Message = err.Error()
	}
	return res, true
}

func (e *Executor) runCommandHook(step planner.Step, handler transportApplyFunc, base config.Resource, hookName, command string) (string, bool, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", false, nil
	}
	hookResource := base
	hookResource.Command = command
	hookResource.Creates = ""
	hookResource.OnlyIf = ""
	hookResource.Unless = ""
	hookResource.Retries = 0
	hookResource.RetryDelaySeconds = 0
	hookResource.RetryBackoff = ""
	hookResource.RetryJitterSecs = 0
	hookResource.UntilContains = ""
	hookResource.RescueCommand = ""
	hookResource.AlwaysCommand = ""

	preparedResource, audit, prepErr := prepareResourceForExecution(step.Host, hookResource)
	if prepErr != nil {
		return hookName + ": " + prepErr.Error(), false, prepErr
	}
	changed, _, msg, execErr := handler(step, preparedResource)
	msg = appendAuditMessage(msg, hookName+" hook")
	msg = appendAuditMessage(msg, audit)
	recordPath, recordErr := e.maybeRecordSession(step, preparedResource, msg, execErr)
	if recordErr != nil {
		msg = appendAuditMessage(msg, "session record error: "+recordErr.Error())
	} else if recordPath != "" {
		msg = appendAuditMessage(msg, "session record: "+recordPath)
	}
	if execErr != nil {
		if strings.TrimSpace(msg) == "" {
			msg = execErr.Error()
		}
		return msg, changed, execErr
	}
	return msg, changed, nil
}

func prepareResourceForExecution(host config.Host, r config.Resource) (config.Resource, string, error) {
	if r.Type != "command" || !r.Become {
		return r, "", nil
	}
	transport := strings.ToLower(strings.TrimSpace(host.Transport))
	r.BecomeUser = strings.TrimSpace(r.BecomeUser)
	audit := "privilege escalation via sudo"
	if r.BecomeUser != "" {
		audit += " as " + r.BecomeUser
	}

	switch transport {
	case "local", "ssh":
		r.Command = wrapWithSudo(r.Command, r.BecomeUser)
		if strings.TrimSpace(r.OnlyIf) != "" {
			r.OnlyIf = wrapWithSudo(r.OnlyIf, r.BecomeUser)
		}
		if strings.TrimSpace(r.Unless) != "" {
			r.Unless = wrapWithSudo(r.Unless, r.BecomeUser)
		}
		return r, audit, nil
	case "winrm":
		return r, "", fmt.Errorf("resource %q uses become, but winrm escalation is not supported", r.ID)
	default:
		// plugin and future transports can inspect become flags directly.
		return r, audit, nil
	}
}

func buildRefreshSourceIndex(steps []planner.Step) map[string][]string {
	index := map[string][]string{}
	seen := map[string]map[string]struct{}{}
	addSource := func(target, source string) {
		target = strings.TrimSpace(target)
		source = strings.TrimSpace(source)
		if target == "" || source == "" {
			return
		}
		if seen[target] == nil {
			seen[target] = map[string]struct{}{}
		}
		if _, ok := seen[target][source]; ok {
			return
		}
		seen[target][source] = struct{}{}
		index[target] = append(index[target], source)
	}
	for _, step := range steps {
		r := step.Resource
		for _, target := range r.Notify {
			addSource(target, r.ID)
		}
		for _, source := range r.Subscribe {
			addSource(r.ID, source)
		}
	}
	for target := range index {
		sort.Strings(index[target])
	}
	return index
}

func refreshTriggeredSources(resource config.Resource, refreshSources map[string][]string, changedByResource map[string]bool) []string {
	sources := refreshSources[resource.ID]
	if len(sources) == 0 {
		return nil
	}
	triggered := make([]string, 0, len(sources))
	for _, src := range sources {
		if changedByResource[src] {
			triggered = append(triggered, src)
		}
	}
	return triggered
}

func wrapWithSudo(command, user string) string {
	if strings.TrimSpace(user) == "" {
		return "sudo sh -lc " + shellQuote(command)
	}
	return "sudo -u " + shellQuote(user) + " sh -lc " + shellQuote(command)
}

func appendAuditMessage(message, audit string) string {
	message = strings.TrimSpace(message)
	audit = strings.TrimSpace(audit)
	if audit == "" {
		return message
	}
	if message == "" {
		return audit
	}
	return message + "; " + audit
}

func (e *Executor) captureFilebucketSnapshot(step planner.Step) filebucketSnapshot {
	r := step.Resource
	if r.Type != "file" || strings.TrimSpace(r.Path) == "" || strings.TrimSpace(e.baseDir) == "" {
		return filebucketSnapshot{}
	}
	transport := strings.ToLower(strings.TrimSpace(step.Host.Transport))
	if transport == "winrm" && isLocalWinRMHost(step.Host) {
		transport = "local"
	}
	if transport != "local" {
		return filebucketSnapshot{}
	}
	full := filepath.Clean(r.Path)
	current, err := os.ReadFile(full)
	if err != nil {
		return filebucketSnapshot{}
	}
	if bytes.Equal(current, []byte(r.Content)) {
		return filebucketSnapshot{}
	}
	sum := sha256.Sum256(current)
	return filebucketSnapshot{
		Eligible: true,
		Path:     full,
		Content:  current,
		Checksum: "sha256:" + hex.EncodeToString(sum[:]),
	}
}

func (e *Executor) persistFilebucketBackup(step planner.Step, snap filebucketSnapshot) (string, error) {
	if !snap.Eligible {
		return "", nil
	}
	base := filepath.Join(e.baseDir, ".masterchef", "filebucket")
	objectsDir := filepath.Join(base, "objects")
	if err := os.MkdirAll(objectsDir, 0o755); err != nil {
		return "", fmt.Errorf("create objects dir: %w", err)
	}
	objectKey := strings.ReplaceAll(snap.Checksum, ":", "-")
	objectPath := filepath.Join(objectsDir, objectKey)
	if _, err := os.Stat(objectPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(objectPath, snap.Content, 0o644); err != nil {
			return "", fmt.Errorf("write object: %w", err)
		}
	}
	history := map[string]any{
		"time":        time.Now().UTC(),
		"resource_id": step.Resource.ID,
		"host":        step.Resource.Host,
		"path":        snap.Path,
		"checksum":    snap.Checksum,
		"size_bytes":  len(snap.Content),
	}
	line, err := json.Marshal(history)
	if err != nil {
		return "", fmt.Errorf("marshal history: %w", err)
	}
	historyPath := filepath.Join(base, "history.ndjson")
	f, err := os.OpenFile(historyPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", fmt.Errorf("open history: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return "", fmt.Errorf("append history: %w", err)
	}
	return "filebucket backup: " + snap.Checksum, nil
}

func normalizeRetryBackoff(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", "constant", "linear", "exponential":
		return mode
	default:
		return ""
	}
}

func retryDelayForAttempt(base time.Duration, attempt int, mode string) time.Duration {
	if base <= 0 || attempt <= 0 {
		return 0
	}
	switch mode {
	case "linear":
		return base * time.Duration(attempt)
	case "exponential":
		if attempt > 30 {
			attempt = 30
		}
		return base * time.Duration(1<<(attempt-1))
	default:
		return base
	}
}

func retryJitterForAttempt(resourceID string, attempt int, max time.Duration) time.Duration {
	if max <= 0 || attempt <= 0 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(resourceID)))
	_, _ = h.Write([]byte(":"))
	_, _ = h.Write([]byte(strconv.Itoa(attempt)))
	n := h.Sum32()
	maxNanos := max.Nanoseconds()
	if maxNanos <= 0 {
		return 0
	}
	return time.Duration(int64(n)%(maxNanos+1)) * time.Nanosecond
}

type sessionRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	Host       string    `json:"host"`
	Transport  string    `json:"transport"`
	Resource   string    `json:"resource_id"`
	Command    string    `json:"command,omitempty"`
	Become     bool      `json:"become"`
	BecomeUser string    `json:"become_user,omitempty"`
	Output     string    `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
}

func (e *Executor) maybeRecordSession(step planner.Step, resource config.Resource, output string, execErr error) (string, error) {
	if !shouldRecordSession(step.Host, resource) {
		return "", nil
	}
	if strings.TrimSpace(e.baseDir) == "" {
		return "", nil
	}
	dir := filepath.Join(e.baseDir, ".masterchef", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create session dir: %w", err)
	}
	name := sanitizeSessionToken(step.Resource.ID) + "-" + time.Now().UTC().Format("20060102T150405.000000000") + ".json"
	path := filepath.Join(dir, name)
	record := sessionRecord{
		Timestamp:  time.Now().UTC(),
		Host:       step.Host.Name,
		Transport:  strings.ToLower(strings.TrimSpace(step.Host.Transport)),
		Resource:   step.Resource.ID,
		Command:    resource.Command,
		Become:     resource.Become,
		BecomeUser: resource.BecomeUser,
		Output:     strings.TrimSpace(output),
	}
	if execErr != nil {
		record.Error = execErr.Error()
	}
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal session record: %w", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", fmt.Errorf("write session record: %w", err)
	}
	return path, nil
}

func shouldRecordSession(host config.Host, resource config.Resource) bool {
	if resource.Type != "command" || !resource.Become {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(host.Transport)) {
	case "ssh", "winrm":
		return true
	default:
		return false
	}
}

func sanitizeSessionToken(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "resource"
	}
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	return b.String()
}

func (e *Executor) registerBuiltinTransports() {
	e.transportHandlers = map[string]transportApplyFunc{
		"local": func(_ planner.Step, r config.Resource) (bool, bool, string, error) {
			pRes, err := e.applyLocalResource(r)
			return pRes.Changed, pRes.Skipped, pRes.Message, err
		},
		"ssh": func(step planner.Step, r config.Resource) (bool, bool, string, error) {
			return e.applyOverSSH(step, r)
		},
		"winrm": func(step planner.Step, r config.Resource) (bool, bool, string, error) {
			return e.applyOverWinRM(step, r)
		},
	}
}

func (e *Executor) RegisterTransport(name string, handler transportApplyFunc) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return fmt.Errorf("transport name is required")
	}
	if handler == nil {
		return fmt.Errorf("transport handler is required")
	}
	e.transportHandlers[name] = handler
	return nil
}

func (e *Executor) applyLocalResource(r config.Resource) (provider.Result, error) {
	h, ok := e.registry.Lookup(r.Type)
	if !ok {
		return provider.Result{}, fmt.Errorf("no provider registered for type %q", r.Type)
	}
	ctx, cancel := context.WithTimeout(context.Background(), e.stepTimeout)
	defer cancel()
	return h.Apply(ctx, r)
}

func serialOrderedSteps(in []planner.Step, serial int, failureDomain string) []planner.Step {
	if serial <= 0 {
		serial = 1
	}
	hostByName := map[string]config.Host{}
	hostSeen := map[string]struct{}{}
	hostOrder := make([]string, 0)
	for _, step := range in {
		host := strings.TrimSpace(step.Host.Name)
		if host == "" {
			host = strings.TrimSpace(step.Resource.Host)
		}
		if host == "" {
			host = "unknown-host"
		}
		if _, ok := hostByName[host]; !ok {
			hostByName[host] = step.Host
		}
		if _, ok := hostSeen[host]; ok {
			continue
		}
		hostSeen[host] = struct{}{}
		hostOrder = append(hostOrder, host)
	}
	hostOrder = orderedHostsByFailureDomain(hostOrder, hostByName, failureDomain)

	out := make([]planner.Step, 0, len(in))
	for i := 0; i < len(hostOrder); i += serial {
		end := i + serial
		if end > len(hostOrder) {
			end = len(hostOrder)
		}
		batchHosts := map[string]struct{}{}
		for _, host := range hostOrder[i:end] {
			batchHosts[host] = struct{}{}
		}
		for _, step := range in {
			host := strings.TrimSpace(step.Host.Name)
			if host == "" {
				host = strings.TrimSpace(step.Resource.Host)
			}
			if host == "" {
				host = "unknown-host"
			}
			if _, ok := batchHosts[host]; ok {
				out = append(out, step)
			}
		}
	}
	return out
}

func orderedHostsByFailureDomain(hostOrder []string, hostByName map[string]config.Host, failureDomain string) []string {
	sort.Strings(hostOrder)
	failureDomain = strings.ToLower(strings.TrimSpace(failureDomain))
	if failureDomain == "" {
		return hostOrder
	}

	grouped := map[string][]string{}
	domainOrder := make([]string, 0)
	for _, host := range hostOrder {
		domain := strings.ToLower(strings.TrimSpace(hostByName[host].Topology[failureDomain]))
		if domain == "" {
			domain = "unknown-" + failureDomain
		}
		if _, ok := grouped[domain]; !ok {
			domainOrder = append(domainOrder, domain)
		}
		grouped[domain] = append(grouped[domain], host)
	}
	for domain := range grouped {
		sort.Strings(grouped[domain])
	}
	sort.Strings(domainOrder)

	ordered := make([]string, 0, len(hostOrder))
	for {
		progressed := false
		for _, domain := range domainOrder {
			queue := grouped[domain]
			if len(queue) == 0 {
				continue
			}
			ordered = append(ordered, queue[0])
			grouped[domain] = queue[1:]
			progressed = true
		}
		if !progressed {
			break
		}
	}
	return ordered
}

func (e *Executor) applyOverSSH(step planner.Step, r config.Resource) (bool, bool, string, error) {
	switch r.Type {
	case "file":
		marker := "MASTERCHEF_EOF_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
		var b strings.Builder
		b.WriteString("mkdir -p ")
		b.WriteString(shellQuote(filepath.Dir(r.Path)))
		b.WriteString("\ncat > ")
		b.WriteString(shellQuote(r.Path))
		b.WriteString(" <<'")
		b.WriteString(marker)
		b.WriteString("'\n")
		b.WriteString(r.Content)
		if !strings.HasSuffix(r.Content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString(marker)
		b.WriteString("\n")
		if r.Mode != "" {
			b.WriteString("chmod ")
			b.WriteString(shellQuote(r.Mode))
			b.WriteString(" ")
			b.WriteString(shellQuote(r.Path))
			b.WriteString("\n")
		}

		out, err := e.runSSH(step.Host, b.String())
		if err != nil {
			return false, false, string(out), err
		}
		return true, false, strings.TrimSpace(string(out)), nil

	case "command":
		var b strings.Builder
		if r.Creates != "" {
			b.WriteString("if [ -e ")
			b.WriteString(shellQuote(r.Creates))
			b.WriteString(" ]; then echo __MASTERCHEF_SKIP_CREATES__; exit 0; fi\n")
		}
		if r.Unless != "" {
			b.WriteString("if sh -lc ")
			b.WriteString(shellQuote(r.Unless))
			b.WriteString(" >/dev/null 2>&1; then echo __MASTERCHEF_SKIP_UNLESS__; exit 0; fi\n")
		}
		b.WriteString("sh -lc ")
		b.WriteString(shellQuote(r.Command))
		b.WriteString("\n")

		out, err := e.runSSH(step.Host, b.String())
		outText := strings.TrimSpace(string(out))
		if outText == "__MASTERCHEF_SKIP_CREATES__" || outText == "__MASTERCHEF_SKIP_UNLESS__" {
			return false, true, outText, nil
		}
		if err != nil {
			return false, false, outText, err
		}
		return true, false, outText, nil
	default:
		return false, false, "", fmt.Errorf("unsupported resource type %q for ssh transport", r.Type)
	}
}

func (e *Executor) runSSH(host config.Host, script string) ([]byte, error) {
	args := e.buildSSHArgs(host, script)

	ctx, cancel := context.WithTimeout(context.Background(), e.stepTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("ssh apply failed: %w: %s", err, string(out))
	}
	return out, nil
}

func (e *Executor) buildSSHArgs(host config.Host, script string) []string {
	target := strings.TrimSpace(host.Address)
	if target == "" {
		target = strings.TrimSpace(host.Name)
	}
	if strings.TrimSpace(host.User) != "" {
		target = strings.TrimSpace(host.User) + "@" + target
	}

	args := make([]string, 0, 12)
	if host.Port > 0 {
		args = append(args, "-p", strconv.Itoa(host.Port))
	}
	if jump := buildSSHJumpTarget(host); jump != "" {
		args = append(args, "-J", jump)
	}
	if proxy := strings.TrimSpace(host.ProxyCommand); proxy != "" {
		args = append(args, "-o", "ProxyCommand="+proxy)
	}
	args = append(args, target, "sh", "-lc", script)
	return args
}

func buildSSHJumpTarget(host config.Host) string {
	jumpHost := strings.TrimSpace(host.JumpAddress)
	if jumpHost == "" {
		return ""
	}
	if strings.TrimSpace(host.JumpUser) != "" {
		jumpHost = strings.TrimSpace(host.JumpUser) + "@" + jumpHost
	}
	if host.JumpPort > 0 {
		jumpHost += ":" + strconv.Itoa(host.JumpPort)
	}
	return jumpHost
}

func (e *Executor) applyOverWinRM(step planner.Step, r config.Resource) (bool, bool, string, error) {
	if isLocalWinRMHost(step.Host) {
		// Local shim keeps tests deterministic while preserving transport semantics.
		res, err := e.applyLocalResource(r)
		return res.Changed, res.Skipped, res.Message, err
	}

	target := step.Host.Address
	if target == "" {
		target = step.Host.Name
	}
	if strings.TrimSpace(target) == "" {
		return false, false, "", fmt.Errorf("winrm host target is required")
	}

	switch r.Type {
	case "file":
		ps := "Set-Content -Path " + quotePowerShell(r.Path) + " -Value " + quotePowerShell(r.Content)
		if r.Mode != "" {
			ps += "; # mode mapping for winrm is provider-specific and currently advisory"
		}
		out, err := e.runWinRMPowerShell(target, ps)
		if err != nil {
			return false, false, strings.TrimSpace(string(out)), err
		}
		return true, false, strings.TrimSpace(string(out)), nil
	case "command":
		ps := r.Command
		if r.Creates != "" {
			ps = "if (Test-Path " + quotePowerShell(r.Creates) + ") { Write-Output '__MASTERCHEF_SKIP_CREATES__'; exit 0 }; " + ps
		}
		if r.Unless != "" {
			ps = "if (" + r.Unless + ") { Write-Output '__MASTERCHEF_SKIP_UNLESS__'; exit 0 }; " + ps
		}
		out, err := e.runWinRMPowerShell(target, ps)
		outText := strings.TrimSpace(string(out))
		if outText == "__MASTERCHEF_SKIP_CREATES__" || outText == "__MASTERCHEF_SKIP_UNLESS__" {
			return false, true, outText, nil
		}
		if err != nil {
			return false, false, outText, err
		}
		return true, false, outText, nil
	default:
		return false, false, "", fmt.Errorf("unsupported resource type %q for winrm transport", r.Type)
	}
}

func (e *Executor) runWinRMPowerShell(target, script string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.stepTimeout)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		"pwsh",
		"-NoProfile",
		"-Command",
		"Invoke-Command -ComputerName "+quotePowerShell(target)+" -ScriptBlock { "+script+" }",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("winrm apply failed: %w: %s", err, string(out))
	}
	return out, nil
}

func isLocalWinRMHost(host config.Host) bool {
	target := strings.ToLower(strings.TrimSpace(host.Address))
	if target == "" {
		target = strings.ToLower(strings.TrimSpace(host.Name))
	}
	switch target {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func quotePowerShell(in string) string {
	in = strings.ReplaceAll(in, "'", "''")
	return "'" + in + "'"
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
