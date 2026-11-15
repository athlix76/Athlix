// Command admincli is the operator tool for one-off admin tasks that have no API:
//
//	admincli create-admin -email you@example.com -name "You" [-role super_admin]
//	admincli reset-credentials -email you@example.com
//	admincli import-districts -file lgd.csv
//
// It loads config with config.LoadDBOnly (no REDIS_ADDR / MEDIA_BUCKET needed) and builds Deps
// with app.Bootstrap, like cmd/api. One-time credentials go to stdout only, once; they are
// never logged or stored in plain text. See README.md in this folder.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/admin"
	"athlix/backend/internal/modules/locationidentity"
	"athlix/backend/internal/platform/config"
	"athlix/backend/internal/platform/httpx"
)

// bootFunc builds Deps (replaced in tests).
type bootFunc func(ctx context.Context) (*app.Deps, func(), error)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, bootstrap))
}

func bootstrap(ctx context.Context) (*app.Deps, func(), error) {
	cfg, err := config.LoadDBOnly()
	if err != nil {
		return nil, nil, err
	}
	return app.Bootstrap(ctx, cfg)
}

const usage = `usage:
  admincli create-admin -email <email> -name <name> [-role super_admin]
  admincli reset-credentials -email <email>
  admincli import-districts -file <path.csv>
`

// run executes one subcommand and returns the process exit code
// (0 ok, 1 runtime error, 2 usage error).
func run(ctx context.Context, args []string, stdout, stderr io.Writer, boot bootFunc) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	switch args[0] {
	case "create-admin":
		email := fs.String("email", "", "admin email (login)")
		name := fs.String("name", "", "admin display name")
		role := fs.String("role", "super_admin", "admin role name")
		if fs.Parse(args[1:]) != nil {
			return 2
		}
		if strings.TrimSpace(*email) == "" || strings.TrimSpace(*name) == "" {
			fmt.Fprint(stderr, "create-admin: -email and -name are required\n", usage)
			return 2
		}
		if strings.TrimSpace(*role) == "" {
			*role = "super_admin"
		}
		return withDeps(ctx, stderr, boot, func(d *app.Deps) error {
			a, c, err := admin.CreateAdmin(ctx, d, *email, *name, *role)
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "Created admin %d (%s) with role %s.\n", a.ID, a.Email, a.Role.Name)
			printCredentials(stdout, c)
			return nil
		})
	case "reset-credentials":
		email := fs.String("email", "", "admin email")
		if fs.Parse(args[1:]) != nil {
			return 2
		}
		if strings.TrimSpace(*email) == "" {
			fmt.Fprint(stderr, "reset-credentials: -email is required\n", usage)
			return 2
		}
		return withDeps(ctx, stderr, boot, func(d *app.Deps) error {
			c, err := admin.ResetCredentialsByEmail(ctx, d, *email)
			if err != nil {
				return err
			}
			fmt.Fprintln(stdout, "Credentials reset; all sessions of this admin were revoked.")
			printCredentials(stdout, c)
			return nil
		})
	case "import-districts":
		path := fs.String("file", "", "LGD CSV file: state_code,district_name,lgd_code[,aliases]")
		if fs.Parse(args[1:]) != nil {
			return 2
		}
		if strings.TrimSpace(*path) == "" {
			fmt.Fprint(stderr, "import-districts: -file is required\n", usage)
			return 2
		}
		f, err := os.Open(*path)
		if err != nil {
			fmt.Fprintln(stderr, "import-districts: cannot open file:", err)
			return 1
		}
		defer f.Close()
		return withDeps(ctx, stderr, boot, func(d *app.Deps) error {
			res, err := locationidentity.ImportDistricts(ctx, d.DB, f)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(res)
		})
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s", args[0], usage)
		return 2
	}
}

func withDeps(ctx context.Context, stderr io.Writer, boot bootFunc, fn func(d *app.Deps) error) int {
	d, cleanup, err := boot(ctx)
	if err != nil {
		fmt.Fprintln(stderr, "bootstrap:", err)
		return 1
	}
	if cleanup != nil {
		defer cleanup()
	}
	if err := fn(d); err != nil {
		fmt.Fprintln(stderr, "error:", describe(err))
		return 1
	}
	return 0
}

// describe renders an error for the operator. Validation errors list their fields.
func describe(err error) string {
	if he, ok := httpx.As(err); ok {
		msg := he.Code + ": " + he.Message
		for k, v := range he.Fields {
			msg += fmt.Sprintf("\n  %s: %s", k, v)
		}
		return msg
	}
	return err.Error()
}

func printCredentials(w io.Writer, c admin.Credentials) {
	fmt.Fprintln(w, "One-time credentials (shown once, not stored in plain text):")
	fmt.Fprintln(w, "  temporary password:", c.TemporaryPassword)
	fmt.Fprintln(w, "  TOTP otpauth URI:  ", c.TotpURI)
	fmt.Fprintln(w, "Add the URI to an authenticator app, then sign in and change the password.")
}
