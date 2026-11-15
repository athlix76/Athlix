package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	lambdaevents "github.com/aws/aws-lambda-go/events"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
)

// ErrUnknownJob is returned for a job name no module provides.
var ErrUnknownJob = errors.New("worker: unknown job")

// worker holds what one invocation needs: the event dispatcher (every module's handlers are
// registered on it) and the job table from app.CollectJobs.
type worker struct {
	dispatch func(ctx context.Context, e events.Event) error
	jobs     map[string]app.JobFunc
	log      *slog.Logger
}

// invocation is the union of the two payload shapes the worker Lambda receives.
type invocation struct {
	Records []json.RawMessage `json:"Records"` // SQS batch
	Job     string            `json:"job"`     // EventBridge rule input {"job": "<name>"}
	Detail  *struct {
		Job string `json:"job"`
	} `json:"detail"` // EventBridge event with the job in detail
}

// Handle is the Lambda entry point. An SQS batch returns partial batch failures; a job payload
// returns the job's error (so EventBridge retries and the failure shows in metrics).
func (w *worker) Handle(ctx context.Context, raw json.RawMessage) (any, error) {
	var inv invocation
	if err := json.Unmarshal(raw, &inv); err != nil {
		return nil, errors.New("worker: payload is not JSON")
	}
	if len(inv.Records) > 0 {
		var batch lambdaevents.SQSEvent
		if err := json.Unmarshal(raw, &batch); err != nil {
			return nil, errors.New("worker: malformed SQS batch")
		}
		return w.HandleSQS(ctx, batch), nil
	}
	name := inv.Job
	if name == "" && inv.Detail != nil {
		name = inv.Detail.Job
	}
	if name == "" {
		return nil, errors.New("worker: payload has neither Records nor job")
	}
	return map[string]string{"job": name, "status": "ok"}, w.RunJob(ctx, name)
}

// HandleSQS dispatches each record and reports the ones that failed so only those are
// redelivered. A body that is not an Event can never succeed: it is logged (ids only) and
// dropped rather than retried until it reaches the DLQ.
func (w *worker) HandleSQS(ctx context.Context, batch lambdaevents.SQSEvent) lambdaevents.SQSEventResponse {
	resp := lambdaevents.SQSEventResponse{BatchItemFailures: []lambdaevents.SQSBatchItemFailure{}}
	for _, rec := range batch.Records {
		var e events.Event
		if err := json.Unmarshal([]byte(rec.Body), &e); err != nil || e.Type == "" {
			w.log.Warn("worker dropped malformed message", slog.String("messageId", rec.MessageId))
			continue
		}
		if err := w.dispatch(ctx, e); err != nil {
			w.log.Error("event handler failed",
				slog.String("messageId", rec.MessageId), slog.String("type", e.Type), slog.String("err", err.Error()))
			resp.BatchItemFailures = append(resp.BatchItemFailures, lambdaevents.SQSBatchItemFailure{ItemIdentifier: rec.MessageId})
		}
	}
	return resp
}

// RunJob runs one named job from any module's Jobs().
func (w *worker) RunJob(ctx context.Context, name string) error {
	j, ok := w.jobs[name]
	if !ok {
		return fmt.Errorf("%w %q (known: %v)", ErrUnknownJob, name, w.jobNames())
	}
	w.log.Info("job started", slog.String("job", name))
	if err := j(ctx); err != nil {
		w.log.Error("job failed", slog.String("job", name), slog.String("err", err.Error()))
		return err
	}
	w.log.Info("job finished", slog.String("job", name))
	return nil
}

func (w *worker) jobNames() []string {
	out := make([]string, 0, len(w.jobs))
	for n := range w.jobs {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
