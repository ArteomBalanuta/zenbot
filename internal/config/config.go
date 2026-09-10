package config

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"log"
	"os"
	"strings"
)

type Config struct {
	WebsocketUrl                      string      `toml:"url"`
	WsUrl                             string      `toml:"wsUrl"`
	Nick                              string      `toml:"nick"`
	CmdPrefix                         string      `toml:"cmdPrefix"`
	Name                              string      `toml:"name"`
	Password                          string      `toml:"password"`
	BotTrip                           string      `toml:"trip"`
	Channel                           string      `toml:"channel"`
	AdminTrips                        []string    `toml:"adminTrips"`
	UserTrips                         []string    `toml:"userTrips"`
	AutoReconnect                     bool        `toml:"autoReconnect"`
	ConnectionHeartbitIntervalMinutes int         `toml:"healthCheckInterval"`
	AutorunCommands                   []string    `toml:"autorunCommands"`
	DbPath                            string      `toml:"dbPath"`
	Agent                             AgentConfig `toml:"agent"`
}

// UnmarshalTOML accepts both Zenbot arrays and Saturn's comma-separated list
// values without leaking compatibility concerns into the runtime model.
func (c *Config) UnmarshalTOML(value any) error {
	root, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("config root must be a TOML table")
	}
	for _, key := range []string{"adminTrips", "userTrips", "autorunCommands"} {
		if raw, exists := root[key]; exists {
			root[key] = normalizeListValue(raw)
		}
	}
	encoded, err := toml.Marshal(root)
	if err != nil {
		return err
	}
	type plainConfig Config
	var decoded plainConfig
	if _, err := toml.Decode(string(encoded), &decoded); err != nil {
		return err
	}
	*c = Config(decoded)
	return nil
}

func normalizeListValue(value any) any {
	text, ok := value.(string)
	if !ok {
		return value
	}
	items := make([]string, 0)
	for _, item := range strings.Split(text, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

func (c *Config) Normalize() {
	if strings.TrimSpace(c.WebsocketUrl) == "" {
		c.WebsocketUrl = strings.TrimSpace(c.WsUrl)
	}
	if strings.TrimSpace(c.Name) == "" {
		c.Name = strings.TrimSpace(c.Nick)
	}
	if strings.TrimSpace(c.Password) == "" {
		c.Password = strings.TrimSpace(c.BotTrip)
	}
	if strings.TrimSpace(c.Password) == "" {
		c.Password = strings.TrimSpace(os.Getenv("TOKEN"))
	}
}

func SetupConfig() *Config {
	var config Config

	_, err := toml.DecodeFile("config.toml", &config)
	if err != nil {
		log.Println("Error reading config: ", err)
		os.Exit(1)
	}
	config.Normalize()

	fmt.Println("initialized Config - websocket URL: ", config.WebsocketUrl)
	return &config
}
