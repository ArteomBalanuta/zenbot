// Package h2 provides the real H2 PostgreSQL-wire boundary used by Zenbot.
package h2

import (
	"bufio"
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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type h2OpenTestHooks struct {
	// onStarted observes the exact successfully started owned server.
	onStarted func(*processServer)
	// h2Version replaces only Open's H2 identity reader in same-package tests.
	h2Version func(context.Context, *sql.DB) (string, error)
}

// Config describes the local H2 PostgreSQL-wire server and database.
type Config struct {
	BaseDir, DatabaseStem, Host string
	Port                        int
	// AutoPort asks an owned processServer to let H2 select an ephemeral PG port.
	// It is intended for isolated test fixtures. Existing Port == 0 behavior is unchanged.
	AutoPort       bool
	H2Jar, Java    string
	StartupTimeout time.Duration

	// testHooks is nil in production and cannot be set outside package h2.
	testHooks *h2OpenTestHooks
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

const currentSchemaVersion = 4

type processServer struct {
	cfg      Config
	cmd      *exec.Cmd
	addr     string
	mu       sync.Mutex
	waitDone chan struct{}
	waitErr  error
	stopOnce sync.Once
	stopErr  error
}

func (s *processServer) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

var h2PGStartup = regexp.MustCompile(`^PG server running at pg://([^:\s]+):(\d+) \(only local connections\)$`)

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
	if s.cfg.AutoPort {
		return s.startAutoPort(ctx)
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
	s.watch()
	deadline := time.Now().Add(s.cfg.StartupTimeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", s.addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = s.stopWithTimeout()
	return fmt.Errorf("H2 PostgreSQL server did not become ready at %s", s.addr)
}

func (s *processServer) startAutoPort(ctx context.Context) error {
	if ip := net.ParseIP(s.cfg.Host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("H2 auto-port host must be a loopback IP, got %q", s.cfg.Host)
	}
	cmd := exec.CommandContext(ctx, s.cfg.Java, "-cp", s.cfg.H2Jar, "org.h2.tools.Server", "-pg", "-pgPort", "0", "-ifNotExists", "-baseDir", s.cfg.BaseDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("H2 auto-port startup output: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("H2 auto-port startup error output: %w", err)
	}
	s.cmd = cmd
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start H2 auto-port PostgreSQL server: %w", err)
	}
	s.watch()
	endpoint := make(chan string, 1)
	var found sync.Once
	readOutput := func(r interface{ Read([]byte) (int, error) }) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			match := h2PGStartup.FindStringSubmatch(strings.TrimSpace(scanner.Text()))
			if len(match) != 3 {
				continue
			}
			port, err := strconv.Atoi(match[2])
			if err != nil || port <= 0 || port > 65535 {
				continue
			}
			// H2 may advertise a non-loopback host selected from a virtual adapter
			// even when it is constrained to local connections. The configured host
			// has already been validated as loopback and is the endpoint Zenbot owns.
			found.Do(func() { endpoint <- net.JoinHostPort(s.cfg.Host, strconv.Itoa(port)) })
		}
	}
	go readOutput(stdout)
	go readOutput(stderr)
	timeout := time.NewTimer(s.cfg.StartupTimeout)
	defer timeout.Stop()
	select {
	case addr := <-endpoint:
		s.mu.Lock()
		s.addr = addr
		s.mu.Unlock()
	case <-s.waitDone:
		return fmt.Errorf("H2 auto-port endpoint discovery: process exited: %w", s.waitErr)
	case <-ctx.Done():
		_ = s.stopWithTimeout()
		return fmt.Errorf("H2 auto-port endpoint discovery: %w", ctx.Err())
	case <-timeout.C:
		_ = s.stopWithTimeout()
		return errors.New("H2 auto-port endpoint discovery timed out")
	}
	deadline := time.Now().Add(s.cfg.StartupTimeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", s.Addr(), 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-s.waitDone:
			return fmt.Errorf("H2 auto-port readiness: process exited: %w", s.waitErr)
		default:
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = s.stopWithTimeout()
	return fmt.Errorf("H2 auto-port server did not become ready at %s", s.Addr())
}

func (s *processServer) watch() {
	s.waitDone = make(chan struct{})
	go func() {
		err := s.cmd.Wait()
		s.mu.Lock()
		s.waitErr = err
		s.mu.Unlock()
		close(s.waitDone)
	}()
}

func (s *processServer) Stop(ctx context.Context) error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	select {
	case <-s.waitDone:
		return nil
	default:
	}
	s.stopOnce.Do(func() {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
		}
		s.stopErr = s.stopOwned(ctx)
	})
	return s.stopErr
}

func (s *processServer) stopWithTimeout() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.stopOwned(ctx)
}

func (s *processServer) stopOwned(ctx context.Context) error {
	if s.cmd == nil || s.cmd.Process == nil || s.waitDone == nil {
		return nil
	}
	select {
	case <-s.waitDone:
		return nil
	default:
	}
	signaled := s.cmd.Process.Signal(os.Interrupt) == nil
	select {
	case <-s.waitDone:
		if signaled {
			return nil
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.waitErr
	case <-ctx.Done():
		if err := s.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill H2 PostgreSQL server: %w", err)
		}
		<-s.waitDone
		// Windows does not deliver os.Interrupt to the Java child reliably.
		// A completed forced shutdown of this owned process is still successful.
		return nil
	}
}

