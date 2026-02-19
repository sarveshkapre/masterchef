package control

import "testing"

func TestFIPSModeStoreSetAndValidate(t *testing.T) {
	store := NewFIPSModeStore()
	if store.Get().Enabled {
		t.Fatalf("expected fips mode to be disabled by default")
	}
	mode, err := store.Set(FIPSModeInput{Enabled: true, BlockLegacyTLS: true})
	if err != nil {
		t.Fatalf("set fips mode failed: %v", err)
	}
	if !mode.Enabled || mode.MinRSAKeyBits < 2048 {
		t.Fatalf("unexpected mode %+v", mode)
	}

	ok := store.Validate(FIPSValidationInput{
		SignatureAlgorithm: "rsa-pss-sha256",
		HashAlgorithm:      "sha256",
		RSAKeyBits:         3072,
		TLSVersion:         "tls1.2",
	})
	if !ok.Allowed {
		t.Fatalf("expected compliant crypto profile to pass: %+v", ok)
	}

	bad := store.Validate(FIPSValidationInput{
		SignatureAlgorithm: "rsa-pss-sha256",
		HashAlgorithm:      "sha1",
		RSAKeyBits:         1024,
		TLSVersion:         "tls1.0",
	})
	if bad.Allowed {
		t.Fatalf("expected non-fips profile to fail")
	}
	if len(bad.Reasons) < 2 {
		t.Fatalf("expected multiple denial reasons, got %+v", bad.Reasons)
	}
}

func TestFIPSModeStoreValidationGuards(t *testing.T) {
	store := NewFIPSModeStore()
	if _, err := store.Set(FIPSModeInput{Enabled: true, MinRSAKeyBits: 1024}); err == nil {
		t.Fatalf("expected invalid rsa key minimum to fail")
	}

	if _, err := store.Set(FIPSModeInput{Enabled: false}); err != nil {
		t.Fatalf("disable mode failed: %v", err)
	}
	ignored := store.Validate(FIPSValidationInput{
		SignatureAlgorithm: "md5-rsa",
		HashAlgorithm:      "md5",
		RSAKeyBits:         512,
		TLSVersion:         "ssl3",
	})
	if !ignored.Allowed || ignored.ModeActive {
		t.Fatalf("expected validation bypass when mode disabled: %+v", ignored)
	}
}
