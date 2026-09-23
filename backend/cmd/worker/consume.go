package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"video_share/internal/messaging"
)

// pollRetryDelay 是 Kafka 轮询失败后的退避时间，避免持续失败时空转打满 CPU。
const pollRetryDelay = time.Second

// eventConsumer 抽象 worker 主循环用到的 Kafka 操作，便于在单元测试中注入假实现。
type eventConsumer interface {
	Poll(ctx context.Context) ([]*kgo.Record, error)
	Commit(ctx context.Context, record *kgo.Record) error
}

type eventProcessor interface {
	Process(ctx context.Context, event messaging.TranscodeRequested) error
}

// commitTracker 阻止同一分区里「失败偏移之后的成功提交」把消费位点推过失败点。
//
// Kafka 的提交是分区级的「下一条待消费」语义：提交偏移 o 会让位点变成 o+1，于是任何仍未
// 处理成功的失败偏移 f <= o 都不会再重投——既丢消息，也和循环里「不提交偏移、交由 Kafka
// 重投」的承诺矛盾。因此某分区一旦有失败，凡是会越过失败点的提交都要拦下（其他分区不受影响）。
//
// 阻塞可以解除：同一条消息重投并处理成功后，它不再构成阻塞（markRecovered），分区恢复提交。
// 否则一次瞬时失败就会把该分区永久封禁，用无限重复换取不丢消息——那比丢消息更难运维。
//
// 仍保留的代价：持续失败的「毒丸」会一直阻塞该分区，重启后从失败点重投，即用重复处理换取
// 不丢消息——这正是至少一次的取舍。若将来要限制重复，需要显式重试次数或死信队列。
type commitTracker struct {
	failed map[int32]map[int64]struct{}
}

// markFailed 记录该分区一个处理失败的偏移。
func (c *commitTracker) markFailed(partition int32, offset int64) {
	if c.failed == nil {
		c.failed = make(map[int32]map[int64]struct{})
	}
	offsets, ok := c.failed[partition]
	if !ok {
		offsets = make(map[int64]struct{})
		c.failed[partition] = offsets
	}
	offsets[offset] = struct{}{}
}

// markRecovered 在失败的消息重投并处理成功后解除它造成的阻塞。
func (c *commitTracker) markRecovered(partition int32, offset int64) {
	offsets, ok := c.failed[partition]
	if !ok {
		return
	}
	delete(offsets, offset)
	if len(offsets) == 0 {
		delete(c.failed, partition)
	}
}

// blocks 判断提交该偏移是否会跳过仍未处理成功的失败偏移（提交 o 即把位点设成 o+1，
// 失败偏移 f <= o 从此不再重投）。命中时返回最早的失败偏移。
func (c *commitTracker) blocks(partition int32, offset int64) (int64, bool) {
	var blocked int64
	found := false
	for failed := range c.failed[partition] {
		if failed <= offset && (!found || failed < blocked) {
			blocked, found = failed, true
		}
	}
	return blocked, found
}

// consume 是 worker 的主循环：单条消息失败与 Kafka 瞬时错误都不能终止整个进程。
// 一条毒消息（例如库中已没有对应任务）或一次消费组加入超时，都会引发崩溃重启循环。
// 只有 ctx 被取消（SIGINT/SIGTERM）或提交偏移失败才返回错误。
func consume(ctx context.Context, log *slog.Logger, consumer eventConsumer, processor eventProcessor) error {
	commits := &commitTracker{}
	for {
		records, err := consumer.Poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Warn("poll transcode events failed, retrying", "error", err)
			timer := time.NewTimer(pollRetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue

		}
		for _, record := range records {
			var event messaging.TranscodeRequested
			if err := json.Unmarshal(record.Value, &event); err != nil {
				log.Error("discard invalid transcode event", "partition", record.Partition, "offset", record.Offset, "error", err)
				if blocked, ok := commits.blocks(record.Partition, record.Offset); ok {
					log.Warn("withholding commit behind an earlier failure",
						"partition", record.Partition, "offset", record.Offset, "blocked_offset", blocked)
					continue
				}
				if err := consumer.Commit(ctx, record); err != nil {
					return err
				}
				continue
			}
			started := time.Now()
			if err := processor.Process(ctx, event); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				// 不提交偏移，交由 Kafka 重投；仅跳过这一条，继续处理后续消息。
				// 记下失败位点，避免同一分区后续的成功提交把位点推过它。
				commits.markFailed(record.Partition, record.Offset)
				log.Error("handle transcode event failed, skipping record",
					"event_id", event.EventID, "job_id", event.JobID, "video_id", event.VideoID,
					"partition", record.Partition, "offset", record.Offset, "error", err)
				continue
			}
			commits.markRecovered(record.Partition, record.Offset)
			if blocked, ok := commits.blocks(record.Partition, record.Offset); ok {
				log.Warn("withholding commit behind an earlier failure",
					"event_id", event.EventID, "partition", record.Partition, "offset", record.Offset, "blocked_offset", blocked)
				continue
			}
			if err := consumer.Commit(ctx, record); err != nil {
				return err
			}
			log.Info("transcode event handled", "event_id", event.EventID, "job_id", event.JobID, "video_id", event.VideoID, "latency_ms", time.Since(started).Milliseconds())
		}
	}
}
