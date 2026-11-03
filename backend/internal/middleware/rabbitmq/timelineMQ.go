package rabbitmq

import (
	"context"
	"errors"
	"time"
)

type TimelineMQ struct {
	publisher *feedPublisher
}

const (
	timelineExchange   = "video.timeline.events"
	timelineQueue      = "video.timeline.update.queue"
	timelineBindingKey = "video.timeline.*"
	timelinePublishRK  = "video.timeline.publish"
)

type TimelineEvent struct {
	EventID    string    `json:"event_id"`
	VideoID    uint      `json:"video_id"`
	CreateTime int64     `json:"create_time"`
	OccurredAt time.Time `json:"occurred_at"`
}

func NewTimelineMQ(base *RabbitMQ) (*TimelineMQ, error) {
	p, err := newFeedPublisher(base, timelineExchange, timelineQueue, timelineBindingKey, timelinePublishRK)
	if err != nil {
		return nil, err
	}
	return &TimelineMQ{publisher: p}, nil
}

func (t *TimelineMQ) PublishVideo(ctx context.Context, id uint, at time.Time) error {
	if t == nil || t.publisher == nil {
		return errors.New("timeline mq not initialized")
	}
	event, err := feedEvent(id, at)
	if err != nil {
		return err
	}
	return t.publisher.publish(ctx, event)
}
