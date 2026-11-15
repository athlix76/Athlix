package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	lambdaevents "github.com/aws/aws-lambda-go/events"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
)

func newWorker(t *testing.T, failType string) (*worker, *[]string, *[]string) {
	t.Helper()
	var seen, ran []string
	disp := events.NewDispatcher()
	disp.On("a.ok", func(_ context.Context, e events.Event) error { seen = append(seen, e.Type); return nil })
	disp.On(failType, func(context.Context, events.Event) error { return errors.New("db down") })
	jobs := map[string]app.JobFunc{
		"x.ok":   func(context.Context) error { ran = append(ran, "x.ok"); return nil },
		"x.fail": func(context.Context) error { return errors.New("boom") },
	}
	return &worker{dispatch: disp.Dispatch, jobs: jobs, log: slog.New(slog.NewTextHandler(io.Discard, nil))}, &seen, &ran
}

func body(t *testing.T, typ string) string {
	b, err := json.Marshal(events.New(typ, 1, events.UserPayload{UserID: 1}))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSQSPartialBatchFailures(t *testing.T) {
	w, seen, _ := newWorker(t, "b.fail")
	batch := lambdaevents.SQSEvent{Records: []lambdaevents.SQSMessage{
		{MessageId: "m1", Body: body(t, "a.ok")},
		{MessageId: "m2", Body: body(t, "b.fail")},
		{MessageId: "m3", Body: "not json"},            // dropped, never retried
		{MessageId: "m4", Body: body(t, "no.handler")}, // nobody listens: success
	}}
	raw, _ := json.Marshal(batch)
	out, err := w.Handle(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	resp := out.(lambdaevents.SQSEventResponse)
	if len(resp.BatchItemFailures) != 1 || resp.BatchItemFailures[0].ItemIdentifier != "m2" {
		t.Fatalf("failures = %+v", resp.BatchItemFailures)
	}
	if len(*seen) != 1 || (*seen)[0] != "a.ok" {
		t.Fatalf("seen = %v", *seen)
	}
}

func TestSQSAllOKReturnsEmptyFailures(t *testing.T) {
	w, _, _ := newWorker(t, "b.fail")
	resp := w.HandleSQS(context.Background(), lambdaevents.SQSEvent{Records: []lambdaevents.SQSMessage{{MessageId: "m1", Body: body(t, "a.ok")}}})
	b, _ := json.Marshal(resp)
	if string(b) != `{"batchItemFailures":[]}` {
		t.Fatalf("resp = %s", b)
	}
}

func TestJobPayloads(t *testing.T) {
	w, _, ran := newWorker(t, "b.fail")
	ctx := context.Background()
	if _, err := w.Handle(ctx, json.RawMessage(`{"job":"x.ok"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Handle(ctx, json.RawMessage(`{"source":"aws.events","detail":{"job":"x.ok"}}`)); err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 2 {
		t.Fatalf("ran = %v", *ran)
	}
	if _, err := w.Handle(ctx, json.RawMessage(`{"job":"x.fail"}`)); err == nil {
		t.Fatal("job error not returned")
	}
	if _, err := w.Handle(ctx, json.RawMessage(`{"job":"nope"}`)); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("err = %v", err)
	}
	if _, err := w.Handle(ctx, json.RawMessage(`{}`)); err == nil {
		t.Fatal("empty payload accepted")
	}
	if _, err := w.Handle(ctx, json.RawMessage(`[`)); err == nil {
		t.Fatal("bad JSON accepted")
	}
}

// Jobs come from every module's Jobs() through app.CollectJobs, so a new module's jobs
// (e.g. locationidentity's location.*) are dispatched by name with no worker change.
type jobModule struct{ names []string }

func (jobModule) Name() string        { return "m" }
func (jobModule) Routes(r app.Routes) {}
func (m jobModule) Jobs() map[string]app.JobFunc {
	out := map[string]app.JobFunc{}
	for _, n := range m.names {
		out[n] = func(context.Context) error { return nil }
	}
	return out
}

func TestJobsCollectedFromAllModules(t *testing.T) {
	jobs := app.CollectJobs([]app.Module{
		jobModule{[]string{"notification.event_reminders"}},
		jobModule{[]string{"location.expire_residence", "location.expiry_reminders", "location.purge_proofs"}},
	})
	w := &worker{jobs: jobs, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, n := range []string{"notification.event_reminders", "location.expire_residence", "location.expiry_reminders", "location.purge_proofs"} {
		if err := w.RunJob(context.Background(), n); err != nil {
			t.Errorf("%s: %v", n, err)
		}
	}
}
