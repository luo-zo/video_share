package outbox

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type Publisher interface {
	Publish(ctx context.Context, topic, key string, value []byte) error
}

type Dispatcher struct {
	repository   Repository
	publisher    Publisher
	pollInterval time.Duration
	batchSize    int
	log          *slog.Logger
}

func NewDispatcher(repository Repository, publisher Publisher, pollInterval time.Duration, batchSize int, log *slog.Logger) *Dispatcher {
	if log == nil {
		log = slog.Default()
	}
	return &Dispatcher{repository: repository, publisher: publisher, pollInterval: pollInterval, batchSize: batchSize, log: log}
}

func (d *Dispatcher) Run(ctx context.Context) {
	d.dispatchAndLog(ctx)
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.dispatchAndLog(ctx)
		}
	}
}

func (d *Dispatcher) dispatchAndLog(ctx context.Context) {
	if err := d.DispatchOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		d.log.Warn("outbox dispatch failed", "error", err)
	}
}

func (d *Dispatcher) DispatchOnce(ctx context.Context) error {
	lease := max(30*time.Second, d.pollInterval*10)
	events, err := d.repository.ClaimDue(ctx, d.batchSize, lease)
	if err != nil {
		return err
	}
	var result error
	for i := range events {
		event := events[i]
		if err := d.publisher.Publish(ctx, event.Topic, event.EventKey, event.Payload); err != nil {
			delay := retryDelay(d.pollInterval, event.Attempts)
			markErr := d.repository.MarkFailed(ctx, event.ID, err.Error(), time.Now().UTC().Add(delay))
			result = errors.Join(result, err, markErr)
			continue
		}
		if err := d.repository.MarkPublished(ctx, event.ID); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func retryDelay(base time.Duration, attempts uint) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	shift := min(attempts, 6)
	return base * time.Duration(1<<shift)
}
