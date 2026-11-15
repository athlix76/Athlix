package modules_test

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules"
	"athlix/backend/internal/testutil"
)

// TestAllModulesRegister builds the real router with every module. gin panics on route
// conflicts (e.g. /events/:id vs /events/:eventId), so this catches them before deploy.
// ROUTES_OUT=<file> writes "METHOD path" lines (used by the API contract cross-check).
func TestAllModulesRegister(t *testing.T) {
	d, _ := testutil.Deps(t, nil)
	mods := modules.All(d)
	r := app.NewRouter(d, mods)
	app.RegisterEventHandlers(d, mods)
	jobs := app.CollectJobs(mods)
	routes := r.Routes()
	if len(routes) < 100 {
		t.Fatalf("only %d routes registered", len(routes))
	}
	seen := map[string]bool{}
	for _, m := range mods {
		if seen[m.Name()] {
			t.Fatalf("duplicate module name %q", m.Name())
		}
		seen[m.Name()] = true
	}
	for _, j := range []string{"location.expire_residence", "location.expiry_reminders", "location.purge_proofs"} {
		if jobs[j] == nil {
			t.Fatalf("job %s not registered", j)
		}
	}
	if out := os.Getenv("ROUTES_OUT"); out != "" {
		lines := make([]string, 0, len(routes))
		for _, rt := range routes {
			lines = append(lines, rt.Method+" "+rt.Path)
		}
		sort.Strings(lines)
		if err := os.WriteFile(out, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestSchedulerMatchesJobs keeps infra/modules/scheduler in step with the modules' Jobs():
// every registered job has a schedule and every schedule names a registered job.
func TestSchedulerMatchesJobs(t *testing.T) {
	src, err := os.ReadFile("../../../infra/modules/scheduler/main.tf")
	if err != nil {
		t.Skip("infra not next to backend:", err)
	}
	scheduled := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*"([a-z_]+\.[a-z_]+)"\s*=\s*"cron\(`).FindAllStringSubmatch(string(src), -1) {
		scheduled[m[1]] = true
	}
	d, _ := testutil.Deps(t, nil)
	jobs := app.CollectJobs(modules.All(d))
	for name := range jobs {
		if !scheduled[name] {
			t.Errorf("job %s has no schedule in infra/modules/scheduler/main.tf", name)
		}
	}
	for name := range scheduled {
		if jobs[name] == nil {
			t.Errorf("schedule %s names no registered job", name)
		}
	}
}
