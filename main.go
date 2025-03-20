package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

// wss://hack.chat/chat-ws
var addr = flag.String("addr", "hack.chat", "http service address")

func main() {
	flag.Parse()
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "wss", Host: *addr, Path: "/chat-ws"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)

	if err != nil {
		log.Fatal("dial:", err)
	}

	defer c.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Println("Connection closed gracefully")
				} else {
					log.Println("read error:", err)
				}
				return
			}
			fmt.Println("Message:", string(message))
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	fmt.Println("Connected!")

	for {
		select {
		case <-done:
			log.Println("Connection closed by server.")
			return
		// TODO: should be actually ping! ticker ticks, ping pings ;
		case t := <-ticker.C:
			channel := "programming"
			nick := "goblood"
			id := "42"
			joinPayload := fmt.Sprintf(`{ "cmd": "join", "channel": "%s", "nick": "%s#%s" }`, channel, nick, id)
			err := c.WriteMessage(websocket.TextMessage, []byte(joinPayload))
			if err != nil {
				log.Println("write error:", err)
				return
			}
		case <-interrupt:
			log.Println("Interrupt received, closing connection...")

			// Send close message to server
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Bye!"))
			if err != nil {
				log.Println("Close error:", err)
			}
			return
		}
	}

}
