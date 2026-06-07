package llm

import (
	"encoding/json"
	"testing"
)

func TestToOllamaTool_PassesThroughSchema(t *testing.T) {
	in := Tool{
		Name:        "weather__forecast",
		Description: "Get the weather forecast",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
	}
	out, err := ToOllamaTool(in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Type != "function" {
		t.Errorf("Type = %q, want function", out.Type)
	}
	if out.Function.Name != "weather__forecast" {
		t.Errorf("Function.Name = %q", out.Function.Name)
	}
	if out.Function.Description != "Get the weather forecast" {
		t.Errorf("Function.Description = %q", out.Function.Description)
	}
}

func TestToOllamaTools_ConvertsAllAndReportsBadSchemas(t *testing.T) {
	in := []Tool{
		{Name: "a", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "broken", InputSchema: json.RawMessage(`not json`)},
		{Name: "b", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	out, errs := ToOllamaTools(in)
	if len(out) != 2 {
		t.Fatalf("converted len = %d, want 2 (broken should be skipped)", len(out))
	}
	if out[0].Function.Name != "a" || out[1].Function.Name != "b" {
		t.Errorf("names: %q %q", out[0].Function.Name, out[1].Function.Name)
	}
	if len(errs) != 1 {
		t.Fatalf("errs len = %d, want 1", len(errs))
	}
}

func TestToOllamaTool_ReturnsErrorOnBadSchema(t *testing.T) {
	in := Tool{Name: "x", InputSchema: json.RawMessage(`not json`)}
	if _, err := ToOllamaTool(in); err == nil {
		t.Fatal("expected error for invalid schema")
	}
}
