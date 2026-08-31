package snapshot

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"
)

type TemporarySessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]context.CancelFunc
}

func NewTemporarySessionRegistry() *TemporarySessionRegistry {
	return &TemporarySessionRegistry{sessions: map[string]context.CancelFunc{}}
}
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("snapshot-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}
func (r *TemporarySessionRegistry) Open(parent context.Context) (string, context.Context, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	r.mu.Lock()
	defer r.mu.Unlock()
	id := newID()
	for r.sessions[id] != nil {
		id = newID()
	}
	r.sessions[id] = cancel
	return id, ctx, nil
}
func (r *TemporarySessionRegistry) Close(id string) error {
	r.mu.Lock()
	cancel, ok := r.sessions[id]
	if ok {
		delete(r.sessions, id)
	}
	r.mu.Unlock()
	if !ok {
		return errors.New("snapshot session not found")
	}
	cancel()
	return nil
}
func (r *TemporarySessionRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sessions)
}

// TemporaryTransport is the small existing transport contract needed by a
// snapshot session. Keeping it here avoids coupling the workflow to the engine.
type TemporaryTransport interface {
	Start(context.Context) error
	Messages() <-chan []byte
	Errors() <-chan error
	SendRaw(context.Context, []byte) error
	Close(context.Context) error
}
type TransportFactory func(context.Context, RoomSnapshotRequest) (TemporaryTransport, error)

type CoordinatedSessionFactory struct {
	Registry         *TemporarySessionRegistry
	New              func(context.Context, RoomSnapshotRequest, SnapshotSink) (Session, error) // legacy injection point
	NewTransport     TransportFactory
	OnTransportError func(string, error)
	OnClosed         func(string, int, string)
}

func (f *CoordinatedSessionFactory) BindCoordinator(c interface {
	OnTransportError(string, error) bool
	OnClosed(string, int, string) bool
}) {
	f.OnTransportError = func(id string, e error) { c.OnTransportError(id, e) }
	f.OnClosed = func(id string, code int, reason string) { c.OnClosed(id, code, reason) }
}
func (f *CoordinatedSessionFactory) Create(req RoomSnapshotRequest, sink SnapshotSink) (Session, error) {
	if f == nil || f.Registry == nil {
		return nil, errors.New("temporary session factory is not configured")
	}
	id, ctx, err := f.Registry.Open(context.Background())
	if err != nil {
		return nil, err
	}
	var s Session
	if f.NewTransport != nil {
		t, e := f.NewTransport(ctx, req)
		if e == nil {
			s = &transportSession{id: id, ctx: ctx, transport: t, sink: sink, onError: f.OnTransportError, onClosed: f.OnClosed, join: req.SourceChannel, done: make(chan struct{})}
			err = e
		} else {
			err = e
		}
	} else if f.New != nil {
		s, err = f.New(ctx, req, sink)
	} else {
		err = errors.New("temporary session constructor is not configured")
	}
	if err != nil || s == nil {
		_ = f.Registry.Close(id)
		if err == nil {
			err = errors.New("temporary session factory returned nil session")
		}
		return nil, err
	}
	return &coordinatedSession{Session: s, registry: f.Registry, registryID: id}, nil
}

type coordinatedSession struct {
	Session
	registry   *TemporarySessionRegistry
	registryID string
	once       sync.Once
	closeErr   error
}

func (s *coordinatedSession) Close() error {
	s.once.Do(func() {
		s.closeErr = s.Session.Close()
		if err := s.registry.Close(s.registryID); s.closeErr == nil && err != nil {
			s.closeErr = err
		}
	})
	return s.closeErr
}

type transportSession struct {
	id        string
	ctx       context.Context
	transport TemporaryTransport
	sink      SnapshotSink
	onError   func(string, error)
	onClosed  func(string, int, string)
	join      string
	errorOnce sync.Once
	closeOnce sync.Once
	done      chan struct{}
}

func (s *transportSession) ID() string { return s.id }
func (s *transportSession) Start() error {
	if err := s.transport.Start(s.ctx); err != nil {
		return err
	}
	if s.join != "" {
		join := fmt.Sprintf(`{"cmd":"join","channel":%q}`, s.join)
		if err := s.transport.SendRaw(s.ctx, []byte(join)); err != nil {
			return err
		}
	}
	go s.loop()
	return nil
}
func (s *transportSession) loop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-s.transport.Messages():
			if msg != nil && s.sink != nil {
				s.sink(string(msg))
			}
		case err := <-s.transport.Errors():
			if err != nil {
				s.errorOnce.Do(func() {
					if s.onError != nil {
						s.onError(s.id, err)
					}
				})
				return
			}
		}
	}
}
func (s *transportSession) Flush() error { return nil }
func (s *transportSession) SendRaw(v string) error {
	ctx, c := context.WithTimeout(s.ctx, 5*time.Second)
	defer c()
	return s.transport.SendRaw(ctx, []byte(v))
}
func (s *transportSession) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.done)
		ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		err = s.transport.Close(ctx)
		if s.onClosed != nil {
			s.onClosed(s.id, 1000, "closed")
		}
	})
	return err
}
