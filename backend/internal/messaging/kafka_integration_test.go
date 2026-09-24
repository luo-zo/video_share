//go:build integration

package messaging

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestKafkaPing(t *testing.T) {
	raw := strings.TrimSpace(os.Getenv("TEST_KAFKA_BROKERS"))
	if raw == "" {
		if strings.EqualFold(strings.TrimSpace(os.Getenv("REQUIRE_INTEGRATION_TESTS")), "true") {
			t.Fatal("TEST_KAFKA_BROKERS must be set when REQUIRE_INTEGRATION_TESTS=true")
		}
		t.Skip("TEST_KAFKA_BROKERS not set; skipping Kafka integration test")
	}
	client, err := NewKafkaPublisher(strings.Split(raw, ","), "video-share-integration-test")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}
