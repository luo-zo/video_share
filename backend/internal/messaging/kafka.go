package messaging

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

type KafkaPublisher struct {
	client *kgo.Client
}

func NewKafkaPublisher(brokers []string, clientID string) (*KafkaPublisher, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ClientID(clientID+"-outbox"),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka publisher: %w", err)
	}
	return &KafkaPublisher{client: client}, nil
}

func (p *KafkaPublisher) Ping(ctx context.Context) error {
	if err := p.client.Ping(ctx); err != nil {
		return fmt.Errorf("ping Kafka: %w", err)
	}
	return nil
}

func (p *KafkaPublisher) Publish(ctx context.Context, topic, key string, value []byte) error {
	record := &kgo.Record{Topic: topic, Key: []byte(key), Value: value}
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("publish Kafka record: %w", err)
	}
	return nil
}

func (p *KafkaPublisher) Close() { p.client.Close() }

type KafkaConsumer struct {
	client *kgo.Client
}

func NewKafkaConsumer(brokers []string, clientID, group, topic string) (*KafkaConsumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ClientID(clientID+"-worker"),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka consumer: %w", err)
	}
	return &KafkaConsumer{client: client}, nil
}

func (c *KafkaConsumer) Ping(ctx context.Context) error {
	if err := c.client.Ping(ctx); err != nil {
		return fmt.Errorf("ping Kafka: %w", err)
	}
	return nil
}

func (c *KafkaConsumer) Poll(ctx context.Context) ([]*kgo.Record, error) {
	fetches := c.client.PollFetches(ctx)
	if err := fetches.Err(); err != nil {
		return nil, fmt.Errorf("poll Kafka: %w", err)
	}
	records := make([]*kgo.Record, 0, fetches.NumRecords())
	fetches.EachRecord(func(record *kgo.Record) {
		records = append(records, record)
	})
	return records, nil
}

func (c *KafkaConsumer) Commit(ctx context.Context, record *kgo.Record) error {
	if err := c.client.CommitRecords(ctx, record); err != nil {
		return fmt.Errorf("commit Kafka record: %w", err)
	}
	return nil
}

func (c *KafkaConsumer) Close() { c.client.Close() }
