package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"zenbot/bot/config"
	"zenbot/bot/core"
)

// wss://hack.chat/chat-ws

func main() {
	flag.Parse()
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	c := config.SetupConfig()
	e := core.NewEngine(c)
	go e.Start()

	select {
	case <-interrupt:
		log.Println("Interrupt received, shutting down...")
		e.Stop()
	}

	e.HcConnection.Wg.Wait()
}
