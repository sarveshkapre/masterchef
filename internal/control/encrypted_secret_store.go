package control

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type EncryptedSecretEnvelope struct {
	DEKCipher     string `json:"dek_cipher"`
	ContentCipher string `json:"content_cipher"`
}

type EncryptedSecretItem struct {
	Name          string                  `json:"name"`
	Version       int                     `json:"version"`
	Labels        map[string]string       `json:"labels,omitempty"`
	CreatedAt     time.Time               `json:"created_at"`
	UpdatedAt     time.Time               `json:"updated_at"`
	ExpiresAt     time.Time               `json:"expires_at,omitempty"`
	RotationCount int                     `json:"rotation_count"`
	Envelope      EncryptedSecretEnvelope `json:"envelope"`
}

type EncryptedSecretUpsertInput struct {
	Name       string            `json:"name"`
	Value      string            `json:"value"`
	Labels     map[string]string `json:"labels,omitempty"`
	TTLSeconds int               `json:"ttl_seconds,omitempty"`
	ExpiresAt  time.Time         `json:"expires_at,omitempty"`
}

type EncryptedSecretRotateInput struct {
	Value            string    `json:"value,omitempty"`
	ExtendTTLSeconds int       `json:"extend_ttl_seconds,omitempty"`
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
}

type EncryptedSecretResolveResult struct {
	Name      string    `json:"name"`
	Version   int       `json:"version"`
	Value     string    `json:"value"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type encryptedSecretRecord struct {
	item       EncryptedSecretItem
	content    []byte
	contentN   []byte
	wrappedDEK []byte
	wrapN      []byte
}

type EncryptedSecretStore struct {
	mu     sync.RWMutex
	now    func() time.Time
	items  map[string]*encryptedSecretRecord
	kekGCM cipher.AEAD
}

func NewEncryptedSecretStore() *EncryptedSecretStore {
	kek := make([]byte, 32)
	_, _ = io.ReadFull(rand.Reader, kek)
	block, err := aes.NewCipher(kek)
	if err != nil {
		panic(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return &EncryptedSecretStore{
		now:    func() time.Time { return time.Now().UTC() },
		items:  map[string]*encryptedSecretRecord{},
		kekGCM: aead,
	}
}

func (s *EncryptedSecretStore) Upsert(in EncryptedSecretUpsertInput) (EncryptedSecretItem, error) {
	name := strings.ToLower(strings.TrimSpace(in.Name))
	value := strings.TrimSpace(in.Value)
	if name == "" || value == "" {
		return EncryptedSecretItem{}, errors.New("name and value are required")
	}
	now := s.now()
	expiresAt, err := resolveSecretExpiry(now, in.ExpiresAt, in.TTLSeconds)
	if err != nil {
		return EncryptedSecretItem{}, err
	}
	labels := cloneEncryptedSecretLabels(in.Labels)
	content, contentNonce, wrappedDEK, wrapNonce, err := s.seal(name, value)
	if err != nil {
		return EncryptedSecretItem{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	version := 1
	createdAt := now
	rotationCount := 0
	if existing, ok := s.items[name]; ok {
		version = existing.item.Version + 1
		createdAt = existing.item.CreatedAt
		rotationCount = existing.item.RotationCount
	}
	item := EncryptedSecretItem{
		Name:      name,
		Version:   version,
		Labels:    labels,
		CreatedAt: createdAt,
		UpdatedAt: now,
		ExpiresAt: expiresAt,
		Envelope: EncryptedSecretEnvelope{
			DEKCipher:     "aes-256-gcm",
			ContentCipher: "aes-256-gcm",
		},
		RotationCount: rotationCount,
	}
	s.items[name] = &encryptedSecretRecord{
		item:       item,
		content:    content,
		contentN:   contentNonce,
		wrappedDEK: wrappedDEK,
		wrapN:      wrapNonce,
	}
	return cloneEncryptedSecretItem(item), nil
}

func (s *EncryptedSecretStore) List() []EncryptedSecretItem {
	s.mu.RLock()
	out := make([]EncryptedSecretItem, 0, len(s.items))
	for _, record := range s.items {
		out = append(out, cloneEncryptedSecretItem(record.item))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *EncryptedSecretStore) Get(name string) (EncryptedSecretItem, bool) {
	s.mu.RLock()
	record, ok := s.items[strings.ToLower(strings.TrimSpace(name))]
	s.mu.RUnlock()
	if !ok {
		return EncryptedSecretItem{}, false
	}
	return cloneEncryptedSecretItem(record.item), true
}

func (s *EncryptedSecretStore) Resolve(name string) (EncryptedSecretResolveResult, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return EncryptedSecretResolveResult{}, errors.New("name is required")
	}
	s.mu.RLock()
	record, ok := s.items[normalized]
	if !ok {
		s.mu.RUnlock()
		return EncryptedSecretResolveResult{}, errors.New("secret not found")
	}
	item := cloneEncryptedSecretItem(record.item)
	content := append([]byte{}, record.content...)
	contentNonce := append([]byte{}, record.contentN...)
	wrappedDEK := append([]byte{}, record.wrappedDEK...)
	wrapNonce := append([]byte{}, record.wrapN...)
	s.mu.RUnlock()

	now := s.now()
	if !item.ExpiresAt.IsZero() && !now.Before(item.ExpiresAt) {
		return EncryptedSecretResolveResult{}, errors.New("secret expired")
	}
	plaintext, err := s.open(normalized, content, contentNonce, wrappedDEK, wrapNonce)
	if err != nil {
		return EncryptedSecretResolveResult{}, err
	}
	return EncryptedSecretResolveResult{
		Name:      item.Name,
		Version:   item.Version,
		Value:     plaintext,
		ExpiresAt: item.ExpiresAt,
	}, nil
}

func (s *EncryptedSecretStore) Rotate(name string, in EncryptedSecretRotateInput) (EncryptedSecretItem, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return EncryptedSecretItem{}, errors.New("name is required")
	}
	s.mu.RLock()
	record, ok := s.items[normalized]
	if !ok {
		s.mu.RUnlock()
		return EncryptedSecretItem{}, errors.New("secret not found")
	}
	item := cloneEncryptedSecretItem(record.item)
	content := append([]byte{}, record.content...)
	contentNonce := append([]byte{}, record.contentN...)
	wrappedDEK := append([]byte{}, record.wrappedDEK...)
	wrapNonce := append([]byte{}, record.wrapN...)
	s.mu.RUnlock()

	now := s.now()
	if !item.ExpiresAt.IsZero() && !now.Before(item.ExpiresAt) {
		return EncryptedSecretItem{}, errors.New("secret expired")
	}

	newValue := strings.TrimSpace(in.Value)
	if newValue == "" {
		plaintext, err := s.open(normalized, content, contentNonce, wrappedDEK, wrapNonce)
		if err != nil {
			return EncryptedSecretItem{}, err
		}
		newValue = plaintext
	}
	expiresAt := item.ExpiresAt
	if !in.ExpiresAt.IsZero() || in.ExtendTTLSeconds != 0 {
		next, err := resolveSecretExpiry(now, in.ExpiresAt, in.ExtendTTLSeconds)
		if err != nil {
			return EncryptedSecretItem{}, err
		}
		expiresAt = next
	}
	sealed, sealedNonce, nextWrappedDEK, nextWrapNonce, err := s.seal(normalized, newValue)
	if err != nil {
		return EncryptedSecretItem{}, err
	}

	item.Version++
	item.UpdatedAt = now
	item.ExpiresAt = expiresAt
	item.RotationCount++

	s.mu.Lock()
	s.items[normalized] = &encryptedSecretRecord{
		item:       item,
		content:    sealed,
		contentN:   sealedNonce,
		wrappedDEK: nextWrappedDEK,
		wrapN:      nextWrapNonce,
	}
	s.mu.Unlock()
	return cloneEncryptedSecretItem(item), nil
}

func (s *EncryptedSecretStore) Expired() []EncryptedSecretItem {
	now := s.now()
	s.mu.RLock()
	out := make([]EncryptedSecretItem, 0)
	for _, record := range s.items {
		if !record.item.ExpiresAt.IsZero() && !now.Before(record.item.ExpiresAt) {
			out = append(out, cloneEncryptedSecretItem(record.item))
		}
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *EncryptedSecretStore) seal(name, value string) ([]byte, []byte, []byte, []byte, error) {
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, nil, nil, nil, err
	}
	contentBlock, err := aes.NewCipher(dek)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	contentAEAD, err := cipher.NewGCM(contentBlock)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	contentNonce := make([]byte, contentAEAD.NonceSize())
	if _, err := io.ReadFull(rand.Reader, contentNonce); err != nil {
		return nil, nil, nil, nil, err
	}
	content := contentAEAD.Seal(nil, contentNonce, []byte(value), []byte(name))

	wrapNonce := make([]byte, s.kekGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, wrapNonce); err != nil {
		return nil, nil, nil, nil, err
	}
	wrappedDEK := s.kekGCM.Seal(nil, wrapNonce, dek, []byte(name))
	return content, contentNonce, wrappedDEK, wrapNonce, nil
}

func (s *EncryptedSecretStore) open(name string, content, contentNonce, wrappedDEK, wrapNonce []byte) (string, error) {
	dek, err := s.kekGCM.Open(nil, wrapNonce, wrappedDEK, []byte(name))
	if err != nil {
		return "", errors.New("failed to unwrap data encryption key")
	}
	contentBlock, err := aes.NewCipher(dek)
	if err != nil {
		return "", err
	}
	contentAEAD, err := cipher.NewGCM(contentBlock)
	if err != nil {
		return "", err
	}
	plaintext, err := contentAEAD.Open(nil, contentNonce, content, []byte(name))
	if err != nil {
		return "", errors.New("failed to decrypt secret content")
	}
	return string(plaintext), nil
}

func resolveSecretExpiry(now, absolute time.Time, ttlSeconds int) (time.Time, error) {
	if !absolute.IsZero() && ttlSeconds > 0 {
		return time.Time{}, errors.New("expires_at and ttl_seconds are mutually exclusive")
	}
	if !absolute.IsZero() {
		if !absolute.After(now) {
			return time.Time{}, errors.New("expires_at must be in the future")
		}
		return absolute.UTC(), nil
	}
	if ttlSeconds < 0 {
		return time.Time{}, errors.New("ttl_seconds cannot be negative")
	}
	if ttlSeconds == 0 {
		return time.Time{}, nil
	}
	return now.Add(time.Duration(ttlSeconds) * time.Second).UTC(), nil
}

func cloneEncryptedSecretItem(in EncryptedSecretItem) EncryptedSecretItem {
	in.Labels = cloneEncryptedSecretLabels(in.Labels)
	return in
}

func cloneEncryptedSecretLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		out[k] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
