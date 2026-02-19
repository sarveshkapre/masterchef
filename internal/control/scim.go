package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type SCIMRole struct {
	ID          string    `json:"id"`
	ExternalID  string    `json:"external_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SCIMRoleInput struct {
	ExternalID  string `json:"external_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type SCIMTeam struct {
	ID         string    `json:"id"`
	ExternalID string    `json:"external_id"`
	Name       string    `json:"name"`
	Members    []string  `json:"members,omitempty"`
	Roles      []string  `json:"roles,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SCIMTeamInput struct {
	ExternalID string   `json:"external_id"`
	Name       string   `json:"name"`
	Members    []string `json:"members,omitempty"`
	Roles      []string `json:"roles,omitempty"`
}

type SCIMStore struct {
	mu         sync.RWMutex
	nextRoleID int64
	nextTeamID int64
	roles      map[string]*SCIMRole
	rolesByExt map[string]string
	teams      map[string]*SCIMTeam
	teamsByExt map[string]string
}

func NewSCIMStore() *SCIMStore {
	return &SCIMStore{
		roles:      map[string]*SCIMRole{},
		rolesByExt: map[string]string{},
		teams:      map[string]*SCIMTeam{},
		teamsByExt: map[string]string{},
	}
}

func (s *SCIMStore) UpsertRole(in SCIMRoleInput) (SCIMRole, error) {
	externalID := strings.TrimSpace(in.ExternalID)
	name := strings.TrimSpace(in.Name)
	if externalID == "" || name == "" {
		return SCIMRole{}, errors.New("external_id and name are required")
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if existingID, ok := s.rolesByExt[externalID]; ok {
		item := s.roles[existingID]
		item.Name = name
		item.Description = strings.TrimSpace(in.Description)
		item.UpdatedAt = now
		return cloneSCIMRole(*item), nil
	}
	s.nextRoleID++
	item := SCIMRole{
		ID:          "scim-role-" + itoa(s.nextRoleID),
		ExternalID:  externalID,
		Name:        name,
		Description: strings.TrimSpace(in.Description),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.roles[item.ID] = &item
	s.rolesByExt[externalID] = item.ID
	return cloneSCIMRole(item), nil
}

func (s *SCIMStore) ListRoles() []SCIMRole {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SCIMRole, 0, len(s.roles))
	for _, item := range s.roles {
		out = append(out, cloneSCIMRole(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *SCIMStore) GetRole(id string) (SCIMRole, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.roles[strings.TrimSpace(id)]
	if !ok {
		return SCIMRole{}, false
	}
	return cloneSCIMRole(*item), true
}

func (s *SCIMStore) UpsertTeam(in SCIMTeamInput) (SCIMTeam, error) {
	externalID := strings.TrimSpace(in.ExternalID)
	name := strings.TrimSpace(in.Name)
	if externalID == "" || name == "" {
		return SCIMTeam{}, errors.New("external_id and name are required")
	}
	members := normalizeStringSlice(in.Members)
	roles := normalizeStringSlice(in.Roles)
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if existingID, ok := s.teamsByExt[externalID]; ok {
		item := s.teams[existingID]
		item.Name = name
		item.Members = members
		item.Roles = roles
		item.UpdatedAt = now
		return cloneSCIMTeam(*item), nil
	}
	s.nextTeamID++
	item := SCIMTeam{
		ID:         "scim-team-" + itoa(s.nextTeamID),
		ExternalID: externalID,
		Name:       name,
		Members:    members,
		Roles:      roles,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.teams[item.ID] = &item
	s.teamsByExt[externalID] = item.ID
	return cloneSCIMTeam(item), nil
}

func (s *SCIMStore) ListTeams() []SCIMTeam {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SCIMTeam, 0, len(s.teams))
	for _, item := range s.teams {
		out = append(out, cloneSCIMTeam(*item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *SCIMStore) GetTeam(id string) (SCIMTeam, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.teams[strings.TrimSpace(id)]
	if !ok {
		return SCIMTeam{}, false
	}
	return cloneSCIMTeam(*item), true
}

func cloneSCIMRole(in SCIMRole) SCIMRole {
	return in
}

func cloneSCIMTeam(in SCIMTeam) SCIMTeam {
	out := in
	out.Members = append([]string{}, in.Members...)
	out.Roles = append([]string{}, in.Roles...)
	return out
}
