package domain

import "encoding/json"

// JSONSchema is a structured JSON Schema object. It marshals directly to the
// JSON Schema an LLM function-calling tool definition needs, so capabilities
// are self-describing to agents (name + description + input_schema).
type JSONSchema map[string]any

// ObjectSchema builds an object-typed JSON Schema from its properties.
func ObjectSchema(required []string, properties map[string]JSONSchema) JSONSchema {
	props := map[string]any{}
	for k, v := range properties {
		props[k] = v
	}
	s := JSONSchema{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// StringSchema builds a string property schema with a description.
func StringSchema(description string) JSONSchema {
	return JSONSchema{"type": "string", "description": description}
}

// StringEnumSchema builds a string property schema constrained to an enum.
func StringEnumSchema(description string, values ...string) JSONSchema {
	return JSONSchema{"type": "string", "description": description, "enum": values}
}

// JSON serializes the schema to a JSON string (the wire contract keeps
// input_schema/output_schema as JSON text).
func (s JSONSchema) JSON() string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ParseSchema parses a JSON-schema string back into a JSONSchema.
func ParseSchema(s string) JSONSchema {
	if s == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return JSONSchema(m)
}
