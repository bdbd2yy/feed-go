package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// feedPublisher 为每一路单独持有 channel，串行发送并等待 broker confirm。
// 发送结果不确定时关闭 channel，后续重建，避免把迟到的确认用于下一条消息。
type feedPublisher struct {
	mu                                sync.Mutex
	base                              *RabbitMQ
	exchange, queue, binding, routing string
	ch                                *amqp.Channel
	returns                           chan amqp.Return
}

func newFeedPublisher(base *RabbitMQ, exchange, queue, binding, routing string) (*feedPublisher, error) {
	p := &feedPublisher{base: base, exchange: exchange, queue: queue, binding: binding, routing: routing}
	if err := p.open(); err != nil {
		return nil, err
	}
	return p, nil
}
func (p *feedPublisher) open() error {
	ch, err := p.base.NewChannel()
	if err != nil {
		return err
	}
	if err := DeclareTopic(ch, p.exchange, p.queue, p.binding); err != nil {
		_ = ch.Close()
		return err
	}
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return err
	}
	p.ch = ch
	// 仅允许一个在途消息，容量 1 足以接收它的 basic.return，避免阻塞 AMQP 分发。
	p.returns = ch.NotifyReturn(make(chan amqp.Return, 1))
	return nil
}
func (p *feedPublisher) publish(ctx context.Context, event TimelineEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.ch == nil || p.ch.IsClosed() {
		if err := p.open(); err != nil {
			return err
		}
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	dc, err := p.ch.PublishWithDeferredConfirmWithContext(ctx, p.exchange, p.routing, true, false, amqp.Publishing{
		ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: event.EventID, Timestamp: time.Now(), Body: body,
	})
	if err == nil && dc == nil {
		err = errors.New("publisher confirms not enabled")
	}
	if err == nil {
		var ack bool
		ack, err = dc.WaitContext(ctx)
		if err == nil && !ack {
			err = errors.New("broker nacked feed event")
		}
	}
	// RabbitMQ 在不可路由确认之前发送 basic.return；当前固定版本在同一 AMQP
	// 分发循环中先投递到 buffered return listener，再处理 deferred confirmation。
	// 不用 select 同时竞争两个通知；确认后检查已经缓冲的 return。
	if err == nil {
		select {
		case returned, ok := <-p.returns:
			if !ok {
				err = errors.New("publisher channel closed")
			} else {
				err = fmt.Errorf("feed event unroutable: %d %s", returned.ReplyCode, returned.ReplyText)
			}
		default:
		}
	}
	if err != nil {
		_ = p.ch.Close()
		p.ch = nil
	}
	return err
}

func feedEvent(videoID uint, at time.Time) (TimelineEvent, error) {
	if videoID == 0 {
		return TimelineEvent{}, errors.New("video ID is required")
	}
	// 相同作品发布事件重试时保持标识稳定；消费状态仍需幂等。
	return TimelineEvent{EventID: fmt.Sprintf("video-published:%d", videoID), VideoID: videoID, CreateTime: at.UnixMilli(), OccurredAt: time.Now()}, nil
}
