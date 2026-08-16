package unit

import (
	"encoding/json"
	"testing"

	"example.com/embedded-loop-channel/api/convert"
	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/fake"
	"example.com/embedded-loop-channel/plugin/adb"
)

// TestCapabilitySelfDescribing verifies a capability carries a description and
// a real JSON Schema, so it is self-describing to LLM tool selection.
func TestCapabilitySelfDescribing(t *testing.T) {
	p := adb.New(fake.NewFarm())
	var flash *domain.Capability
	for i := range p.Capabilities(nil) {
		if p.Capabilities(nil)[i].Name == domain.CapabilityFlash {
			flash = &p.Capabilities(nil)[i]
		}
	}
	if flash == nil {
		t.Fatalf("device.flash not found")
	}
	if flash.Description == "" {
		t.Fatalf("flash has no description")
	}

	// input schema must be a real JSON Schema: type=object + required params.
	if flash.InputSchema["type"] != "object" {
		t.Fatalf("input schema type != object: %v", flash.InputSchema["type"])
	}
	req, ok := flash.InputSchema["required"].([]string)
	if !ok || len(req) != 3 {
		t.Fatalf("required = %v (want [partition image version])", flash.InputSchema["required"])
	}
	props, _ := flash.InputSchema["properties"].(map[string]any)
	if _, ok := props["partition"]; !ok {
		t.Fatalf("missing partition property")
	}

	// ToolDefinition is exactly {name, description, input_schema}.
	td := flash.ToolDefinition()
	b, err := json.Marshal(td)
	if err != nil {
		t.Fatalf("marshal tool definition: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["name"] != "device.flash" || m["description"] == "" || m["input_schema"] == nil {
		t.Fatalf("tool definition malformed: %s", b)
	}
}

// TestCapabilityProtoRoundtripPreservesSchema verifies description and schema
// survive the wire contract (proto string) roundtrip.
func TestCapabilityProtoRoundtripPreservesSchema(t *testing.T) {
	c := &domain.Capability{
		Name:        domain.CapabilityFlash,
		Version:     "1.0",
		Description: "刷写固件",
		InputSchema: domain.ObjectSchema(
			[]string{"partition"},
			map[string]domain.JSONSchema{"partition": domain.StringSchema("目标分区")},
		),
		OutputSchema: domain.StringSchema("结果"),
		RiskLevel:    domain.RiskHigh,
	}

	back := convert.CapabilityFromProto(convert.CapabilityToProto(c))

	if back.Description != c.Description {
		t.Fatalf("description lost: %q", back.Description)
	}
	if back.InputSchema == nil || back.InputSchema["type"] != "object" {
		t.Fatalf("input schema lost: %v", back.InputSchema)
	}
	if _, ok := back.InputSchema["properties"]; !ok {
		t.Fatalf("schema properties lost")
	}
}
