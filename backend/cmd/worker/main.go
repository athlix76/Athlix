// Command worker consumes the domain-event queue and runs scheduled jobs.
//
// In AWS Lambda (AWS_LAMBDA_FUNCTION_NAME set) one function handles both invocation shapes:
// an SQS batch (each body is a JSON events.Event, dispatched to every module's handlers, with
// partial batch failures reported) and an EventBridge payload {"job":"<name>"}.
// Locally, `go run ./cmd/worker -job <name>` runs one job and exits (`make worker-job JOB=...`).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules"
	"athlix/backend/internal/platform/config"
)

func main() {
	job := flag.String("job", "", "run one scheduled job by name and exit (e.g. notification.event_reminders)")
	list := flag.Bool("list", false, "print the known job names and exit")
	flag.Parse()

	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	d, cleanup, err := app.Bootstrap(ctx, cfg)
	if err != nil {
		slog.Error("bootstrap", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer cleanup()

	mods := modules.All(d)
	app.RegisterEventHandlers(d, mods)
	w := &worker{dispatch: d.Dispatcher.Dispatch, jobs: app.CollectJobs(mods), log: d.Log}

	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		lambda.Start(w.Handle)
		return
	}
	switch {
	case *list:
		fmt.Println(strings.Join(w.jobNames(), "\n"))
	case *job != "":
		if err := w.RunJob(ctx, *job); err != nil {
			cleanup()
			os.Exit(1)
		}
	default:
		fmt.Println("usage: worker -job <name> | -list   (in AWS this binary is the SQS + EventBridge Lambda)")
	}
}
