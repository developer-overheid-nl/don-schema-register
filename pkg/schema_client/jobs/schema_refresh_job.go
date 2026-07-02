package jobs

import (
	"context"
	"errors"
	"log"
	"time"
)

const (
	refreshHour   = 7
	refreshMinute = 0
	runTimeout    = 120 * time.Minute
	refreshPeriod = 24 * time.Hour
)

type SchemaRefresher interface {
	RefreshChangedSchemas(ctx context.Context) (int, error)
}

// SchemaRefreshJob draait direct na startup en daarna dagelijks om 07:00 een refresh-run.
type SchemaRefreshJob struct {
	refresher SchemaRefresher
	location  *time.Location
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewSchemaRefreshJob start direct een refresh-run en plant daarna een dagelijkse job. Parent context kan nil zijn.
func NewSchemaRefreshJob(refresher SchemaRefresher, parentCtx context.Context) *SchemaRefreshJob {
	if refresher == nil {
		return nil
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)
	job := &SchemaRefreshJob{
		refresher: refresher,
		location:  time.Local,
		ctx:       ctx,
		cancel:    cancel,
	}
	go func() {
		job.runOnce()
		job.loop()
	}()
	return job
}

// Stop beëindigt de job.
func (j *SchemaRefreshJob) Stop() {
	if j == nil || j.cancel == nil {
		return
	}
	j.cancel()
}

func (j *SchemaRefreshJob) loop() {
	for {
		delay := time.Until(nextRunAt(time.Now().In(j.location), refreshHour, refreshMinute))
		timer := time.NewTimer(delay)
		select {
		case <-j.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			j.runOnce()
		}
	}
}

func (j *SchemaRefreshJob) runOnce() {
	runCtx, cancel := context.WithTimeout(j.ctx, runTimeout)
	defer cancel()

	count, err := j.refresher.RefreshChangedSchemas(runCtx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[schema-refresh] run afgebroken: %v", err)
		} else {
			log.Printf("[schema-refresh] run mislukt: %v", err)
		}
		return
	}
	log.Printf("[schema-refresh] run gereed; %d schemas bijgewerkt", count)
}

func nextRunAt(now time.Time, hour, minute int) time.Time {
	loc := now.Location()
	candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
	if !candidate.After(now) {
		candidate = candidate.Add(refreshPeriod)
	}
	return candidate
}
