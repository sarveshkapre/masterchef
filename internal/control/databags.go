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
	"sort"
	"strings"
	"sync"
	"time"
)

type DataBagItem struct {
	Bag        string         `json:"bag"`
	Item       string         `json:"item"`
	Encrypted  bool           `json:"encrypted"`
	Data       map[string]any `json:"data,omitempty"`
	Ciphertext string         `json:"ciphertext,omitempty"`
	Nonce      string         `json:"nonce,omitempty"`
	Tags       []string       `json:"tags,omitempty"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

type DataBagItemSummary struct {
	Bag       string         `json:"bag"`
	Item      string         `json:"item"`
	Encrypted bool           `json:"encrypted"`
	Data      map[string]any `json:"data,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type DataBagSearchRequest struct {
	Bag        string `json:"bag,omitempty"`
	Field      string `json:"field,omitempty"` // dot-path
	Equals     string `json:"equals,omitempty"`
	Contains   string `json:"contains,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type DataBagStore struct {
	mu   sync.RWMutex
	bags map[string]map[string]*DataBagItem
}

func NewDataBagStore() *DataBagStore {
	return &DataBagStore{
		bags: map[string]map[string]*DataBagItem{},
	}
}

func (s *DataBagStore) Upsert(bag, item string, data map[string]any, encrypted bool, passphrase string, tags []string) (DataBagItem, error) {
	bag = normalizeDataBagName(bag)
	item = normalizeDataBagName(item)
	if bag == "" || item == "" {
		return DataBagItem{}, errors.New("bag and item are required")
	}
	if data == nil {
		data = map[string]any{}
	}
	entry := DataBagItem{
		Bag:       bag,
		Item:      item,
		Encrypted: encrypted,
		Tags:      normalizeTags(tags),
		UpdatedAt: time.Now().UTC(),
	}
	if encrypted {
		if strings.TrimSpace(passphrase) == "" {
			return DataBagItem{}, errors.New("passphrase is required for encrypted items")
		}
		ciphertext, nonce, err := encryptDataBagData(data, passphrase)
		if err != nil {
			return DataBagItem{}, err
		}
		entry.Ciphertext = ciphertext
		entry.Nonce = nonce
	} else {
		entry.Data = cloneMap(data)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bags[bag] == nil {
		s.bags[bag] = map[string]*DataBagItem{}
	}
	cp := entry
	s.bags[bag][item] = &cp
	return cloneDataBagItem(entry), nil
}

func (s *DataBagStore) Get(bag, item, passphrase string) (DataBagItem, error) {
	bag = normalizeDataBagName(bag)
	item = normalizeDataBagName(item)
	s.mu.RLock()
	bagItems := s.bags[bag]
	entry := bagItems[item]
	s.mu.RUnlock()
	if entry == nil {
		return DataBagItem{}, errors.New("data bag item not found")
	}
	out := cloneDataBagItem(*entry)
	if out.Encrypted {
		if strings.TrimSpace(passphrase) == "" {
			return DataBagItem{}, errors.New("passphrase is required for encrypted item retrieval")
		}
		plain, err := decryptDataBagData(out.Ciphertext, out.Nonce, passphrase)
		if err != nil {
			return DataBagItem{}, err
		}
		out.Data = plain
	}
	return out, nil
}

func (s *DataBagStore) Delete(bag, item string) bool {
	bag = normalizeDataBagName(bag)
	item = normalizeDataBagName(item)
	s.mu.Lock()
	defer s.mu.Unlock()
	bagItems := s.bags[bag]
	if bagItems == nil {
		return false
	}
	if _, ok := bagItems[item]; !ok {
		return false
	}
	delete(bagItems, item)
	if len(bagItems) == 0 {
		delete(s.bags, bag)
	}
	return true
}

func (s *DataBagStore) ListBags() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.bags))
	for bag := range s.bags {
		out = append(out, bag)
	}
	sort.Strings(out)
	return out
}

func (s *DataBagStore) ListSummaries() []DataBagItemSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DataBagItemSummary, 0)
	for bag, items := range s.bags {
		for item, entry := range items {
			summary := DataBagItemSummary{
				Bag:       bag,
				Item:      item,
				Encrypted: entry.Encrypted,
				Tags:      append([]string{}, entry.Tags...),
				UpdatedAt: entry.UpdatedAt,
			}
			if !entry.Encrypted {
				summary.Data = cloneMap(entry.Data)
			}
			out = append(out, summary)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bag != out[j].Bag {
			return out[i].Bag < out[j].Bag
		}
		return out[i].Item < out[j].Item
	})
	return out
}

func (s *DataBagStore) Search(req DataBagSearchRequest) ([]DataBagItemSummary, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	bagFilter := normalizeDataBagName(req.Bag)
	fieldPath := strings.TrimSpace(req.Field)
	equals := strings.TrimSpace(req.Equals)
	contains := strings.ToLower(strings.TrimSpace(req.Contains))
	passphrase := strings.TrimSpace(req.Passphrase)

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]DataBagItemSummary, 0)
	for bag, items := range s.bags {
		if bagFilter != "" && bag != bagFilter {
			continue
		}
		for itemName, entry := range items {
			data := map[string]any{}
			if entry.Encrypted {
				if passphrase == "" {
					continue
				}
				plain, err := decryptDataBagData(entry.Ciphertext, entry.Nonce, passphrase)
				if err != nil {
					return nil, err
				}
				data = plain
			} else {
				data = cloneMap(entry.Data)
			}
			if !matchStructuredData(data, fieldPath, equals, contains) {
				continue
			}
			out = append(out, DataBagItemSummary{
				Bag:       bag,
				Item:      itemName,
				Encrypted: entry.Encrypted,
				Data:      data,
				Tags:      append([]string{}, entry.Tags...),
				UpdatedAt: entry.UpdatedAt,
			})
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func matchStructuredData(data map[string]any, fieldPath, equals, contains string) bool {
	if fieldPath == "" && equals == "" && contains == "" {
		return true
	}
	v, ok := nestedField(data, fieldPath)
	if !ok {
		return false
	}
	raw := strings.TrimSpace(valueToString(v))
	if equals != "" && raw != equals {
		return false
	}
	if contains != "" && !strings.Contains(strings.ToLower(raw), contains) {
		return false
	}
	return true
}

func nestedField(m map[string]any, fieldPath string) (any, bool) {
	fieldPath = strings.TrimSpace(fieldPath)
	if fieldPath == "" {
		return m, true
	}
	parts := strings.Split(fieldPath, ".")
	var cur any = m
	for _, part := range parts {
		node, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := node[part]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func valueToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func encryptDataBagData(data map[string]any, passphrase string) (string, string, error) {
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

func decryptDataBagData(ciphertext, nonce, passphrase string) (map[string]any, error) {
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

func normalizeDataBagName(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	b, _ := json.Marshal(in)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func cloneDataBagItem(in DataBagItem) DataBagItem {
	out := in
	out.Data = cloneMap(in.Data)
	out.Tags = append([]string{}, in.Tags...)
	return out
}
