package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"zenbot/bot/config"
	"zenbot/bot/core"
	"zenbot/bot/model"
	"zenbot/bot/repository"
)

// wss://hack.chat/chat-ws

func main() {
	flag.Parse()
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	c := config.SetupConfig()
	db, err := repository.NewRepository(c.DbPath)
	if err != nil {
		log.Fatal("Can't connect to db: ", c.DbPath)
	}

	e := core.NewEngine(model.MASTER, c, db)
	go e.Start()

	select {
	case <-interrupt:
		log.Println("Interrupt received, shutting down...")
		e.Stop()
	}

	e.HcConnection.Wg.Wait()
}
