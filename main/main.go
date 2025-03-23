package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
)

// wss://hack.chat/chat-ws

func main() {
	flag.Parse()
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	e := NewEngine()
	go e.Start()

	select {
	case <-interrupt:
		log.Println("Interrupt received, shutting down...")
		e.Stop()
	}

	e.hcConnection.wg.Wait()
}
