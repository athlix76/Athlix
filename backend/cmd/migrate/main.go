// Command migrate applies backend/migrations/*.sql in filename order, once each, recording
// them in schema_migrations. Locally: `make migrate` (uses DB_DSN). In AWS it runs as a Lambda
// (invoked by the deploy workflow) and reads the "db" secret.
//
// Migrations are forward-only. Never edit an applied file; add a new numbered one.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	"athlix/backend/internal/platform/config"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/secrets"
	"athlix/backend/migrations"
)

func main() {
	slog.SetDefault(logger.New(os.Getenv("LOG_LEVEL")))
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		lambda.Start(func(ctx context.Context) (string, error) {
			n, err := run(ctx)
			return fmt.Sprintf("applied %d migrations", n), err
		})
		return
	}
	list := flag.Bool("list", false, "only list pending migrations")
	flag.Parse()
	if *list {
		slog.Info("migrations embedded", slog.Any("files", files()))
		return
	}
	n, err := run(context.Background())
	if err != nil {
		slog.Error("migrate failed", slog.String("err", err.Error()))
		os.Exit(1)
	}
	slog.Info("migrate done", slog.Int("applied", n))
}

func files() []string {
	names, _ := fs.Glob(migrations.FS, "*.sql")
	sort.Strings(names)
	return names
}

func dsn(ctx context.Context) (string, error) {
	cfg, err := config.LoadDBOnly()
	if err != nil {
		return "", err
	}
	if cfg.IsLocal() {
		if cfg.DBDSN == "" {
			return "", errors.New("DB_DSN is required when ENV=local")
		}
		return cfg.DBDSN, nil
	}
	ac, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		return "", errors.New("aws config load failed")
	}
	var s db.Secret
	if err := secrets.NewAWS(ac).GetJSON(ctx, cfg.SecretName("db"), &s); err != nil {
		return "", err
	}
	return s.WithHost(cfg.DBHostOverride).DSN(), nil
}

func run(ctx context.Context) (int, error) {
	d, err := dsn(ctx)
	if err != nil {
		return 0, err
	}
	conn, err := db.Open(d, 2)
	if err != nil {
		return 0, errors.New("database open failed")
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(128) NOT NULL PRIMARY KEY,
		applied_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return 0, err
	}
	applied := 0
	for _, name := range files() {
		var n int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, name).Scan(&n); err != nil {
			return applied, err
		}
		if n > 0 {
			continue
		}
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			return applied, err
		}
		if err := apply(ctx, conn, name, string(body)); err != nil {
			return applied, fmt.Errorf("%s: %w", name, err)
		}
		slog.Info("applied", slog.String("file", name))
		applied++
	}
	return applied, nil
}

// apply runs each statement. MySQL DDL auto-commits, so a failed file must be fixed forward
// (the error says which statement). The file is recorded only after every statement succeeds.
func apply(ctx context.Context, conn *sql.DB, name, body string) error {
	for i, stmt := range Split(body) {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("statement %d: %w", i+1, err)
		}
	}
	_, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, name)
	return err
}

// Split splits a SQL file into statements on ';' outside quotes and comments.
func Split(sqlText string) []string {
	var out []string
	var b strings.Builder
	var quote rune
	lineComment := false
	rs := []rune(sqlText)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if lineComment {
			if r == '\n' {
				lineComment = false
				b.WriteRune(r)
			}
			continue
		}
		if quote != 0 {
			b.WriteRune(r)
			if r == '\\' && i+1 < len(rs) {
				i++
				b.WriteRune(rs[i])
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		switch {
		case r == '\'' || r == '"' || r == '`':
			quote = r
			b.WriteRune(r)
		case r == '-' && i+1 < len(rs) && rs[i+1] == '-':
			lineComment = true
		case r == ';':
			if s := strings.TrimSpace(b.String()); s != "" {
				out = append(out, s)
			}
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		out = append(out, s)
	}
	return out
}
