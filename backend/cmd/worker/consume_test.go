package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"video_share/internal/messaging"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// fakeConsumer 按顺序回放预设的轮询结果；耗尽后取消 ctx 让 consume 正常退出，
// 否则主循环永不返回，测试无法结束。
type fakeConsumer struct {
	records   [][]*kgo.Record
	errs      []error
	index     int
	cancel    context.CancelFunc
	committed []*kgo.Record
}

func (f *fakeConsumer) Poll(ctx context.Context) ([]*kgo.Record, error) {
	if f.index >= len(f.records) {
		f.cancel()
		return nil, ctx.Err()
	}
	records, err := f.records[f.index], f.errs[f.index]
	f.index++
	return records, err
}

func (f *fakeConsumer) Commit(_ context.Context, record *kgo.Record) error {
	f.committed = append(f.committed, record)
	return nil
}

// committedPosition 给出 Kafka 语义下该分区「下一条待消费」的位点：已提交的最大偏移 + 1。
// 提交 offset=n 意味着 n 已被处理，重投从 n+1 开始；位点一旦越过失败偏移，那条消息就再也不会重投。
func (f *fakeConsumer) committedPosition(partition int32) int64 {
	var highest int64 = -1
	for _, record := range f.committed {
		if record.Partition == partition && record.Offset > highest {
			highest = record.Offset
		}
	}
	return highest + 1
}

type fakeProcessor struct {
	failFor   map[uint64]error // 永久失败：模拟毒丸
	failTimes map[uint64]int   // 前 N 次失败，之后成功：模拟重投后痊愈
	calls     map[uint64]int
	seen      []uint64
}

func (f *fakeProcessor) Process(_ context.Context, event messaging.TranscodeRequested) error {
	f.seen = append(f.seen, event.VideoID)
	if f.calls == nil {
		f.calls = make(map[uint64]int)
	}
	f.calls[event.VideoID]++
	if err, ok := f.failFor[event.VideoID]; ok {
		return err
	}
	if limit, ok := f.failTimes[event.VideoID]; ok && f.calls[event.VideoID] <= limit {
		return errors.New("transcode job not found")
	}
	return nil
}

func transcodeRecord(t *testing.T, videoID uint64) *kgo.Record {
	t.Helper()
	return transcodeRecordAt(t, 0, int64(videoID), videoID)
}

func transcodeRecordAt(t *testing.T, partition int32, offset int64, videoID uint64) *kgo.Record {
	t.Helper()
	payload, err := json.Marshal(messaging.TranscodeRequested{
		EventID:         "event",
		JobID:           "job",
		VideoID:         videoID,
		UserID:          1,
		SourceObjectKey: "videos/1/source.mp4",
		SchemaVersion:   messaging.EventSchemaVersion,
	})
	if err != nil {
		t.Fatalf("marshal transcode event: %v", err)
	}
	return &kgo.Record{Value: payload, Partition: partition, Offset: offset}
}

func TestConsumeKeepsRunningAfterTransientPollError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	record := transcodeRecord(t, 1)
	consumer := &fakeConsumer{
		records: [][]*kgo.Record{nil, {record}},
		errs:    []error{errors.New("group join i/o timeout"), nil},
		cancel:  cancel,
	}
	processor := &fakeProcessor{}

	if err := consume(ctx, testLogger(), consumer, processor); !errors.Is(err, context.Canceled) {
		t.Fatalf("consume returned %v, want context.Canceled", err)
	}
	if len(processor.seen) != 1 || processor.seen[0] != 1 {
		t.Fatalf("processed %v, want [1]", processor.seen)
	}
	if len(consumer.committed) != 1 || consumer.committed[0] != record {
		t.Fatalf("committed %d records, want the one healthy record", len(consumer.committed))
	}
}

func TestConsumeSkipsPoisonRecordAndContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	poison := transcodeRecord(t, 1)
	healthy := transcodeRecord(t, 2)
	consumer := &fakeConsumer{
		records: [][]*kgo.Record{{poison, healthy}},
		errs:    []error{nil},
		cancel:  cancel,
	}
	processor := &fakeProcessor{failFor: map[uint64]error{1: errors.New("transcode job not found")}}

	if err := consume(ctx, testLogger(), consumer, processor); !errors.Is(err, context.Canceled) {
		t.Fatalf("consume returned %v, want context.Canceled", err)
	}
	if len(processor.seen) != 2 || processor.seen[0] != 1 || processor.seen[1] != 2 {
		t.Fatalf("processed %v, want [1 2]", processor.seen)
	}
	// 毒丸之后的健康记录也不能提交：提交 offset=2 会把分区位点推到 3，毒丸的 offset=1 就被永久跳过。
	// 这是至少一次语义的代价——用重复处理换不丢消息（对照 TestConsumeDoesNotCommitPastFailedOffsetInSamePartition）。
	if position := consumer.committedPosition(0); position > 1 {
		t.Fatalf("partition 0 commit position is %d, want <= 1 (committing past the poison drops offset 1)", position)
	}
}

func TestConsumeDiscardsUnparsableRecordAndContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	broken := &kgo.Record{Value: []byte("{not json"), Partition: 0, Offset: 1}
	healthy := transcodeRecord(t, 2)
	consumer := &fakeConsumer{
		records: [][]*kgo.Record{{broken, healthy}},
		errs:    []error{nil},
		cancel:  cancel,
	}
	processor := &fakeProcessor{}

	if err := consume(ctx, testLogger(), consumer, processor); !errors.Is(err, context.Canceled) {
		t.Fatalf("consume returned %v, want context.Canceled", err)
	}
	if len(processor.seen) != 1 || processor.seen[0] != 2 {
		t.Fatalf("processed %v, want [2]", processor.seen)
	}
	if len(consumer.committed) != 2 {
		t.Fatalf("committed %d records, want 2 (broken discarded, healthy handled)", len(consumer.committed))
	}
}

// 失败之后仍要继续处理（保持存活），但绝不能把位点推过失败点：提交 offset=6 会让分区位点变成 7，
// 失败的 offset=5 从此不再重投，等于静默丢一条转码任务。
func TestConsumeDoesNotCommitPastFailedOffsetInSamePartition(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failed := transcodeRecordAt(t, 0, 5, 5)
	next := transcodeRecordAt(t, 0, 6, 6)
	consumer := &fakeConsumer{
		records: [][]*kgo.Record{{failed, next}},
		errs:    []error{nil},
		cancel:  cancel,
	}
	processor := &fakeProcessor{failFor: map[uint64]error{5: errors.New("transcode job not found")}}

	if err := consume(ctx, testLogger(), consumer, processor); !errors.Is(err, context.Canceled) {
		t.Fatalf("consume returned %v, want context.Canceled", err)
	}
	if len(processor.seen) != 2 || processor.seen[0] != 5 || processor.seen[1] != 6 {
		t.Fatalf("processed %v, want [5 6] (a failed record must not stall later ones)", processor.seen)
	}
	if position := consumer.committedPosition(0); position > 5 {
		t.Fatalf("partition 0 commit position advanced to %d, want <= 5 so offset 5 is redelivered", position)
	}
}

// 乱码记录被丢弃时同样是在分区里「提交」一个偏移，因此也要受失败点约束。
func TestConsumeDoesNotCommitPastFailedOffsetWhenLaterRecordIsUnparsable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failed := transcodeRecordAt(t, 0, 5, 5)
	broken := &kgo.Record{Value: []byte("{not json"), Partition: 0, Offset: 6}
	consumer := &fakeConsumer{
		records: [][]*kgo.Record{{failed, broken}},
		errs:    []error{nil},
		cancel:  cancel,
	}
	processor := &fakeProcessor{failFor: map[uint64]error{5: errors.New("transcode job not found")}}

	if err := consume(ctx, testLogger(), consumer, processor); !errors.Is(err, context.Canceled) {
		t.Fatalf("consume returned %v, want context.Canceled", err)
	}
	if position := consumer.committedPosition(0); position > 5 {
		t.Fatalf("partition 0 commit position advanced to %d, want <= 5 so offset 5 is redelivered", position)
	}
}

