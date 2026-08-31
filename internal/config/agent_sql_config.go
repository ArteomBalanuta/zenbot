package config

type AgentSqlConfig struct {
	Enabled       bool `toml:"enabled"`
	MaxRows       int  `toml:"maxRows"`
	TimeoutMillis int  `toml:"timeoutMillis"`
}
