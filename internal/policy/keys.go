package policy

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
)

func SavePrivateKey(path string, key ed25519.PrivateKey) error {
	return os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)), 0o600)
}

func SavePublicKey(path string, key ed25519.PublicKey) error {
	return os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)), 0o644)
}

func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(string(b))
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key size")
	}
	return ed25519.PrivateKey(raw), nil
}

func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(string(b))
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key size")
	}
	return ed25519.PublicKey(raw), nil
}
