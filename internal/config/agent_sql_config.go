package config

type AgentSqlConfig struct {
	Enabled        bool `toml:"enabled"`
	MaxSQLChars    int  `toml:"maxSqlChars"`
	MaxRows        int  `toml:"maxRows"`
	MaxColumns     int  `toml:"maxColumns"`
	MaxCellChars   int  `toml:"maxCellChars"`
	MaxResultChars int  `toml:"maxResultChars"`
	TimeoutMillis  int  `toml:"timeoutMillis"`
}
