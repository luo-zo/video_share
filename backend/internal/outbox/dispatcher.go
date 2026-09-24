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
	repository     Repository
	publisher      Publisher
	pollInterval   time.Duration
	publishTimeout time.Duration
	batchSize      int
	log            *slog.Logger
}

const DefaultPublishTimeout = 5 * time.Second

func NewDispatcher(repository Repository, publisher Publisher, pollInterval time.Duration, batchSize int, log *slog.Logger) *Dispatcher {
	return NewDispatcherWithTimeout(repository, publisher, pollInterval, DefaultPublishTimeout, batchSize, log)
}

func NewDispatcherWithTimeout(repository Repository, publisher Publisher, pollInterval, publishTimeout time.Duration, batchSize int, log *slog.Logger) *Dispatcher {
	if log == nil {
		log = slog.Default()
	}
	if publishTimeout <= 0 {
		publishTimeout = DefaultPublishTimeout
	}
	return &Dispatcher{repository: repository, publisher: publisher, pollInterval: pollInterval, publishTimeout: publishTimeout, batchSize: batchSize, log: log}
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
	batchSize := max(1, d.batchSize)
	lease := max(30*time.Second, d.pollInterval*10, d.publishTimeout*time.Duration(batchSize)+d.pollInterval*10)
	events, err := d.repository.ClaimDue(ctx, d.batchSize, lease)
	if err != nil {
		return err
	}
	var result error
	for i := range events {
		event := events[i]
		publishCtx, cancel := context.WithTimeout(ctx, d.publishTimeout)
		publishErr := d.publisher.Publish(publishCtx, event.Topic, event.EventKey, event.Payload)
		cancel()
		if publishErr != nil {
			delay := retryDelay(d.pollInterval, event.Attempts)
			markErr := d.repository.MarkFailed(ctx, event.ID, publishErr.Error(), time.Now().UTC().Add(delay))
			result = errors.Join(result, publishErr, markErr)
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
