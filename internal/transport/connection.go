// Package transport provides a bounded, context-aware WebSocket transport.
package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var ErrClosed = errors.New("transport is closed")

type Dialer interface {
	DialContext(context.Context, string, http.Header) (*websocket.Conn, *http.Response, error)
}
type Config struct {
	URL                                      string
	Dialer                                   Dialer
	PingInterval, WriteTimeout, CloseTimeout time.Duration
}
type Connection struct {
	cfg       Config
	connMu    sync.RWMutex
	conn      *websocket.Conn
	writeMu   sync.Mutex
	startOnce sync.Once
	closeOnce sync.Once
	done      chan struct{}
	messages  chan []byte
	errs      chan error
	connected atomic.Bool
	started   atomic.Bool
}

func NewConnection(cfg Config) *Connection {
	if cfg.Dialer == nil {
		cfg.Dialer = websocket.DefaultDialer
	}
	if cfg.PingInterval <= 0 {
		cfg.PingInterval = 15 * time.Second
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = 5 * time.Second
	}
	if cfg.CloseTimeout <= 0 {
		cfg.CloseTimeout = 5 * time.Second
	}
	return &Connection{cfg: cfg, done: make(chan struct{}), messages: make(chan []byte, 32), errs: make(chan error, 8)}
}
func (c *Connection) Messages() <-chan []byte { return c.messages }
func (c *Connection) Errors() <-chan error    { return c.errs }
func (c *Connection) Connected() bool         { return c.connected.Load() }
func (c *Connection) Start(ctx context.Context) error {
	if c.isClosed() {
		return ErrClosed
	}
	var err error
	c.startOnce.Do(func() { c.started.Store(true); err = c.start(ctx) })
	if !c.started.Load() {
		return ErrClosed
	}
	return err
}
func (c *Connection) isClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *Connection) start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, _, err := c.cfg.Dialer.DialContext(ctx, c.cfg.URL, nil)
	if err != nil {
		c.publish(err)
		return fmt.Errorf("websocket dial: %w", err)
	}
	if c.isClosed() {
		_ = ws.Close()
		return ErrClosed
	}
	c.connMu.Lock()
	c.conn = ws
	c.connMu.Unlock()
	c.connected.Store(true)
	go c.readLoop()
	go c.pingLoop()
	return nil
}
func (c *Connection) publish(err error) {
	if err == nil {
		return
	}
	select {
	case c.errs <- err:
	default:
	}
}
func (c *Connection) readLoop() {
	defer c.connected.Store(false)
	for {
		_, msg, err := c.getConn().ReadMessage()
		if err != nil {
			select {
			case <-c.done:
			default:
				c.publish(err)
			}
			return
		}
		select {
		case c.messages <- append([]byte(nil), msg...):
		case <-c.done:
			return
		}
	}
}
func (c *Connection) pingLoop() {
	ticker := time.NewTicker(c.cfg.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, nil); err != nil {
				c.publish(err)
				return
			}
		case <-c.done:
			return
		}
	}
}
func (c *Connection) getConn() *websocket.Conn {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn
}
func (c *Connection) write(kind int, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.connMu.RLock()
	ws := c.conn
	c.connMu.RUnlock()
	if ws == nil || !c.Connected() {
		return ErrClosed
	}
	_ = ws.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
	return ws.WriteMessage(kind, payload)
}
func (c *Connection) SendText(ctx context.Context, payload string) error {
	return c.SendRaw(ctx, []byte(payload))
}
func (c *Connection) SendRaw(ctx context.Context, payload []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return c.write(websocket.TextMessage, payload)
}
func (c *Connection) Close(ctx context.Context) error {
	var err error
	c.closeOnce.Do(func() {
		close(c.done)
		c.connected.Store(false)
		c.connMu.RLock()
		ws := c.conn
		c.connMu.RUnlock()
		if ws != nil {
			err = ws.Close()
		}
		// Channels remain open: reader and ping goroutines may concurrently publish
		// terminal errors while observing done. Consumers use done/Connected to stop.
	})
	return err
}
