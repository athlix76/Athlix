package db

import (
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestWithHost(t *testing.T) {
	s := Secret{Host: "rds.internal", Port: 3306}
	if got := s.WithHost(""); got.Host != "rds.internal" {
		t.Fatal(got)
	}
	if got := s.WithHost("127.0.0.1:13306"); got.Host != "127.0.0.1" || got.Port != 13306 {
		t.Fatal(got)
	}
	if got := s.WithHost("bastion"); got.Host != "bastion" || got.Port != 3306 {
		t.Fatal(got)
	}
}

func TestForceUTC(t *testing.T) {
	out, err := ForceUTC("u:p@tcp(127.0.0.1:3306)/athlix?loc=Local&time_zone=%27%2B05%3A30%27")
	if err != nil {
		t.Fatal(err)
	}
	c, err := mysql.ParseDSN(out)
	if err != nil || c.Loc != time.UTC || !c.ParseTime || c.Params["time_zone"] != "'+00:00'" {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := ForceUTC("u:secret-pw@tcp(bad"); err == nil || strings.Contains(err.Error(), "secret-pw") {
		t.Fatalf("bad DSN must fail without echoing it: %v", err)
	}
}
