package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type AccessibilityProfile struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	KeyboardOnly          bool      `json:"keyboard_only"`
	ScreenReaderOptimized bool      `json:"screen_reader_optimized"`
	HighContrast          bool      `json:"high_contrast"`
	ReducedMotion         bool      `json:"reduced_motion"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type AccessibilityStore struct {
	mu       sync.RWMutex
	profiles map[string]*AccessibilityProfile
	activeID string
}

func NewAccessibilityStore() *AccessibilityStore {
	now := time.Now().UTC()
	profiles := map[string]*AccessibilityProfile{
		"default": {
			ID:                    "default",
			Name:                  "Default",
			KeyboardOnly:          false,
			ScreenReaderOptimized: false,
			HighContrast:          false,
			ReducedMotion:         false,
			UpdatedAt:             now,
		},
		"keyboard-first": {
			ID:                    "keyboard-first",
			Name:                  "Keyboard First",
			KeyboardOnly:          true,
			ScreenReaderOptimized: true,
			HighContrast:          false,
			ReducedMotion:         false,
			UpdatedAt:             now,
		},
		"high-contrast": {
			ID:                    "high-contrast",
			Name:                  "High Contrast",
			KeyboardOnly:          true,
			ScreenReaderOptimized: true,
			HighContrast:          true,
			ReducedMotion:         true,
			UpdatedAt:             now,
		},
	}
	return &AccessibilityStore{
		profiles: profiles,
		activeID: "default",
	}
}

func (s *AccessibilityStore) ListProfiles() []AccessibilityProfile {
	s.mu.RLock()
	out := make([]AccessibilityProfile, 0, len(s.profiles))
	for _, item := range s.profiles {
		out = append(out, *item)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *AccessibilityStore) UpsertProfile(in AccessibilityProfile) (AccessibilityProfile, error) {
	id := normalizeProfileID(in.ID)
	if id == "" {
		return AccessibilityProfile{}, errors.New("id is required")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = profileDisplayName(id)
	}
	if !in.KeyboardOnly && !in.ScreenReaderOptimized && !in.HighContrast && !in.ReducedMotion {
		return AccessibilityProfile{}, errors.New("profile must enable at least one accessibility mode")
	}
	item := AccessibilityProfile{
		ID:                    id,
		Name:                  name,
		KeyboardOnly:          in.KeyboardOnly,
		ScreenReaderOptimized: in.ScreenReaderOptimized,
		HighContrast:          in.HighContrast,
		ReducedMotion:         in.ReducedMotion,
		UpdatedAt:             time.Now().UTC(),
	}
	s.mu.Lock()
	s.profiles[id] = &item
	s.mu.Unlock()
	return item, nil
}

func (s *AccessibilityStore) SetActive(id string) (AccessibilityProfile, error) {
	id = normalizeProfileID(id)
	if id == "" {
		return AccessibilityProfile{}, errors.New("profile id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.profiles[id]
	if !ok {
		return AccessibilityProfile{}, errors.New("profile not found")
	}
	s.activeID = item.ID
	return *item, nil
}

func (s *AccessibilityStore) ActiveProfile() AccessibilityProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item := s.profiles[s.activeID]
	if item == nil {
		return AccessibilityProfile{}
	}
	return *item
}

func normalizeProfileID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.ReplaceAll(id, "_", "-")
	id = strings.ReplaceAll(id, " ", "-")
	return id
}

func profileDisplayName(id string) string {
	parts := strings.Split(id, "-")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		first := strings.ToUpper(part[:1])
		if len(part) == 1 {
			out = append(out, first)
			continue
		}
		out = append(out, first+part[1:])
	}
	return strings.Join(out, " ")
}
