package eligibility

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func expectPlacesLoad(mock sqlmock.Sqlmock, district string) {
	mock.ExpectQuery("FROM states WHERE active = 1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "is_ut"}).AddRow(TestStateTN, "TN", "Tamil Nadu", false))
	mock.ExpectQuery("FROM districts").
		WillReturnRows(sqlmock.NewRows([]string{"id", "state_id", "name", "aliases", "active"}).AddRow(TestDistrictThanj, TestStateTN, district, "[]", true))
}

func versionedPlaces(t *testing.T) (*Places, *Places, sqlmock.Sqlmock, sqlmock.Sqlmock, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	mk := func() (*Places, sqlmock.Sqlmock) {
		conn, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		p := NewPlaces(conn, time.Hour)
		p.UseVersionStore(rdb, time.Millisecond)
		return p, mock
	}
	a, ma := mk()
	b, mb := mk()
	return a, b, ma, mb, mr
}

// An edit on instance A (Bump) makes instance B reload at its next version check, long
// before the TTL.
func TestPlacesBumpReloadsOtherInstance(t *testing.T) {
	ctx := context.Background()
	a, b, ma, mb, _ := versionedPlaces(t)
	expectPlacesLoad(mb, "Thanjavur")
	if b.DistrictName(ctx, TestDistrictThanj) != "Thanjavur" {
		t.Fatal("first load")
	}
	time.Sleep(2 * time.Millisecond)
	if b.DistrictName(ctx, TestDistrictThanj) != "Thanjavur" { // version unchanged: no reload
		t.Fatal("cached")
	}
	if err := a.Bump(ctx); err != nil {
		t.Fatal(err)
	}
	expectPlacesLoad(mb, "Thanjavur (new)")
	time.Sleep(2 * time.Millisecond)
	if got := b.DistrictName(ctx, TestDistrictThanj); got != "Thanjavur (new)" {
		t.Fatalf("B must reload after the bump, got %q", got)
	}
	if err := mb.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := ma.ExpectationsWereMet(); err != nil { // A never loaded
		t.Fatal(err)
	}
}

// Redis down: no error, no request failure, the cached data keeps serving (TTL fallback).
func TestPlacesRedisDownFallsBackToTTL(t *testing.T) {
	ctx := context.Background()
	a, b, _, mb, mr := versionedPlaces(t)
	expectPlacesLoad(mb, "Thanjavur")
	if b.DistrictName(ctx, TestDistrictThanj) != "Thanjavur" {
		t.Fatal("first load")
	}
	mr.Close()
	time.Sleep(2 * time.Millisecond)
	if b.DistrictName(ctx, TestDistrictThanj) != "Thanjavur" {
		t.Fatal("must keep serving the cache")
	}
	if err := a.Bump(ctx); err == nil {
		t.Fatal("bump reports the Redis error for logging")
	}
	if err := mb.ExpectationsWereMet(); err != nil { // no extra reload
		t.Fatal(err)
	}
}

// Without a version store Bump only invalidates locally; static caches never reload.
func TestPlacesBumpWithoutStore(t *testing.T) {
	ctx := context.Background()
	p := TestPlaces()
	if err := p.Bump(ctx); err != nil {
		t.Fatal(err)
	}
	if p.DistrictName(ctx, TestDistrictThanj) != "Thanjavur" {
		t.Fatal("static cache must keep its data")
	}
}
