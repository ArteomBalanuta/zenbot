package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"zenbot/internal/command"
	"zenbot/internal/config"
	"zenbot/internal/factory"
	"zenbot/internal/model"
	"zenbot/internal/repository"
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

	e := factory.NewEngine(model.MASTER, c, db)

	e.RegisterCommand(&command.List{})
	e.RegisterCommand(&command.Say{})
	e.RegisterCommand(&command.Afk{})
	e.RegisterCommand(&command.Kick{})

	go e.Start()

	select {
	case <-interrupt:
		log.Println("Interrupt received, shutting down...")
		e.Stop()
	}

	e.WaitConnectionWgDone()
}
