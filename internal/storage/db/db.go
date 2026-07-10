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

	_ "modernc.org/sqlite"

	"github.com/tinoosan/agen8/pkg/fsutil"
)

type Driver string

const (
	DriverSQLite Driver = "sqlite"
)

type Config struct {
	Driver       Driver
	DataDir      string
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

const (
	defaultSQLiteMaxOpenConns  = 25
	defaultSQLiteMaxIdleConns  = 25
	defaultSQLiteConnMaxLife   = 5 * time.Minute
	defaultSQLiteBusyTimeoutMS = 10000
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
	if driver != DriverSQLite {
		return nil, fmt.Errorf("db: Agen8 supports SQLite storage only")
	}
	dbPath := ""
	if driver == DriverSQLite {
		dbPath = SQLitePath(cfg.DataDir)
		if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
			return nil, fmt.Errorf("sqlite: create data dir: %w", err)
		}
		mu.Lock()
		if err := ensureSQLiteFileSecure(dbPath); err != nil {
			mu.Unlock()
			return nil, fmt.Errorf("sqlite: secure db file: %w", err)
		}
		mu.Unlock()
	}
	key, sqlDriver, dsn, dialect, pool, err := resolve(cfg)
	if err != nil {
		return nil, err
	}

	mu.Lock()
	defer mu.Unlock()

	handle := handles[key]
	openedNow := false
	if handle == nil {
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
				_ = handle.db.Close()
				delete(handles, key)
			}
			return nil, err
		}
		migrated[migrationKey] = true
	}
	return handle, nil
}

func ensureSQLiteFileSecure(path string) error {
	if path == "" {
		return fmt.Errorf("sqlite: db path is required")
	}
	dir := filepath.Dir(path)
	name := filepath.Base(path)
	if dir == "." || name == "." || name == string(filepath.Separator) {
		return fmt.Errorf("sqlite: db path %q is invalid", path)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open sqlite root %s: %w", dir, err)
	}
	defer root.Close()
	if info, err := root.Lstat(name); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("sqlite: db file must not be a symlink")
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("sqlite: db path is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat sqlite db file: %w", err)
	}
	file, err := root.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if closeErr := file.Close(); closeErr != nil {
		return fmt.Errorf("close %s: %w", path, closeErr)
	}
	if err := root.Chmod(name, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
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

func resolve(cfg Config) (key, sqlDriver, dsn string, dialect Dialect, pool PoolConfig, err error) {
	path := SQLitePath(cfg.DataDir)
	if strings.TrimSpace(path) == "" {
		return "", "", "", nil, PoolConfig{}, fmt.Errorf("sqlite: data dir is required")
	}
	return "sqlite:" + path, "sqlite", SQLiteDSN(path), sqliteDialect{}, sqlitePool(cfg.Pool), nil
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
