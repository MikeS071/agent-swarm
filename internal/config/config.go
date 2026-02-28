package config

// Config contains project runtime settings used by the dispatcher.
type Config struct {
	Project ProjectConfig
}

type ProjectConfig struct {
	MaxAgents int
	MinRAMMB  int
}
