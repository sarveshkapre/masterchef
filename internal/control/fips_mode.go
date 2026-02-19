package control

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type FIPSModeInput struct {
	Enabled                    bool     `json:"enabled"`
	MinRSAKeyBits              int      `json:"min_rsa_key_bits,omitempty"`
	AllowedSignatureAlgorithms []string `json:"allowed_signature_algorithms,omitempty"`
	AllowedHashAlgorithms      []string `json:"allowed_hash_algorithms,omitempty"`
	BlockLegacyTLS             bool     `json:"block_legacy_tls"`
}

type FIPSMode struct {
	Enabled                    bool      `json:"enabled"`
	MinRSAKeyBits              int       `json:"min_rsa_key_bits"`
	AllowedSignatureAlgorithms []string  `json:"allowed_signature_algorithms"`
	AllowedHashAlgorithms      []string  `json:"allowed_hash_algorithms"`
	BlockLegacyTLS             bool      `json:"block_legacy_tls"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

type FIPSValidationInput struct {
	Name               string `json:"name,omitempty"`
	SignatureAlgorithm string `json:"signature_algorithm,omitempty"`
	HashAlgorithm      string `json:"hash_algorithm,omitempty"`
	RSAKeyBits         int    `json:"rsa_key_bits,omitempty"`
	TLSVersion         string `json:"tls_version,omitempty"`
}

type FIPSValidationResult struct {
	Allowed    bool     `json:"allowed"`
	Reasons    []string `json:"reasons,omitempty"`
	ModeActive bool     `json:"mode_active"`
}

type FIPSModeStore struct {
	mu   sync.RWMutex
	mode FIPSMode
}

func NewFIPSModeStore() *FIPSModeStore {
	return &FIPSModeStore{
		mode: FIPSMode{
			Enabled:                    false,
			MinRSAKeyBits:              2048,
			AllowedSignatureAlgorithms: []string{"ecdsa-p256-sha256", "ecdsa-p384-sha384", "ed25519", "rsa-pss-sha256", "rsa-pss-sha384"},
			AllowedHashAlgorithms:      []string{"sha256", "sha384", "sha512"},
			BlockLegacyTLS:             true,
			UpdatedAt:                  time.Now().UTC(),
		},
	}
}

func (s *FIPSModeStore) Get() FIPSMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneFIPSMode(s.mode)
}

func (s *FIPSModeStore) Set(in FIPSModeInput) (FIPSMode, error) {
	mode := FIPSMode{
		Enabled:                    in.Enabled,
		MinRSAKeyBits:              in.MinRSAKeyBits,
		AllowedSignatureAlgorithms: normalizeFIPSAlgorithms(in.AllowedSignatureAlgorithms),
		AllowedHashAlgorithms:      normalizeFIPSHashes(in.AllowedHashAlgorithms),
		BlockLegacyTLS:             in.BlockLegacyTLS,
		UpdatedAt:                  time.Now().UTC(),
	}
	if mode.MinRSAKeyBits == 0 {
		mode.MinRSAKeyBits = 2048
	}
	if mode.MinRSAKeyBits < 2048 {
		return FIPSMode{}, errors.New("min_rsa_key_bits must be >= 2048")
	}
	if len(mode.AllowedSignatureAlgorithms) == 0 {
		mode.AllowedSignatureAlgorithms = []string{"ecdsa-p256-sha256", "ecdsa-p384-sha384", "ed25519", "rsa-pss-sha256", "rsa-pss-sha384"}
	}
	if len(mode.AllowedHashAlgorithms) == 0 {
		mode.AllowedHashAlgorithms = []string{"sha256", "sha384", "sha512"}
	}
	s.mu.Lock()
	s.mode = mode
	s.mu.Unlock()
	return cloneFIPSMode(mode), nil
}

func (s *FIPSModeStore) Validate(in FIPSValidationInput) FIPSValidationResult {
	mode := s.Get()
	if !mode.Enabled {
		return FIPSValidationResult{Allowed: true, ModeActive: false}
	}
	reasons := make([]string, 0, 4)
	sig := strings.ToLower(strings.TrimSpace(in.SignatureAlgorithm))
	hash := strings.ToLower(strings.TrimSpace(in.HashAlgorithm))
	tls := strings.ToLower(strings.TrimSpace(in.TLSVersion))

	if sig == "" {
		reasons = append(reasons, "signature_algorithm is required when fips mode is enabled")
	} else if !sliceContains(mode.AllowedSignatureAlgorithms, sig) {
		reasons = append(reasons, "signature algorithm is not allowed in fips mode")
	}
	if hash != "" && !sliceContains(mode.AllowedHashAlgorithms, hash) {
		reasons = append(reasons, "hash algorithm is not allowed in fips mode")
	}
	if strings.HasPrefix(sig, "rsa") && in.RSAKeyBits > 0 && in.RSAKeyBits < mode.MinRSAKeyBits {
		reasons = append(reasons, "rsa key length below fips minimum")
	}
	if mode.BlockLegacyTLS && tls != "" && tls != "tls1.2" && tls != "tls1.3" {
		reasons = append(reasons, "legacy tls versions are not allowed in fips mode")
	}
	return FIPSValidationResult{
		Allowed:    len(reasons) == 0,
		Reasons:    reasons,
		ModeActive: true,
	}
}

func normalizeFIPSAlgorithms(in []string) []string {
	in = normalizeStringSlice(in)
	if len(in) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"ecdsa-p256-sha256": {},
		"ecdsa-p384-sha384": {},
		"ed25519":           {},
		"rsa-pss-sha256":    {},
		"rsa-pss-sha384":    {},
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		alg := strings.ToLower(strings.TrimSpace(item))
		if _, ok := allowed[alg]; !ok {
			continue
		}
		if _, ok := seen[alg]; ok {
			continue
		}
		seen[alg] = struct{}{}
		out = append(out, alg)
	}
	sort.Strings(out)
	return out
}

func normalizeFIPSHashes(in []string) []string {
	in = normalizeStringSlice(in)
	if len(in) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"sha256": {},
		"sha384": {},
		"sha512": {},
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		hash := strings.ToLower(strings.TrimSpace(item))
		if _, ok := allowed[hash]; !ok {
			continue
		}
		if _, ok := seen[hash]; ok {
			continue
		}
		seen[hash] = struct{}{}
		out = append(out, hash)
	}
	sort.Strings(out)
	return out
}

func cloneFIPSMode(in FIPSMode) FIPSMode {
	out := in
	out.AllowedSignatureAlgorithms = append([]string{}, in.AllowedSignatureAlgorithms...)
	out.AllowedHashAlgorithms = append([]string{}, in.AllowedHashAlgorithms...)
	return out
}
