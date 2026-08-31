// Package h2 provides the real H2 PostgreSQL-wire boundary used by Zenbot.
package h2

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config describes the externally managed H2 PostgreSQL server.
type Config struct {
	BaseDir, DatabaseStem, Host string
	Port                        int
	H2Jar, Java                 string
	StartupTimeout              time.Duration
}
type Server interface {
	Start(context.Context) error
	Stop(context.Context) error
	Addr() string
}
type Database struct {
	DB     *sql.DB
	Server Server
}

type processServer struct {
	cfg  Config
	cmd  *exec.Cmd
	addr string
}

func (s *processServer) Addr() string { return s.addr }
func (s *processServer) Start(ctx context.Context) error {
	if s.cfg.Java == "" {
		s.cfg.Java = "java"
	}
	if s.cfg.Host == "" {
		s.cfg.Host = "127.0.0.1"
	}
	if s.cfg.Port == 0 {
		s.cfg.Port = 5435
	}
	if s.cfg.StartupTimeout == 0 {
		s.cfg.StartupTimeout = 10 * time.Second
	}
	if _, err := exec.LookPath(s.cfg.Java); err != nil {
		return fmt.Errorf("h2 prerequisite: Java runtime %q unavailable: %w", s.cfg.Java, err)
	}
	if s.cfg.H2Jar == "" {
		return errors.New("h2 prerequisite: H2Jar is required (pinned H2 2.3.232)")
	}
	if _, err := os.Stat(s.cfg.H2Jar); err != nil {
		return fmt.Errorf("h2 prerequisite: H2 jar unavailable: %w", err)
	}
	if s.cfg.BaseDir == "" {
		s.cfg.BaseDir = "."
	}
	if err := os.MkdirAll(s.cfg.BaseDir, 0755); err != nil {
		return err
	}
	s.addr = net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	if c, err := net.DialTimeout("tcp", s.addr, 200*time.Millisecond); err == nil {
		c.Close()
		return nil
	}
	s.cmd = exec.CommandContext(ctx, s.cfg.Java, "-cp", s.cfg.H2Jar, "org.h2.tools.Server", "-pg", "-pgPort", strconv.Itoa(s.cfg.Port), "-ifNotExists", "-baseDir", s.cfg.BaseDir)
	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("start H2 PostgreSQL server: %w", err)
	}
	deadline := time.Now().Add(s.cfg.StartupTimeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", s.addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = s.cmd.Process.Kill()
	return fmt.Errorf("H2 PostgreSQL server did not become ready at %s", s.addr)
}
func (s *processServer) Stop(ctx context.Context) error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	_ = s.cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = s.cmd.Process.Kill()
		return ctx.Err()
	}
}

// Open starts/uses the configured H2 PG server, proves its identity, and bootstraps schema.
func Open(ctx context.Context, c Config) (*Database, error) {
	if c.DatabaseStem == "" {
		return nil, errors.New("H2 database stem is required")
	}
	c.DatabaseStem = strings.TrimSuffix(strings.TrimSuffix(c.DatabaseStem, ".mv.db"), ".db")
	if c.BaseDir == "" {
		c.BaseDir = filepath.Dir(c.DatabaseStem)
		if c.BaseDir == "." {
			c.BaseDir = "."
		}
	}
	c.DatabaseStem = filepath.Base(c.DatabaseStem)
	s := &processServer{cfg: c}
	if err := s.Start(ctx); err != nil {
		return nil, err
	}
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := c.Port
	if port == 0 {
		port = 5435
	}
	dsn := fmt.Sprintf("postgres://%s@%s:%d/%s?sslmode=disable", "sa", host, port, c.DatabaseStem)
	pgxConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		_ = s.Stop(context.Background())
		return nil, err
	}
	// H2's PostgreSQL compatibility server rejects PostgreSQL startup
	// runtime parameters; the wire protocol itself remains usable.
	pgxConfig.RuntimeParams = map[string]string{}
	db := sql.OpenDB(stdlib.GetConnector(*pgxConfig))
	if db == nil {
		_ = s.Stop(context.Background())
		return nil, errors.New("open H2 PostgreSQL database: nil database")
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		_ = s.Stop(context.Background())
		return nil, fmt.Errorf("H2 PostgreSQL connection unavailable: %w", err)
	}
	var version string
	if err = db.QueryRowContext(ctx, "SELECT H2VERSION()").Scan(&version); err != nil {
		db.Close()
		_ = s.Stop(context.Background())
		return nil, fmt.Errorf("H2 identity check failed: %w", err)
	}
	if version == "" {
		return nil, errors.New("H2 identity check returned empty version")
	}
	if err = bootstrap(ctx, db); err != nil {
		db.Close()
		_ = s.Stop(context.Background())
		return nil, err
	}
	return &Database{DB: db, Server: s}, nil
}
func bootstrap(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, stmt := range strings.Split(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("H2 schema bootstrap: %w", err)
		}
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS user_presence_log (
		id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
		trip VARCHAR, name VARCHAR, hash VARCHAR, event_type VARCHAR,
		created_on BIGINT NOT NULL, channel VARCHAR
	)`); err != nil {
		return fmt.Errorf("H2 presence schema bootstrap: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("H2 schema version bootstrap: %w", err)
	}
	var version int
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version); err != nil {
		return err
	}
	if version == 0 {
		if _, err = tx.ExecContext(ctx, "INSERT INTO schema_version(version) VALUES(1)"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Pinned H2 2.3.232 schema. The runtime server remains the executing database.
//
//go:embed schema-h2.sql
var schemaSQL string
