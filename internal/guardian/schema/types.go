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
	Version           int                `json:"version"`
	Mode              Mode               `json:"mode"`
	Settings          Settings           `json:"settings"`
	EnforcementPoints []string           `json:"enforcement_points"`
	Contexts          map[string]Context `json:"contexts"`
	Rules             []Rule             `json:"rules"`
	Overrides         Overrides          `json:"overrides"`
	Events            Events             `json:"events"`
}

type Settings struct {
	FailClosed       bool `json:"fail_closed"`
	CacheTTLSeconds  int  `json:"cache_ttl_seconds"`
	MaxEvidenceBytes int  `json:"max_evidence_bytes"`
}

type Context struct {
	Severity string `json:"severity"`
}

type Rule struct {
	ID                string      `json:"id"`
	Enabled           bool        `json:"enabled"`
	Description       string      `json:"description"`
	Severity          string      `json:"severity"`
	EnforcementPoints []string    `json:"enforcement_points"`
	Target            Target      `json:"target"`
	Check             Check       `json:"check"`
	PassWhen          PassWhen    `json:"pass_when"`
	FailReason        string      `json:"fail_reason"`
	Evidence          EvidenceRef `json:"evidence"`
}

type Target struct {
	Kind   string   `json:"kind"`
	Paths  []string `json:"paths"`
	Match  string   `json:"match"`
	Source string   `json:"source"`
	Fields []string `json:"fields"`
}

type Check struct {
	Type   string         `json:"type"`
	Params map[string]any `json:"params"`
}

type PassWhen struct {
	Op         string      `json:"op"`
	Conditions []Condition `json:"conditions"`
}

type Condition struct {
	Metric string   `json:"metric"`
	Equals any      `json:"equals,omitempty"`
	GTE    *float64 `json:"gte,omitempty"`
	LTE    *float64 `json:"lte,omitempty"`
}

type EvidenceRef struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type Overrides struct {
	Enabled          bool   `json:"enabled"`
	RequireReason    bool   `json:"require_reason"`
	RequireExpiry    bool   `json:"require_expiry"`
	MaxDurationHours int    `json:"max_duration_hours"`
	Store            string `json:"store"`
}

type Events struct {
	File    string   `json:"file"`
	Include []string `json:"include"`
}
