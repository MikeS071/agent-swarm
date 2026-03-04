package schema

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*FlowPolicy, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read flow policy %s: %w", path, err)
	}
	var p FlowPolicy
	if err := yaml.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parse flow policy %s: %w", path, err)
	}
	if err := Validate(&p); err != nil {
		return nil, err
	}
	return &p, nil
}
