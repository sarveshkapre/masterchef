package control

import "testing"

func TestMTLSAuthorityPolicyAndHandshake(t *testing.T) {
	store := NewMTLSStore()
	ca, err := store.CreateAuthority(MTLSAuthorityInput{
		Name:     "prod-ca",
		CABundle: "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----",
	})
	if err != nil {
		t.Fatalf("create authority failed: %v", err)
	}

	_, err = store.SetPolicy(MTLSComponentPolicy{
		Component:          "control-plane",
		MinTLSVersion:      "1.3",
		RequireClientCert:  true,
		AllowedAuthorities: []string{ca.ID},
	})
	if err != nil {
		t.Fatalf("set policy failed: %v", err)
	}

	ok := store.CheckHandshake(MTLSHandshakeCheckInput{
		Component:   "control-plane",
		AuthorityID: ca.ID,
		TLSVersion:  "1.3",
		ClientCert:  true,
	})
	if !ok.Allowed {
		t.Fatalf("expected handshake to be allowed: %+v", ok)
	}

	fail := store.CheckHandshake(MTLSHandshakeCheckInput{
		Component:   "control-plane",
		AuthorityID: ca.ID,
		TLSVersion:  "1.2",
		ClientCert:  true,
	})
	if fail.Allowed {
		t.Fatalf("expected tls version failure")
	}
}
