package policy

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
)

type Bundle struct {
	ConfigPath string `json:"config_path"`
	ConfigSHA  string `json:"config_sha"`
	Signature  string `json:"signature,omitempty"`
}

func Build(configPath string) (*Bundle, error) {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	sum := sha256.Sum256(b)
	return &Bundle{
		ConfigPath: configPath,
		ConfigSHA:  base64.StdEncoding.EncodeToString(sum[:]),
	}, nil
}

func (b *Bundle) Sign(privateKey ed25519.PrivateKey) error {
	if len(privateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid private key size")
	}
	msg := []byte(b.ConfigPath + ":" + b.ConfigSHA)
	sig := ed25519.Sign(privateKey, msg)
	b.Signature = base64.StdEncoding.EncodeToString(sig)
	return nil
}

func (b *Bundle) Verify(publicKey ed25519.PublicKey) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size")
	}
	rawSig, err := base64.StdEncoding.DecodeString(b.Signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	msg := []byte(b.ConfigPath + ":" + b.ConfigSHA)
	if !ed25519.Verify(publicKey, msg, rawSig) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func GenerateKeypair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func SaveBundle(path string, bundle *Bundle) error {
	b, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bundle: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write bundle: %w", err)
	}
	return nil
}

func LoadBundle(path string) (*Bundle, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out Bundle
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
