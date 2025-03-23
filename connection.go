package main

import (
	"github.com/gorilla/websocket"
	"log"
	"sync"
	"time"
)

type Connection struct {
	// con       *Connection
	url         string
	wsCon       *websocket.Conn
	connectCh   chan error
	wg          sync.WaitGroup
	msgListener *UserMessageListener
	joinedRoom  bool
}

func NewConnection(url string, msgListener *UserMessageListener) *Connection {
	cInstance := &Connection{
		url:         url,
		connectCh:   make(chan error, 1),
		msgListener: msgListener,
		joinedRoom:  false,
	}

	return cInstance
}

func (c *Connection) Connect() {
	defer c.wg.Done()
	wc, _, err := websocket.DefaultDialer.Dial(c.url, nil)
	c.wsCon = wc
	c.connectCh <- err
	if err != nil {
		log.Println("Connection error:", err)
	}
	close(c.connectCh) // Always close the channel after sending the result

	c.wg.Add(1)
	go c.ReadMessages()
	go c.SendPing() // we dont increment c.wg as it is secondary thread!
}

func (c *Connection) IsConnected() bool {
	if err, ok := <-c.connectCh; ok == false || err != nil {
		log.Fatal("Failed to connect:", err)
		return false
	} else {
		log.Println("Connection established successfully!")
		return true
	}
}

func (c *Connection) SendPing() {
	defer c.wg.Done()

	seconds15, _ := time.ParseDuration("15s")
	ticker := time.NewTicker(seconds15)
	defer ticker.Stop()

	select {
	case <-ticker.C:
		err := c.wsCon.WriteMessage(websocket.PingMessage, nil)
		if err != nil {
			log.Println("Error sending ping", err)
			return
		} else {
			log.Println("Sent ping.")
		}
	}
}

func (c *Connection) ReadMessages() {
	defer c.wg.Done()
	for {
		_, message, err := c.wsCon.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Println("Connection closed gracefully")
			} else {
				log.Println("read error:", err)
			}
			break
		}
		c.msgListener.notify(string(message))
	}
}

func (c *Connection) GetConnection() *Connection {
	return c
}

func (c *Connection) Write(payload string) {
	werr := c.wsCon.WriteMessage(websocket.TextMessage, []byte(payload))

	if werr != nil {
		log.Println("write error:", werr)
		return
	}
}

func (c *Connection) Close() error {
	if c.wsCon != nil {
		return c.wsCon.Close()
	}
	return nil
}
