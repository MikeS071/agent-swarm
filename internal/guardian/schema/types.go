package schema

type Mode string

const (
	ModeAdvisory Mode = "advisory"
	ModeEnforce  Mode = "enforce"
)

type Result string

const (
	ResultAllow Result = "ALLOW"
	ResultWarn  Result = "WARN"
	ResultBlock Result = "BLOCK"
)

type FlowPolicy struct {
	Version           int                `yaml:"version"`
	Mode              Mode               `yaml:"mode"`
	Settings          Settings           `yaml:"settings"`
	EnforcementPoints []string           `yaml:"enforcement_points"`
	Contexts          map[string]Context `yaml:"contexts"`
	Rules             []Rule             `yaml:"rules"`
	Overrides         Overrides          `yaml:"overrides"`
	Events            Events             `yaml:"events"`
}

type Settings struct {
	FailClosed       bool `yaml:"fail_closed"`
	CacheTTLSeconds  int  `yaml:"cache_ttl_seconds"`
	MaxEvidenceBytes int  `yaml:"max_evidence_bytes"`
}

type Context struct {
	Severity string `yaml:"severity"`
}

type Rule struct {
	ID                string      `yaml:"id"`
	Enabled           bool        `yaml:"enabled"`
	Description       string      `yaml:"description"`
	Severity          string      `yaml:"severity"`
	EnforcementPoints []string    `yaml:"enforcement_points"`
	Target            Target      `yaml:"target"`
	Check             Check       `yaml:"check"`
	PassWhen          PassWhen    `yaml:"pass_when"`
	FailReason        string      `yaml:"fail_reason"`
	Evidence          EvidenceRef `yaml:"evidence"`
}

type Target struct {
	Kind   string   `yaml:"kind"`
	Paths  []string `yaml:"paths"`
	Match  string   `yaml:"match"`
	Source string   `yaml:"source"`
	Fields []string `yaml:"fields"`
}

type Check struct {
	Type   string                 `yaml:"type"`
	Params map[string]interface{} `yaml:"params"`
}

type PassWhen struct {
	Op         string      `yaml:"op"`
	Conditions []Condition `yaml:"conditions"`
}

type Condition struct {
	Metric string      `yaml:"metric"`
	Equals interface{} `yaml:"equals,omitempty"`
	GTE    *float64    `yaml:"gte,omitempty"`
	LTE    *float64    `yaml:"lte,omitempty"`
}

type EvidenceRef struct {
	Kind string `yaml:"kind"`
	Path string `yaml:"path"`
}

type Overrides struct {
	Enabled          bool   `yaml:"enabled"`
	RequireReason    bool   `yaml:"require_reason"`
	RequireExpiry    bool   `yaml:"require_expiry"`
	MaxDurationHours int    `yaml:"max_duration_hours"`
	Store            string `yaml:"store"`
}

type Events struct {
	File    string   `yaml:"file"`
	Include []string `yaml:"include"`
}
