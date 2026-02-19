package control

import "testing"

func TestCanonicalizeDocumentDeterministicConfig(t *testing.T) {
	first, err := CanonicalizeDocument(CanonicalizationInput{
		Kind:    "config",
		Content: []byte(`{"resources":[{"id":"a","type":"command","host":"local","command":"echo hi"}],"vars":{"z":"2","a":"1"}}`),
	})
	if err != nil {
		t.Fatalf("canonicalize first config failed: %v", err)
	}
	second, err := CanonicalizeDocument(CanonicalizationInput{
		Kind:    "config",
		Content: []byte(`{"vars":{"a":"1","z":"2"},"resources":[{"command":"echo hi","host":"local","type":"command","id":"a"}]}`),
	})
	if err != nil {
		t.Fatalf("canonicalize second config failed: %v", err)
	}
	if first.CanonicalSHA != second.CanonicalSHA || first.Canonical != second.Canonical {
		t.Fatalf("expected canonicalized config outputs to match: first=%+v second=%+v", first, second)
	}
}

func TestCanonicalizeDocumentPlan(t *testing.T) {
	item, err := CanonicalizeDocument(CanonicalizationInput{
		Kind:    "plan",
		Content: []byte(`{"steps":[{"action":"apply","resource":{"id":"x","type":"command","host":"local","command":"echo hi"}}]}`),
	})
	if err != nil {
		t.Fatalf("canonicalize plan failed: %v", err)
	}
	if item.CanonicalSHA == "" || item.Bytes == 0 {
		t.Fatalf("unexpected canonicalization result: %+v", item)
	}
}
