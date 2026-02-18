package release

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
)

type SignedSBOM struct {
	SBOM      SBOM   `json:"sbom"`
	Signature string `json:"signature"`
}

func SignSBOM(sbom SBOM, priv ed25519.PrivateKey) (SignedSBOM, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return SignedSBOM{}, errors.New("invalid private key")
	}
	b, err := CanonicalSBOMBytes(sbom)
	if err != nil {
		return SignedSBOM{}, err
	}
	sig := ed25519.Sign(priv, b)
	return SignedSBOM{SBOM: sbom, Signature: base64.StdEncoding.EncodeToString(sig)}, nil
}

func VerifySignedSBOM(s SignedSBOM, pub ed25519.PublicKey) error {
	if len(pub) != ed25519.PublicKeySize {
		return errors.New("invalid public key")
	}
	rawSig, err := base64.StdEncoding.DecodeString(s.Signature)
	if err != nil {
		return err
	}
	b, err := CanonicalSBOMBytes(s.SBOM)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, b, rawSig) {
		return errors.New("invalid signature")
	}
	return nil
}
