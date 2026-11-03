package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Uses exclusively isolated queues/exchanges; does not consume application queues.
func TestFeedPublisherConfirmedRouting(t *testing.T) {
	url := os.Getenv("TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("TEST_RABBITMQ_URL not set")
	}
	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	base := &RabbitMQ{Conn: conn}
	name := fmt.Sprintf("feed-test-%d", time.Now().UnixNano())
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	// feedPublisher normally declares durable topology; cleanup only our unique names.
	defer ch.ExchangeDelete(name, false, false)
	defer ch.QueueDelete(name, false, false, false)
	defer ch.QueueDelete(name+".dlx", false, false, false)
	p, err := newFeedPublisher(base, name, name, "publish", "publish")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if p.ch != nil {
			_ = p.ch.Close()
		}
	}()
	event, _ := feedEvent(42, time.Now())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := p.publish(ctx, event); err != nil {
		t.Fatal(err)
	}
	msg, ok, err := ch.Get(name, true)
	if err != nil || !ok {
		t.Fatalf("delivery: %v %v", ok, err)
	}
	var got TimelineEvent
	if err := json.Unmarshal(msg.Body, &got); err != nil || got.VideoID != 42 || msg.MessageId != event.EventID {
		t.Fatalf("unexpected event: %+v %v", got, err)
	}
	if err := ch.QueueUnbind(name, "publish", name, nil); err != nil {
		t.Fatal(err)
	}
	if err := p.publish(ctx, event); err == nil {
		t.Fatal("mandatory return must fail despite broker ack")
	}
	// Next attempt recreates the channel/topology and recovers the binding.
	if err := p.publish(ctx, event); err != nil {
		t.Fatalf("publisher did not recover: %v", err)
	}
}
