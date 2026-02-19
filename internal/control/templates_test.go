package control

import "testing"

func TestValidateSurveyAnswers(t *testing.T) {
	schema := map[string]SurveyField{
		"env": {
			Type:     "string",
			Required: true,
			Enum:     []string{"prod", "staging"},
		},
		"retries": {
			Type: "int",
		},
		"force": {
			Type: "bool",
		},
	}

	if err := ValidateSurveyAnswers(schema, map[string]string{"env": "prod", "retries": "2", "force": "true"}); err != nil {
		t.Fatalf("expected valid answers, got error: %v", err)
	}
	if err := ValidateSurveyAnswers(schema, map[string]string{"retries": "2"}); err == nil {
		t.Fatalf("expected missing required field error")
	}
	if err := ValidateSurveyAnswers(schema, map[string]string{"env": "dev"}); err == nil {
		t.Fatalf("expected enum validation error")
	}
	if err := ValidateSurveyAnswers(schema, map[string]string{"env": "prod", "retries": "x"}); err == nil {
		t.Fatalf("expected integer validation error")
	}
	if err := ValidateSurveyAnswers(schema, map[string]string{"env": "prod", "extra": "x"}); err == nil {
		t.Fatalf("expected unknown field validation error")
	}
}

func TestRenderTemplateText_StrictMode(t *testing.T) {
	rendered, missing := RenderTemplateText("env={{ env }} token={{token}}", map[string]string{"env": "prod"}, true)
	if rendered != "env=prod token={{token}}" {
		t.Fatalf("unexpected strict render output %q", rendered)
	}
	if len(missing) != 1 || missing[0] != "token" {
		t.Fatalf("expected missing token in strict mode, got %#v", missing)
	}
}

func TestRenderTemplateText_NonStrictMode(t *testing.T) {
	rendered, missing := RenderTemplateText("env={{ env }} token={{token}}", map[string]string{"env": "prod"}, false)
	if rendered != "env=prod token=" {
		t.Fatalf("unexpected non-strict render output %q", rendered)
	}
	if len(missing) != 1 || missing[0] != "token" {
		t.Fatalf("expected missing token in non-strict mode, got %#v", missing)
	}
}
