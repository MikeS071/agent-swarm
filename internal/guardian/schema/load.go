package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type nodeKind int

const (
	nodeAny nodeKind = iota
	nodeObject
	nodeArray
)

type shapeNode struct {
	kind         nodeKind
	fields       map[string]*shapeNode
	element      *shapeNode
	allowUnknown bool
}

func object(fields map[string]*shapeNode) *shapeNode {
	return &shapeNode{kind: nodeObject, fields: fields}
}

func array(element *shapeNode) *shapeNode {
	return &shapeNode{kind: nodeArray, element: element}
}

func anyNode() *shapeNode {
	return &shapeNode{kind: nodeAny}
}

var policyShape = object(map[string]*shapeNode{
	"version":            anyNode(),
	"mode":               anyNode(),
	"settings":           object(map[string]*shapeNode{"fail_closed": anyNode(), "cache_ttl_seconds": anyNode(), "max_evidence_bytes": anyNode()}),
	"enforcement_points": array(anyNode()),
	"contexts": &shapeNode{
		kind:         nodeObject,
		allowUnknown: true,
		element:      object(map[string]*shapeNode{"severity": anyNode()}),
	},
	"rules": array(object(map[string]*shapeNode{
		"id":                 anyNode(),
		"enabled":            anyNode(),
		"description":        anyNode(),
		"severity":           anyNode(),
		"enforcement_points": array(anyNode()),
		"target": object(map[string]*shapeNode{
			"kind":   anyNode(),
			"paths":  array(anyNode()),
			"match":  anyNode(),
			"source": anyNode(),
			"fields": array(anyNode()),
		}),
		"check": object(map[string]*shapeNode{
			"type": anyNode(),
			"params": {
				kind:         nodeObject,
				allowUnknown: true,
				element:      anyNode(),
			},
		}),
		"pass_when": object(map[string]*shapeNode{
			"op": anyNode(),
			"conditions": array(object(map[string]*shapeNode{
				"metric": anyNode(),
				"equals": anyNode(),
				"gte":    anyNode(),
				"lte":    anyNode(),
			})),
		}),
		"fail_reason": anyNode(),
		"evidence":    object(map[string]*shapeNode{"kind": anyNode(), "path": anyNode()}),
	})),
	"overrides": object(map[string]*shapeNode{
		"enabled":            anyNode(),
		"require_reason":     anyNode(),
		"require_expiry":     anyNode(),
		"max_duration_hours": anyNode(),
		"store":              anyNode(),
	}),
	"events": object(map[string]*shapeNode{
		"file":    anyNode(),
		"include": array(anyNode()),
	}),
})

func Load(path string) (*FlowPolicy, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read flow policy %s: %w", path, err)
	}
	policy, err := Parse(body)
	if err != nil {
		return nil, fmt.Errorf("load flow policy %s: %w", path, err)
	}
	return policy, nil
}

func Parse(body []byte) (*FlowPolicy, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, ValidationErrors{{Path: "policy", Message: "is empty"}}
	}

	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, ValidationErrors{{Path: "policy", Message: fmt.Sprintf("invalid JSON: %v", err)}}
	}

	errs := validateStructure(raw, policyShape, "policy")
	if len(errs) > 0 {
		return nil, errs.sorted()
	}

	var policy FlowPolicy
	if err := json.Unmarshal(body, &policy); err != nil {
		return nil, ValidationErrors{{Path: "policy", Message: fmt.Sprintf("invalid policy value types: %v", err)}}
	}

	if err := Validate(&policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

func validateStructure(value any, shape *shapeNode, path string) ValidationErrors {
	var errs ValidationErrors
	if shape == nil {
		return errs
	}

	switch shape.kind {
	case nodeAny:
		return errs
	case nodeArray:
		items, ok := value.([]any)
		if !ok {
			errs.add(path, "must be an array")
			return errs
		}
		for i, item := range items {
			errs = append(errs, validateStructure(item, shape.element, fmt.Sprintf("%s[%d]", path, i))...)
		}
		return errs
	case nodeObject:
		obj, ok := value.(map[string]any)
		if !ok {
			errs.add(path, "must be an object")
			return errs
		}
		for k, v := range obj {
			childPath := joinPath(path, k)
			child, hasKnown := shape.fields[k]
			switch {
			case hasKnown:
				errs = append(errs, validateStructure(v, child, childPath)...)
			case shape.allowUnknown && shape.element != nil:
				errs = append(errs, validateStructure(v, shape.element, childPath)...)
			default:
				errs.add(childPath, "unknown field")
			}
		}
		return errs
	default:
		return errs
	}
}

func joinPath(path, child string) string {
	if path == "" {
		return child
	}
	if path == "policy" {
		return child
	}
	return path + "." + child
}
