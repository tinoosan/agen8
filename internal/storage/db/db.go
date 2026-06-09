package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/tinoosan/agen8/pkg/fsutil"
)

type Driver string

const (
	DriverSQLite   Driver = "sqlite"
	DriverPostgres Driver = "postgres"
)

type Config struct {
	Driver       Driver
	DataDir      string
	DatabaseURL  string
	Pool         PoolConfig
	Migrate      Migrator
	MigrationKey string
}

type PoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type Migrator func(context.Context, *sql.DB, Driver) error

type Handle struct {
	db      *sql.DB
	driver  Driver
	dialect Dialect
}

func (h *Handle) DB() *sql.DB {
	if h == nil {
		return nil
	}
	return h.db
}

func (h *Handle) Driver() Driver {
	if h == nil {
		return ""
	}
	return h.driver
}

func (h *Handle) Dialect() Dialect {
	if h == nil {
		return nil
	}
	return h.dialect
}

type Dialect interface {
	Placeholder(n int) string
	JSONType() string
	NowExpr() string
}

func Rebind(query string, dialect Dialect) string {
	if dialect == nil {
		return query
	}
	var b strings.Builder
	b.Grow(len(query))
	arg := 1
	for i := 0; i < len(query); i++ {
		if query[i] != '?' {
			b.WriteByte(query[i])
			continue
		}
		b.WriteString(dialect.Placeholder(arg))
		arg++
	}
	return b.String()
}

type sqliteDialect struct{}

func (sqliteDialect) Placeholder(int) string { return "?" }
func (sqliteDialect) JSONType() string       { return "TEXT" }
func (sqliteDialect) NowExpr() string        { return "CURRENT_TIMESTAMP" }

type postgresDialect struct{}

func (postgresDialect) Placeholder(n int) string {
	if n <= 0 {
		n = 1
	}
	return "$" + strconv.Itoa(n)
}
func (postgresDialect) JSONType() string { return "JSONB" }
func (postgresDialect) NowExpr() string  { return "NOW()" }

const (
	defaultSQLiteMaxOpenConns  = 25
	defaultSQLiteMaxIdleConns  = 25
	defaultSQLiteConnMaxLife   = 5 * time.Minute
	defaultSQLiteBusyTimeoutMS = 10000

	defaultPostgresMaxOpenConns = 25
	defaultPostgresMaxIdleConns = 25
	defaultPostgresConnMaxLife  = 30 * time.Minute
)

var (
	mu       sync.Mutex
	handles  = map[string]*Handle{}
	migrated = map[string]bool{}
	openSQL  = sql.Open
)

func Open(ctx context.Context, cfg Config) (*Handle, error) {
	driver := cfg.Driver
	if driver == "" {
		driver = DriverSQLite
	}
	key, sqlDriver, dsn, dialect, pool, err := resolve(cfg, driver)
	if err != nil {
		return nil, err
	}

	mu.Lock()
	defer mu.Unlock()

	handle := handles[key]
	openedNow := false
	if handle == nil {
		if driver == DriverSQLite {
			if err := os.MkdirAll(filepath.Dir(SQLitePath(cfg.DataDir)), 0755); err != nil {
				return nil, fmt.Errorf("sqlite: create data dir: %w", err)
			}
		}
		opened, err := openSQL(sqlDriver, dsn)
		if err != nil {
			return nil, fmt.Errorf("%s: open db: %w", driver, err)
		}
		applyPool(opened, pool)
		handle = &Handle{db: opened, driver: driver, dialect: dialect}
		handles[key] = handle
		openedNow = true
	}

	migrationKey := key + "|migration:" + normalizeMigrationKey(cfg.MigrationKey)
	if cfg.Migrate != nil && !migrated[migrationKey] {
		if err := cfg.Migrate(ctx, handle.db, driver); err != nil {
			if openedNow {
				handle.db.Close()
				delete(handles, key)
			}
			return nil, err
		}
		migrated[migrationKey] = true
	}
	return handle, nil
}

func normalizeMigrationKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return "default"
	}
	return key
}

func SQLitePath(dataDir string) string {
	return fsutil.GetSQLitePath(strings.TrimSpace(dataDir))
}

func SQLiteDSN(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", sqliteBusyTimeoutMS()))
	u := url.URL{Scheme: "file", Path: path, RawQuery: q.Encode()}
	return u.String()
}

func resolve(cfg Config, driver Driver) (key, sqlDriver, dsn string, dialect Dialect, pool PoolConfig, err error) {
	switch driver {
	case DriverSQLite:
		path := SQLitePath(cfg.DataDir)
		if strings.TrimSpace(path) == "" {
			return "", "", "", nil, PoolConfig{}, fmt.Errorf("sqlite: data dir is required")
		}
		return "sqlite:" + path, "sqlite", SQLiteDSN(path), sqliteDialect{}, sqlitePool(cfg.Pool), nil
	case DriverPostgres:
		dsn := strings.TrimSpace(cfg.DatabaseURL)
		if dsn == "" {
			return "", "", "", nil, PoolConfig{}, fmt.Errorf("postgres: database url is required")
		}
		return "postgres:" + dsn, "pgx", dsn, postgresDialect{}, postgresPool(cfg.Pool), nil
	default:
		return "", "", "", nil, PoolConfig{}, fmt.Errorf("db: unsupported driver %q", driver)
	}
}

func applyPool(db *sql.DB, pool PoolConfig) {
	maxOpen := pool.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 1
	}
	maxIdle := pool.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = maxOpen
	}
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	if pool.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(pool.ConnMaxLifetime)
	}
}

func sqlitePool(pool PoolConfig) PoolConfig {
	if pool.MaxOpenConns <= 0 {
		pool.MaxOpenConns = envInt("AGEN8_SQLITE_MAX_OPEN_CONNS", defaultSQLiteMaxOpenConns)
	}
	if pool.MaxIdleConns <= 0 {
		pool.MaxIdleConns = envInt("AGEN8_SQLITE_MAX_IDLE_CONNS", defaultSQLiteMaxIdleConns)
	}
	if pool.ConnMaxLifetime <= 0 {
		pool.ConnMaxLifetime = envDuration("AGEN8_SQLITE_CONN_MAX_LIFETIME", defaultSQLiteConnMaxLife)
	}
	return pool
}

func postgresPool(pool PoolConfig) PoolConfig {
	if pool.MaxOpenConns <= 0 {
		pool.MaxOpenConns = envInt("AGEN8_POSTGRES_MAX_OPEN_CONNS", defaultPostgresMaxOpenConns)
	}
	if pool.MaxIdleConns <= 0 {
		pool.MaxIdleConns = envInt("AGEN8_POSTGRES_MAX_IDLE_CONNS", defaultPostgresMaxIdleConns)
	}
	if pool.ConnMaxLifetime <= 0 {
		pool.ConnMaxLifetime = envDuration("AGEN8_POSTGRES_CONN_MAX_LIFETIME", defaultPostgresConnMaxLife)
	}
	return pool
}

func sqliteBusyTimeoutMS() int {
	return envInt("AGEN8_SQLITE_BUSY_TIMEOUT_MS", defaultSQLiteBusyTimeoutMS)
}

func envInt(name string, def int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return def
	}
	return v
}

func envDuration(name string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
