package worker

import (
	"context"
	"encoding/json"
	"feedsystem_video_go/internal/middleware/rabbitmq"
	rediscache "feedsystem_video_go/internal/middleware/redis"
	"feedsystem_video_go/internal/video"
	"fmt"
	"log"
	"time"

	oredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type videoPublisher interface {
	PublishVideo(context.Context, uint, time.Time) error
}

// 两路均收到 broker confirm 后才允许删除 Outbox。失败时重投两路，而非跨队列事务。
func publishOutbox(ctx context.Context, msg video.OutboxMsg, timeline, fanout videoPublisher) error {
	if err := timeline.PublishVideo(ctx, msg.VideoID, msg.CreateTime); err != nil {
		return fmt.Errorf("timeline publish: %w", err)
	}
	if err := fanout.PublishVideo(ctx, msg.VideoID, msg.CreateTime); err != nil {
		return fmt.Errorf("fanout publish: %w", err)
	}
	return nil
}

func StartOutboxPoller(db *gorm.DB, tmq *rabbitmq.TimelineMQ, fmq *rabbitmq.FanoutMQ) {
	if db == nil || tmq == nil || fmq == nil {
		log.Printf("Outbox disabled: both feed publishers are required")
		return
	}
	go func() {
		for {
			var messages []video.OutboxMsg
			err := db.Where("status = ?", "pending").Order("create_time ASC").Limit(100).Find(&messages).Error
			if err != nil || len(messages) == 0 {
				time.Sleep(time.Second)
				continue
			}
			failed := false
			for _, msg := range messages {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := publishOutbox(ctx, msg, tmq, fmq)
				cancel()
				if err != nil {
					failed = true
					log.Printf("Outbox retained: video=%d err=%v", msg.VideoID, err)
					continue
				}
				if err := db.Delete(&msg).Error; err != nil {
					failed = true
					log.Printf("Outbox delete failed: id=%d err=%v", msg.ID, err)
				}
			}
			if failed {
				time.Sleep(time.Second)
			}
		}
	}()
}

func StartConsumer(tmq *rabbitmq.TimelineMQ, queueName string, redisClient *rediscache.Client, rmq *rabbitmq.RabbitMQ) {
	if tmq == nil || rmq == nil || rmq.Conn == nil {
		log.Printf("Timeline consumer disabled: rabbitmq is not initialized")
		return
	}
	if redisClient == nil {
		log.Printf("Timeline consumer disabled: redis is not initialized")
		return
	}

	go func() {
		for {
			// 每次重连创建独立的 Channel，不与发布者共用
			ch, err := rmq.NewChannel()
			if err != nil {
				log.Printf("Timeline consumer: 创建 Channel 失败: %v, 5秒后重试", err)
				time.Sleep(5 * time.Second)
				continue
			}

			if err := ch.Qos(10, 0, false); err != nil {
				log.Printf("Timeline consumer: QoS 设置失败: %v", err)
			}

			msgs, err := ch.Consume(queueName, "", false, false, false, false, nil)
			if err != nil {
				log.Printf("Timeline consumer: 注册消费失败: %v, 5秒后重试", err)
				ch.Close()
				time.Sleep(5 * time.Second)
				continue
			}

			log.Printf("Timeline consumer 已启动, queue=%s", queueName)

			for msg := range msgs {
				var event rabbitmq.TimelineEvent
				if err := json.Unmarshal(msg.Body, &event); err != nil {
					log.Printf("Timeline consumer: 反序列化失败: %v", err)
					msg.Ack(false)
					continue
				}

				if event.VideoID == 0 {
					log.Printf("Timeline consumer: discard event with zero video ID")
					msg.Ack(false)
					continue
				}

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				timelineKey := redisClient.Key("feed:global_timeline")
				err = redisClient.ZAdd(ctx, timelineKey, oredis.Z{
					Score:  float64(event.CreateTime),
					Member: fmt.Sprintf("%d", event.VideoID),
				})

				if err != nil {
					log.Printf("Timeline consumer: 写入Zset失败: %v", err)
					msg.Nack(false, true)
					cancel()
					continue
				}

				if err := redisClient.ZRemRangeByRank(ctx, timelineKey, 0, -1001); err != nil {
					log.Printf("Timeline consumer: ZRem失败: %v", err)
				}

				msg.Ack(false)
				cancel()
			}

			// msgs channel 关闭说明 AMQP Channel 断开，关闭并重连
			ch.Close()
			log.Printf("Timeline consumer: Channel 断开, 5秒后重连...")
			time.Sleep(5 * time.Second)
		}
	}()
}
