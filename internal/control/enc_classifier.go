package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type ENCProvider struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Endpoint       string            `json:"endpoint"`
	Headers        map[string]string `json:"headers,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	Enabled        bool              `json:"enabled"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type ENCProviderInput struct {
	Name           string            `json:"name"`
	Endpoint       string            `json:"endpoint"`
	Headers        map[string]string `json:"headers,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Enabled        bool              `json:"enabled"`
}

type ENCClassifyInput struct {
	ProviderID string         `json:"provider_id"`
	Node       string         `json:"node"`
	Facts      map[string]any `json:"facts,omitempty"`
	Labels     map[string]any `json:"labels,omitempty"`
}

type ENCClassifyOutput struct {
	ProviderID  string         `json:"provider_id"`
	Node        string         `json:"node"`
	Classes     []string       `json:"classes,omitempty"`
	RunList     []string       `json:"run_list,omitempty"`
	Environment string         `json:"environment,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	Source      string         `json:"source"`
	ReceivedAt  time.Time      `json:"received_at"`
}

type encRemoteResponse struct {
	Classes     []string       `json:"classes"`
	RunList     []string       `json:"run_list"`
	Environment string         `json:"environment"`
	Attributes  map[string]any `json:"attributes"`
}

type ENCProviderStore struct {
	mu     sync.RWMutex
	nextID int64
	items  map[string]*ENCProvider
	client *http.Client
}

func NewENCProviderStore() *ENCProviderStore {
	return &ENCProviderStore{
		items:  map[string]*ENCProvider{},
		client: &http.Client{},
	}
}

func (s *ENCProviderStore) Upsert(in ENCProviderInput) (ENCProvider, error) {
	name := strings.TrimSpace(in.Name)
	endpoint := strings.TrimSpace(in.Endpoint)
	if name == "" || endpoint == "" {
		return ENCProvider{}, errors.New("name and endpoint are required")
	}
	timeout := in.TimeoutSeconds
	if timeout <= 0 {
		timeout = 5
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if strings.EqualFold(item.Name, name) {
			item.Endpoint = endpoint
			item.Headers = cloneStringMap(in.Headers)
			item.TimeoutSeconds = timeout
			item.Enabled = in.Enabled
			item.UpdatedAt = now
			return cloneENCProvider(*item), nil
		}
	}
	s.nextID++
	item := &ENCProvider{
		ID:             "enc-provider-" + itoa(s.nextID),
		Name:           name,
		Endpoint:       endpoint,
		Headers:        cloneStringMap(in.Headers),
		TimeoutSeconds: timeout,
		Enabled:        in.Enabled,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.items[item.ID] = item
	return cloneENCProvider(*item), nil
}

func (s *ENCProviderStore) List() []ENCProvider {
	s.mu.RLock()
	out := make([]ENCProvider, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, cloneENCProvider(*item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *ENCProviderStore) Get(id string) (ENCProvider, bool) {
	s.mu.RLock()
	item, ok := s.items[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return ENCProvider{}, false
	}
	return cloneENCProvider(*item), true
}

func (s *ENCProviderStore) SetEnabled(id string, enabled bool) (ENCProvider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[strings.TrimSpace(id)]
	if !ok {
		return ENCProvider{}, errors.New("enc provider not found")
	}
	item.Enabled = enabled
	item.UpdatedAt = time.Now().UTC()
	return cloneENCProvider(*item), nil
}

func (s *ENCProviderStore) Classify(in ENCClassifyInput) (ENCClassifyOutput, error) {
	providerID := strings.TrimSpace(in.ProviderID)
	node := strings.TrimSpace(in.Node)
	if providerID == "" || node == "" {
		return ENCClassifyOutput{}, errors.New("provider_id and node are required")
	}
	provider, ok := s.Get(providerID)
	if !ok {
		return ENCClassifyOutput{}, errors.New("enc provider not found")
	}
	if !provider.Enabled {
		return ENCClassifyOutput{}, errors.New("enc provider is disabled")
	}

	payload := map[string]any{
		"node":   node,
		"facts":  cloneAnyMap(in.Facts),
		"labels": cloneAnyMap(in.Labels),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ENCClassifyOutput{}, err
	}
	req, err := http.NewRequest(http.MethodPost, provider.Endpoint, bytes.NewReader(body))
	if err != nil {
		return ENCClassifyOutput{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range provider.Headers {
		req.Header.Set(k, v)
	}

	client := *s.client
	client.Timeout = time.Duration(provider.TimeoutSeconds) * time.Second
	resp, err := client.Do(req)
	if err != nil {
		return ENCClassifyOutput{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if len(msg) == 0 {
			return ENCClassifyOutput{}, errors.New("enc provider request failed")
		}
		return ENCClassifyOutput{}, errors.New("enc provider request failed: " + strings.TrimSpace(string(msg)))
	}

	var remote encRemoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&remote); err != nil {
		return ENCClassifyOutput{}, err
	}
	classes := normalizeStringSlice(remote.Classes)
	runList := normalizeStringSlice(remote.RunList)
	out := ENCClassifyOutput{
		ProviderID:  provider.ID,
		Node:        node,
		Classes:     classes,
		RunList:     runList,
		Environment: strings.TrimSpace(remote.Environment),
		Attributes:  cloneAnyMap(remote.Attributes),
		Source:      provider.Name,
		ReceivedAt:  time.Now().UTC(),
	}
	return out, nil
}

func cloneENCProvider(in ENCProvider) ENCProvider {
	out := in
	out.Headers = cloneStringMap(in.Headers)
	return out
}
