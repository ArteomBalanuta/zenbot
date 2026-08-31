package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"zenbot/internal/command"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/factory"
	"zenbot/internal/model"
	"zenbot/internal/repository/h2"
	"zenbot/internal/transport"
)

type lifecycleEngine struct{ e *core.EngineImpl }

func (x lifecycleEngine) Start(ctx context.Context) error { return x.e.StartContext(ctx) }
func (x lifecycleEngine) Stop(ctx context.Context) error  { return x.e.StopContext(ctx) }
func (x lifecycleEngine) Healthy() bool                   { return x.e.Healthy() }

func main() {
	flag.Parse()
	log.SetFlags(0)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	c := config.SetupConfig()
	db, err := h2.Open(ctx, h2.Config{DatabaseStem: c.DbPath, H2Jar: os.Getenv("H2_JAR"), Java: os.Getenv("JAVA"), Port: 5435})
	if err != nil {
		log.Fatal("Can't connect to db: ", err)
	}
	defer func() {
		if db.DB != nil {
			_ = db.DB.Close()
		}
		if db.Server != nil {
			stop, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			_ = db.Server.Stop(stop)
		}
	}()

	transportErrors := make(chan error, 16)
	e, err := factory.NewEngineWithOptions(model.MASTER, c, db, factory.EngineOptions{Transport: transport.Config{URL: c.WebsocketUrl}, LifecycleErrors: transportErrors})
	if err != nil {
		log.Fatal(err)
	}
	manager := core.NewReplicaManager(c.Channel)
	rf := factory.ReplicaFactory{Config: c, Repository: db, Options: factory.EngineOptions{Transport: transport.Config{URL: c.WebsocketUrl}, LifecycleErrors: transportErrors}}
	e.SetReplicaController(core.NewManagedReplicaController(manager, func(ctx context.Context, channel string) (core.ManagedEngine, error) {
		return rf.NewReplica(ctx, channel)
	}))
	if err := command.RegisterUserUtilities(e); err != nil {
		log.Fatal("Can't register Saturn utility commands: ", err)
	}

	lifecycle := core.NewLifecycle(func() core.LifecycleEngine { return lifecycleEngine{e} }, core.RetryPolicy{MaxRetries: 3, StopTimeout: 10 * time.Second})
	go func() {
		for err := range lifecycle.Errors() {
			log.Printf("lifecycle: %v", err)
		}
	}()
	go func() {
		for err := range transportErrors {
			log.Printf("transport: %v", err)
		}
	}()
	if err := lifecycle.Start(ctx); err != nil {
		log.Fatal(err)
	}
	<-ctx.Done()
	stopCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	if err := lifecycle.Stop(stopCtx); err != nil {
		log.Printf("host stop: %v", err)
	}
	if err := manager.StopAll(stopCtx); err != nil {
		log.Printf("replica stop: %v", err)
	}
}
