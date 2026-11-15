// Package db opens MySQL and provides transaction and error helpers.
//
// ALL SQL uses ? bind parameters. Never build SQL with fmt.Sprintf/+ around a value.
// Dynamic identifiers (sort column) must go through a hard-coded allowlist map.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Secret is the JSON shape of the "db" secret (RDS-managed secret format).
type Secret struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	DBName   string `json:"dbname"`
}

// WithHost returns a copy with Host (and Port, if given as host:port) replaced. Used by the
// DB-only tools to reach RDS through an SSH tunnel (config DB_HOST_OVERRIDE); "" = unchanged.
func (s Secret) WithHost(hostPort string) Secret {
	if hostPort == "" {
		return s
	}
	if h, p, err := net.SplitHostPort(hostPort); err == nil {
		if n, err := strconv.Atoi(p); err == nil {
			s.Host, s.Port = h, n
			return s
		}
	}
	s.Host = hostPort
	return s
}

// DSN builds a go-sql-driver DSN from the secret.
func (s Secret) DSN() string {
	c := mysql.NewConfig()
	c.User, c.Passwd, c.Net = s.Username, s.Password, "tcp"
	port := s.Port
	if port == 0 {
		port = 3306
	}
	c.Addr = fmt.Sprintf("%s:%d", s.Host, port)
	c.DBName = s.DBName
	c.ParseTime = true
	c.Loc = time.UTC
	c.Params = map[string]string{"charset": "utf8mb4", "time_zone": "'+00:00'"}
	c.MultiStatements = false
	c.TLSConfig = "preferred"
	return c.FormatDSN()
}

// Open connects with pool settings sized for Lambda (few connections per instance).
func Open(dsn string, maxOpen int) (*sql.DB, error) {
	dsn, err := ForceUTC(dsn)
	if err != nil {
		return nil, err
	}
	d, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, errors.New("db: invalid DSN")
	}
	d.SetMaxOpenConns(maxOpen)
	d.SetMaxIdleConns(maxOpen)
	d.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.PingContext(ctx); err != nil {
		return nil, errors.New("db: cannot reach database") // never include the DSN
	}
	return d, nil
}

// Querier is satisfied by *sql.DB and *sql.Tx so repositories work inside or outside a tx.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// WithTx runs fn in a transaction, rolling back on error or panic.
func WithTx(ctx context.Context, d *sql.DB, fn func(tx *sql.Tx) error) (err error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// IsNoRows reports sql.ErrNoRows.
func IsNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }

// IsDuplicate reports a MySQL duplicate-key error (1062).
func IsDuplicate(err error) bool {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1062
	}
	return err != nil && strings.Contains(err.Error(), "Duplicate entry")
}

// Placeholders returns "?,?,?" for n values (for IN lists). n must be > 0.
func Placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// Int64Args converts ids to []any for IN-list args.
func Int64Args(ids []int64) []any {
	out := make([]any, len(ids))
	for i, v := range ids {
		out[i] = v
	}
	return out
}

// NullString returns nil for "" so optional columns store NULL.
func NullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// NullInt64 returns nil for 0.
func NullInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// ForceUTC makes every connection work in UTC whatever the DSN or server default says:
// parseTime, loc=UTC and session time_zone '+00:00'. Without it a local MySQL running in IST
// stores CURRENT_TIMESTAMP defaults (e.g. listing_fee_rules.effective_from) hours ahead of the
// app clock, and time-window queries silently miss rows. The error never contains the DSN.
func ForceUTC(dsn string) (string, error) {
	c, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", errors.New("db: invalid DSN")
	}
	c.ParseTime = true
	c.Loc = time.UTC
	if c.Params == nil {
		c.Params = map[string]string{}
	}
	c.Params["time_zone"] = "'+00:00'"
	return c.FormatDSN(), nil
}