func readH2Version(ctx context.Context, db *sql.DB) (string, error) {
	var version string
	err := db.QueryRowContext(ctx, "SELECT H2VERSION()").Scan(&version)
	return version, err
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
	absoluteBaseDir, err := filepath.Abs(c.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve H2 base directory: %w", err)
	}
	c.BaseDir = absoluteBaseDir
	c.DatabaseStem = filepath.Base(c.DatabaseStem)
	s := &processServer{cfg: c}
	if err := s.Start(ctx); err != nil {
		return nil, err
	}
	if c.testHooks != nil && c.testHooks.onStarted != nil {
		c.testHooks.onStarted(s)
	}
	dsn := fmt.Sprintf("postgres://%s@%s/%s?sslmode=disable", "sa", s.Addr(), c.DatabaseStem)
	pgxConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		_ = s.Stop(context.Background())
		return nil, err
	}
	// Saturn initializes its embedded H2 files through DriverManager without a
	// username. Preserve that credentialless ownership model over the PG wire.
	pgxConfig.User = ""
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
	readVersion := readH2Version
	if c.testHooks != nil && c.testHooks.h2Version != nil {
		readVersion = c.testHooks.h2Version
	}
	version, err := readVersion(ctx, db)
	if err != nil {
		db.Close()
		_ = s.Stop(context.Background())
		return nil, fmt.Errorf("H2 identity check failed: %w", err)
	}
	if version == "" {
		db.Close()
		_ = s.Stop(context.Background())
		return nil, errors.New("H2 identity check returned empty version")
	}
	if err = bootstrap(ctx, db); err != nil {
		db.Close()
		_ = s.Stop(context.Background())
		return nil, err
	}
	legacyPath := filepath.Join(c.BaseDir, c.DatabaseStem) + ".db"
	if _, err = migrateSQLiteIfNeeded(ctx, db, legacyPath); err != nil {
		db.Close()
		_ = s.Stop(context.Background())
		return nil, fmt.Errorf("SQLite to H2 migration failed: %w", err)
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
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("H2 schema version bootstrap: %w", err)
	}
	if err = ensureMessageVisibility(ctx, tx); err != nil {
		return err
	}
	for _, stmt := range strings.Split(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("H2 schema bootstrap: %w", err)
		}
	}
	if err = ensureMessageVisibility(ctx, tx); err != nil {
		return err
	}
	if err = upgradeTripRoleConstraint(ctx, tx); err != nil {
		return err
	}
	// Existing Go and Saturn mail rows use JSON string contents. New Go writes
	// explicitly opt into PLAIN; the default keeps legacy writers identifiable.
	if _, err = tx.ExecContext(ctx, `ALTER TABLE mail ADD COLUMN IF NOT EXISTS text_encoding VARCHAR NOT NULL DEFAULT 'JSON_STRING'`); err != nil {
		return fmt.Errorf("H2 mail text encoding upgrade: %w", err)
	}
	var version int
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version); err != nil {
		return err
	}
	if version < currentSchemaVersion {
		if _, err = tx.ExecContext(ctx, "INSERT INTO schema_version(version) VALUES($1)", currentSchemaVersion); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type schemaQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type schemaExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type schemaAccess interface {
	schemaQueryer
	schemaExecutor
}

func tableExists(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, table string) (bool, error) {
	var count int
	err := queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE LOWER(table_name)=LOWER($1)`, table).Scan(&count)
	return count > 0, err
}

func ensureMessageVisibility(ctx context.Context, database interface {
	schemaExecutor
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) error {
	exists, err := tableExists(ctx, database, "messages")
	if err != nil || !exists {
		return err
	}
	for _, statement := range []string{
		`ALTER TABLE messages ADD COLUMN IF NOT EXISTS visibility VARCHAR DEFAULT 'PUBLIC'`,
		`UPDATE messages SET visibility='PUBLIC' WHERE visibility IS NULL`,
		`ALTER TABLE messages ALTER COLUMN visibility SET DEFAULT 'PUBLIC'`,
	} {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("H2 message visibility upgrade: %w", err)
		}
	}
	return nil
}

func tripCheckConstraints(ctx context.Context, queryer schemaQueryer) ([]string, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT constraint_name FROM information_schema.table_constraints WHERE LOWER(table_name)='trips' AND constraint_type='CHECK'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var constraints []string
	for rows.Next() {
		var constraint string
		if err := rows.Scan(&constraint); err != nil {
			return nil, err
		}
		constraints = append(constraints, constraint)
	}
	return constraints, rows.Err()
}

func upgradeTripRoleConstraint(ctx context.Context, database interface {
	schemaAccess
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) error {
	exists, err := tableExists(ctx, database, "trips")
	if err != nil || !exists {
		return err
	}
	constraints, err := tripCheckConstraints(ctx, database)
	if err != nil {
		return fmt.Errorf("H2 trip-role constraint inspection: %w", err)
	}
	for _, constraint := range constraints {
		if _, err := database.ExecContext(ctx, "ALTER TABLE trips DROP CONSTRAINT "+quoteIdentifier(constraint)); err != nil {
			return fmt.Errorf("H2 trip-role constraint drop: %w", err)
		}
	}
	if _, err := database.ExecContext(ctx, `ALTER TABLE trips ADD CONSTRAINT trips_type_check CHECK (type IN ('ADMIN','MODERATOR','TRUSTED','USER','REGULAR','PEST'))`); err != nil {
		return fmt.Errorf("H2 trip-role constraint upgrade: %w", err)
	}
	return nil
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

// Pinned H2 2.3.232 schema. The runtime server remains the executing database.
//
//go:embed schema-h2.sql
var schemaSQL string
