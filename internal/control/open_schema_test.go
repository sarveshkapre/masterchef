package control

import "testing"

func TestOpenSchemaValidateJSONSchema(t *testing.T) {
	store := NewOpenSchemaStore()
	doc, err := store.Upsert(OpenSchemaInput{
		Name:    "cfg-json",
		Format:  "json_schema",
		Enabled: true,
		Content: `{"type":"object","required":["version","inventory"]}`,
	})
	if err != nil {
		t.Fatalf("upsert json schema: %v", err)
	}
	ok := store.Validate(OpenSchemaValidationInput{SchemaID: doc.ID, Document: "version: v0\ninventory: {}\n"})
	if !ok.Valid {
		t.Fatalf("expected schema validation success, got %+v", ok)
	}
	bad := store.Validate(OpenSchemaValidationInput{SchemaID: doc.ID, Document: "version: v0\n"})
	if bad.Valid {
		t.Fatalf("expected schema validation failure")
	}
}

func TestOpenSchemaValidateCUE(t *testing.T) {
	store := NewOpenSchemaStore()
	doc, err := store.Upsert(OpenSchemaInput{
		Name:    "cfg-cue",
		Format:  "cue",
		Enabled: true,
		Content: "version: string\ninventory: _\nresources: [..._]\n",
	})
	if err != nil {
		t.Fatalf("upsert cue schema: %v", err)
	}
	ok := store.Validate(OpenSchemaValidationInput{SchemaID: doc.ID, Document: "version: v0\ninventory: {}\nresources: []\n"})
	if !ok.Valid {
		t.Fatalf("expected cue schema validation success, got %+v", ok)
	}
}
