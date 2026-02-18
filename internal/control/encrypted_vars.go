package control

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type EncryptedVariableFile struct {
	Name       string    `json:"name"`
	KeyVersion int       `json:"key_version"`
	Ciphertext string    `json:"ciphertext"`
	Nonce      string    `json:"nonce"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type EncryptedVariableFileSummary struct {
	Name       string    `json:"name"`
	KeyVersion int       `json:"key_version"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type EncryptedVariableKeyStatus struct {
	CurrentKeyVersion int `json:"current_key_version"`
	FileCount         int `json:"file_count"`
}

type EncryptedVariableRotationResult struct {
	PreviousKeyVersion int       `json:"previous_key_version"`
	CurrentKeyVersion  int       `json:"current_key_version"`
	RotatedFiles       int       `json:"rotated_files"`
	RotatedAt          time.Time `json:"rotated_at"`
}

type EncryptedVariableStore struct {
	mu                sync.RWMutex
	rootDir           string
	manifestPath      string
	files             map[string]EncryptedVariableFile
	currentKeyVersion int
}

func NewEncryptedVariableStore(baseDir string) *EncryptedVariableStore {
	root := filepath.Join(baseDir, ".masterchef", "encrypted-vars")
	_ = os.MkdirAll(root, 0o755)
	s := &EncryptedVariableStore{
		rootDir:      root,
		manifestPath: filepath.Join(root, "manifest.json"),
		files:        map[string]EncryptedVariableFile{},
	}
	s.load()
	return s
}

func (s *EncryptedVariableStore) Upsert(name string, data map[string]any, passphrase string) (EncryptedVariableFileSummary, error) {
	name = normalizeEncryptedVarName(name)
	if name == "" {
		return EncryptedVariableFileSummary{}, errors.New("name is required")
	}
	if strings.TrimSpace(passphrase) == "" {
		return EncryptedVariableFileSummary{}, errors.New("passphrase is required")
	}
	if data == nil {
		data = map[string]any{}
	}
	ciphertext, nonce, err := encryptVariablePayload(data, passphrase)
	if err != nil {
		return EncryptedVariableFileSummary{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentKeyVersion <= 0 {
		s.currentKeyVersion = 1
	}
	item := EncryptedVariableFile{
		Name:       name,
		KeyVersion: s.currentKeyVersion,
		Ciphertext: ciphertext,
		Nonce:      nonce,
		UpdatedAt:  time.Now().UTC(),
	}
	s.files[name] = item
	if err := s.persistLocked(); err != nil {
		return EncryptedVariableFileSummary{}, err
	}
	return encryptedFileSummary(item), nil
}

func (s *EncryptedVariableStore) Get(name, passphrase string) (map[string]any, EncryptedVariableFileSummary, error) {
	name = normalizeEncryptedVarName(name)
	if name == "" {
		return nil, EncryptedVariableFileSummary{}, errors.New("name is required")
	}
	if strings.TrimSpace(passphrase) == "" {
		return nil, EncryptedVariableFileSummary{}, errors.New("passphrase is required")
	}
	s.mu.RLock()
	item, ok := s.files[name]
	s.mu.RUnlock()
	if !ok {
		return nil, EncryptedVariableFileSummary{}, errors.New("encrypted variable file not found")
	}
	data, err := decryptVariablePayload(item.Ciphertext, item.Nonce, passphrase)
	if err != nil {
		return nil, EncryptedVariableFileSummary{}, err
	}
	return data, encryptedFileSummary(item), nil
}

func (s *EncryptedVariableStore) Delete(name string) bool {
	name = normalizeEncryptedVarName(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.files[name]; !ok {
		return false
	}
	delete(s.files, name)
	_ = os.Remove(filepath.Join(s.rootDir, name+".json"))
	_ = s.persistManifestLocked()
	return true
}

func (s *EncryptedVariableStore) List() []EncryptedVariableFileSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]EncryptedVariableFileSummary, 0, len(s.files))
	for _, item := range s.files {
		out = append(out, encryptedFileSummary(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *EncryptedVariableStore) KeyStatus() EncryptedVariableKeyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return EncryptedVariableKeyStatus{
		CurrentKeyVersion: s.currentKeyVersion,
		FileCount:         len(s.files),
	}
}

func (s *EncryptedVariableStore) Rotate(oldPassphrase, newPassphrase string) (EncryptedVariableRotationResult, error) {
	oldPassphrase = strings.TrimSpace(oldPassphrase)
	newPassphrase = strings.TrimSpace(newPassphrase)
	if oldPassphrase == "" || newPassphrase == "" {
		return EncryptedVariableRotationResult{}, errors.New("old_passphrase and new_passphrase are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	nextVersion := s.currentKeyVersion + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	updated := make(map[string]EncryptedVariableFile, len(s.files))
	for name, item := range s.files {
		plain, err := decryptVariablePayload(item.Ciphertext, item.Nonce, oldPassphrase)
		if err != nil {
			return EncryptedVariableRotationResult{}, errors.New("failed to decrypt existing files with old passphrase")
		}
		ciphertext, nonce, err := encryptVariablePayload(plain, newPassphrase)
		if err != nil {
			return EncryptedVariableRotationResult{}, err
		}
		item.Ciphertext = ciphertext
		item.Nonce = nonce
		item.KeyVersion = nextVersion
		item.UpdatedAt = time.Now().UTC()
		updated[name] = item
	}
	s.files = updated
	prev := s.currentKeyVersion
	s.currentKeyVersion = nextVersion
	if err := s.persistLocked(); err != nil {
		return EncryptedVariableRotationResult{}, err
	}
	return EncryptedVariableRotationResult{
		PreviousKeyVersion: prev,
		CurrentKeyVersion:  s.currentKeyVersion,
		RotatedFiles:       len(updated),
		RotatedAt:          time.Now().UTC(),
	}, nil
}

func encryptedFileSummary(item EncryptedVariableFile) EncryptedVariableFileSummary {
	return EncryptedVariableFileSummary{
		Name:       item.Name,
		KeyVersion: item.KeyVersion,
		UpdatedAt:  item.UpdatedAt,
	}
}

func normalizeEncryptedVarName(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func encryptVariablePayload(data map[string]any, passphrase string) (string, string, error) {
	plain, err := json.Marshal(data)
	if err != nil {
		return "", "", err
	}
	key := sha256.Sum256([]byte(passphrase))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", err
	}
	ciphertext := gcm.Seal(nil, nonce, plain, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), base64.StdEncoding.EncodeToString(nonce), nil
}

func decryptVariablePayload(ciphertext, nonce, passphrase string) (map[string]any, error) {
	cipherBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(ciphertext))
	if err != nil {
		return nil, err
	}
	nonceBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(nonce))
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256([]byte(passphrase))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonceBytes, cipherBytes, nil)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func (s *EncryptedVariableStore) load() {
	type manifest struct {
		CurrentKeyVersion int `json:"current_key_version"`
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if raw, err := os.ReadFile(s.manifestPath); err == nil {
		var m manifest
		if json.Unmarshal(raw, &m) == nil {
			s.currentKeyVersion = m.CurrentKeyVersion
		}
	}
	files, err := filepath.Glob(filepath.Join(s.rootDir, "*.json"))
	if err != nil {
		return
	}
	for _, path := range files {
		if path == s.manifestPath {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var item EncryptedVariableFile
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		item.Name = normalizeEncryptedVarName(item.Name)
		if item.Name == "" {
			continue
		}
		s.files[item.Name] = item
	}
}

func (s *EncryptedVariableStore) persistLocked() error {
	for _, item := range s.files {
		raw, err := json.MarshalIndent(item, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(s.rootDir, item.Name+".json"), append(raw, '\n'), 0o600); err != nil {
			return err
		}
	}
	return s.persistManifestLocked()
}

func (s *EncryptedVariableStore) persistManifestLocked() error {
	type manifest struct {
		CurrentKeyVersion int `json:"current_key_version"`
	}
	raw, err := json.MarshalIndent(manifest{CurrentKeyVersion: s.currentKeyVersion}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.manifestPath, append(raw, '\n'), 0o600)
}
