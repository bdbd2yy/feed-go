package worker

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"feedsystem_video_go/internal/middleware/rabbitmq"
)

// Fanout 独立 channel/queue：慢粉丝分发不再阻塞 Timeline 消费。
func StartFanoutConsumer(rmq *rabbitmq.RabbitMQ, w *FanoutWorker) {
	if rmq == nil || rmq.Conn == nil || w == nil || w.cache == nil {
		log.Print("Fanout consumer disabled")
		return
	}
	go func() {
		for {
			ch, err := rmq.NewChannel()
			if err != nil {
				log.Printf("Fanout channel: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
			if err = ch.Qos(10, 0, false); err != nil {
				_ = ch.Close()
				time.Sleep(5 * time.Second)
				continue
			}
			msgs, err := ch.Consume(rabbitmq.FanoutQueue, "", false, false, false, false, nil)
			if err != nil {
				_ = ch.Close()
				log.Printf("Fanout consume: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
			for msg := range msgs {
				var event rabbitmq.TimelineEvent
				if err := json.Unmarshal(msg.Body, &event); err != nil || event.VideoID == 0 {
					_ = msg.Reject(false)
					continue
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				err := w.ProcessVideo(ctx, event.VideoID)
				cancel()
				if err != nil {
					log.Printf("Fanout failed: video=%d err=%v", event.VideoID, err)
					_ = msg.Nack(false, true)
				} else {
					_ = msg.Ack(false)
				}
			}
			_ = ch.Close()
			time.Sleep(5 * time.Second)
		}
	}()
}
