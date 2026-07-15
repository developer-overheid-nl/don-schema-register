package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRefresher struct {
	count  int
	err    error
	called int
}

func (f *fakeRefresher) RefreshChangedSchemas(ctx context.Context) (int, error) {
	f.called++
	return f.count, f.err
}

func TestNewSchemaRefreshJobNilRefresher(t *testing.T) {
	if job := NewSchemaRefreshJob(nil, nil); job != nil {
		t.Fatalf("job = %#v, want nil", job)
	}
}

func TestSchemaRefreshJobRunOnce(t *testing.T) {
	refresher := &fakeRefresher{count: 2}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := &SchemaRefreshJob{refresher: refresher, ctx: ctx}
	job.runOnce()

	if refresher.called != 1 {
		t.Fatalf("called = %d, want 1", refresher.called)
	}
}

func TestSchemaRefreshJobRunOnceHandlesError(t *testing.T) {
	refresher := &fakeRefresher{err: errors.New("boom")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := &SchemaRefreshJob{refresher: refresher, ctx: ctx}
	job.runOnce()

	if refresher.called != 1 {
		t.Fatalf("called = %d, want 1", refresher.called)
	}
}

func TestSchemaRefreshJobStop(t *testing.T) {
	(*SchemaRefreshJob)(nil).Stop()

	ctx, cancel := context.WithCancel(context.Background())
	job := &SchemaRefreshJob{ctx: ctx, cancel: cancel}
	job.Stop()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("context was not cancelled")
	}
}

func TestSchemaRefreshJobLoopStopsWhenContextCancelled(t *testing.T) {
	refresher := &fakeRefresher{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	job := &SchemaRefreshJob{
		refresher: refresher,
		location:  time.UTC,
		ctx:       ctx,
	}
	job.loop()

	if refresher.called != 0 {
		t.Fatalf("called = %d, want 0", refresher.called)
	}
}

func TestNextRunAt(t *testing.T) {
	loc := time.FixedZone("test", 3600)
	before := time.Date(2026, 7, 15, 6, 30, 0, 0, loc)
	run := nextRunAt(before, 7, 0)
	if !run.Equal(time.Date(2026, 7, 15, 7, 0, 0, 0, loc)) {
		t.Fatalf("run = %s, want same day 07:00", run)
	}

	after := time.Date(2026, 7, 15, 7, 0, 0, 0, loc)
	run = nextRunAt(after, 7, 0)
	if !run.Equal(time.Date(2026, 7, 16, 7, 0, 0, 0, loc)) {
		t.Fatalf("run = %s, want next day 07:00", run)
	}
}