// 阻塞是按分区的：一个分区的失败不得拖住其他分区的位点。
func TestConsumeStillCommitsUnaffectedPartition(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failed := transcodeRecordAt(t, 0, 5, 5)
	other := transcodeRecordAt(t, 1, 9, 9)
	consumer := &fakeConsumer{
		records: [][]*kgo.Record{{failed, other}},
		errs:    []error{nil},
		cancel:  cancel,
	}
	processor := &fakeProcessor{failFor: map[uint64]error{5: errors.New("transcode job not found")}}

	if err := consume(ctx, testLogger(), consumer, processor); !errors.Is(err, context.Canceled) {
		t.Fatalf("consume returned %v, want context.Canceled", err)
	}
	if position := consumer.committedPosition(1); position != 10 {
		t.Fatalf("partition 1 commit position is %d, want 10 (unaffected partition must keep committing)", position)
	}
	if position := consumer.committedPosition(0); position > 5 {
		t.Fatalf("partition 0 commit position advanced to %d, want <= 5", position)
	}
}

// 失败的消息重投成功之后，该分区必须解除阻塞：只拦「越过失败点」的提交，不能把一次
// 瞬时失败变成永久封禁——那会用无限重复换取不丢消息，比丢消息更难运维。
func TestConsumeCommitsFailedOffsetAfterSuccessfulRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failed := transcodeRecordAt(t, 0, 5, 5)
	next := transcodeRecordAt(t, 0, 6, 6)
	consumer := &fakeConsumer{
		// 第一批：5 失败、6 成功（6 的提交必须被拦下）；第二批：5 重投并成功；
		// 第三批：6 重投并成功，位点恢复前进。
		records: [][]*kgo.Record{{failed, next}, {failed}, {next}},
		errs:    []error{nil, nil, nil},
		cancel:  cancel,
	}
	processor := &fakeProcessor{failTimes: map[uint64]int{5: 1}}

	if err := consume(ctx, testLogger(), consumer, processor); !errors.Is(err, context.Canceled) {
		t.Fatalf("consume returned %v, want context.Canceled", err)
	}
	if len(processor.seen) != 4 || processor.seen[0] != 5 || processor.seen[1] != 6 || processor.seen[2] != 5 || processor.seen[3] != 6 {
		t.Fatalf("processed %v, want [5 6 5 6]", processor.seen)
	}
	if position := consumer.committedPosition(0); position != 7 {
		t.Fatalf("partition 0 commit position is %d, want 7 (a successful retry must unblock the partition)", position)
	}
}

// 解除阻塞是按偏移的：重投成功只清掉那一条，更靠后的失败仍然拦着它后面的提交，直到它自己重投成功。
func TestConsumeClearsOnlyTheRetriedOffset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	consumer := &fakeConsumer{
		records: [][]*kgo.Record{
			{transcodeRecordAt(t, 0, 5, 5), transcodeRecordAt(t, 0, 6, 6)}, // 5、6 都失败
			{transcodeRecordAt(t, 0, 5, 5), transcodeRecordAt(t, 0, 7, 7)}, // 5 重投成功；7 虽成功但被 6 挡住
			{transcodeRecordAt(t, 0, 6, 6), transcodeRecordAt(t, 0, 7, 7)}, // 6 重投成功，7 终于可提交
		},
		errs:   []error{nil, nil, nil},
		cancel: cancel,
	}
	processor := &fakeProcessor{failTimes: map[uint64]int{5: 1, 6: 1}}

	if err := consume(ctx, testLogger(), consumer, processor); !errors.Is(err, context.Canceled) {
		t.Fatalf("consume returned %v, want context.Canceled", err)
	}
	if len(consumer.committed) != 3 ||
		consumer.committed[0].Offset != 5 || consumer.committed[1].Offset != 6 || consumer.committed[2].Offset != 7 {
		t.Fatalf("committed offsets %v, want [5 6 7] in that order", committedOffsets(consumer))
	}
	if position := consumer.committedPosition(0); position != 8 {
		t.Fatalf("partition 0 commit position is %d, want 8 (later messages must eventually commit)", position)
	}
}

func committedOffsets(c *fakeConsumer) []int64 {
	offsets := make([]int64, 0, len(c.committed))
	for _, record := range c.committed {
		offsets = append(offsets, record.Offset)
	}
	return offsets
}
