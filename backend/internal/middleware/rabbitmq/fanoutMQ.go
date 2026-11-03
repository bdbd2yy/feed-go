package rabbitmq

import (
	"context"
	"errors"
	"time"
)

const (
	FanoutQueue      = "video.fanout.update.queue"
	fanoutExchange   = "video.fanout.events"
	fanoutBindingKey = "video.fanout.*"
	fanoutPublishRK  = "video.fanout.publish"
)

type FanoutMQ struct{ publisher *feedPublisher }

func NewFanoutMQ(base *RabbitMQ) (*FanoutMQ, error) {
	p, err := newFeedPublisher(base, fanoutExchange, FanoutQueue, fanoutBindingKey, fanoutPublishRK)
	if err != nil {
		return nil, err
	}
	return &FanoutMQ{publisher: p}, nil
}
func (f *FanoutMQ) PublishVideo(ctx context.Context, id uint, at time.Time) error {
	if f == nil || f.publisher == nil {
		return errors.New("fanout mq not initialized")
	}
	event, err := feedEvent(id, at)
	if err != nil {
		return err
	}
	return f.publisher.publish(ctx, event)
}
