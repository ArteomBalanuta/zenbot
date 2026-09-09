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
	Channel                           string      `toml:"channel"`
	AdminTrips                        []string    `toml:"adminTrips"`
	UserTrips                         []string    `toml:"userTrips"`
	AutoReconnect                     bool        `toml:"autoReconnect"`
	ConnectionHeartbitIntervalMinutes int         `toml:"healthCheckInterval"`
	AutorunCommands                   []string    `toml:"autorunCommands"`
	DbPath                            string      `toml:"dbPath"`
	Agent                             AgentConfig `toml:"agent"`
}

func (c *Config) Normalize() {
	if strings.TrimSpace(c.WebsocketUrl) == "" {
		c.WebsocketUrl = strings.TrimSpace(c.WsUrl)
	}
	if strings.TrimSpace(c.Name) == "" {
		c.Name = strings.TrimSpace(c.Nick)
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
