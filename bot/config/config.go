package config

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"log"
	"os"
)

type Config struct {
	WebsocketUrl                      string   `toml:"url"`
	CmdPrefix                         string   `toml:"cmdPrefix"`
	Name                              string   `toml:"name"`
	Password                          string   `toml:"password"`
	Channel                           string   `toml:"channel"`
	AdminTrips                        []string `toml:"adminTrips"`
	AutoReconnect                     bool     `toml:"autoReconnect"`
	ConnectionHeartbitIntervalMinutes int      `toml:"healthCheckInterval"`
	AutorunCommands                   []string `toml:"autorunCommands"`
}

func SetupConfig() *Config {
	var config Config

	_, err := toml.DecodeFile("config.toml", &config)
	if err != nil {
		log.Println("Error reading config: ", err)
		os.Exit(1)
	}

	fmt.Println("initialized Config - websocket URL: ", config.WebsocketUrl)
	return &config
}
