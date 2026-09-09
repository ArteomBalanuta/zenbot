package config

import "fmt"

type AgentSqlConfig struct {
	Enabled        bool `toml:"enabled"`
	MaxSQLChars    int  `toml:"maxSqlChars"`
	MaxRows        int  `toml:"maxRows"`
	MaxColumns     int  `toml:"maxColumns"`
	MaxCellChars   int  `toml:"maxCellChars"`
	MaxResultChars int  `toml:"maxResultChars"`
	TimeoutMillis  int  `toml:"timeoutMillis"`
}

func (c AgentSqlConfig) Validate() error {
	for name, value := range map[string]int{
		"dynamicSqlMaxSqlChars":    c.MaxSQLChars,
		"dynamicSqlMaxRows":        c.MaxRows,
		"dynamicSqlMaxColumns":     c.MaxColumns,
		"dynamicSqlMaxCellChars":   c.MaxCellChars,
		"dynamicSqlMaxResultChars": c.MaxResultChars,
		"dynamicSqlTimeoutMillis":  c.TimeoutMillis,
	} {
		if value <= 0 {
			return fmt.Errorf("agent.%s must be positive", name)
		}
		if value > maxConfigLimit {
			return fmt.Errorf("agent.%s exceeds maximum %d", name, maxConfigLimit)
		}
	}
	return nil
}
