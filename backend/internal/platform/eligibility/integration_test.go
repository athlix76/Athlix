//go:build integration

// Integration check against a real MySQL with all migrations applied:
//
//	ATHLIX_TEST_DSN='user:pass@tcp(127.0.0.1:3306)/athlix?parseTime=true' go test -tags integration ./internal/platform/eligibility/
//
// It inserts rows for user ids >= 900000000 and removes nothing outside them.
package eligibility

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"athlix/backend/internal/platform/settings"
)

func TestIntegrationSQL(t *testing.T) {
	dsn := os.Getenv("ATHLIX_TEST_DSN")
	if dsn == "" {
		t.Skip("ATHLIX_TEST_DSN not set")
	}
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()
	const uid = 900000001

	p := NewPlaces(conn, 0)
	d, ok := p.MatchDistrict(ctx, "Tamil Nadu", "Tanjore")
	if !ok || d.Name != "Thanjavur" {
		t.Fatalf("seeded districts: %+v %v", d, ok)
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM user_location_identities WHERE user_id = ?`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO user_location_identities (user_id, identity_type, district_id, state_id, proof_type, status, verified_at)
		VALUES (?, 'native', ?, ?, 'digilocker_aadhaar', 'verified', NOW())`, uid, d.ID, d.StateID); err != nil {
		t.Fatal(err)
	}
	// second verified native must violate the slot key
	if _, err := conn.ExecContext(ctx, `INSERT INTO user_location_identities (user_id, identity_type, district_id, state_id, proof_type, status)
		VALUES (?, 'native', ?, ?, 'birth_certificate', 'verified')`, uid, d.ID, d.StateID); err == nil {
		t.Fatal("expected unique violation for a second verified native")
	}
	c := NewWithPlaces(conn, settings.NewStatic(nil), p)
	ids, err := c.Load(ctx, nil, uid)
	if err != nil || len(ids) != 1 {
		t.Fatalf("load: %v %v", ids, err)
	}
	cond, args := Filter("e", ids, now)
	if _, err := conn.QueryContext(ctx, `SELECT e.id FROM events e WHERE e.status = 'published' AND `+cond, args...); err != nil {
		t.Fatalf("filter SQL: %v\n%s", err, cond)
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM user_location_identities WHERE user_id = ?`, uid); err != nil {
		t.Fatal(err)
	}
}
