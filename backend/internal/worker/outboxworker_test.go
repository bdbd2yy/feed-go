package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"feedsystem_video_go/internal/video"
)

type fakeVideoPublisher struct {
	calls int
	err   error
}

func (p *fakeVideoPublisher) PublishVideo(context.Context, uint, time.Time) error {
	p.calls++
	return p.err
}
func TestOutboxDualPublishRetriesBothAfterPartialSuccess(t *testing.T) {
	timeline := &fakeVideoPublisher{}
	fanout := &fakeVideoPublisher{err: errors.New("broker unavailable")}
	msg := video.OutboxMsg{VideoID: 7, CreateTime: time.Now()}
	if err := publishOutbox(context.Background(), msg, timeline, fanout); err == nil {
		t.Fatal("partial success must retain outbox")
	}
	fanout.err = nil
	if err := publishOutbox(context.Background(), msg, timeline, fanout); err != nil {
		t.Fatal(err)
	}
	if timeline.calls != 2 || fanout.calls != 2 {
		t.Fatal("both destinations must tolerate replay")
	}
	timeline.err = errors.New("timeline failed")
	if err := publishOutbox(context.Background(), msg, timeline, fanout); err == nil {
		t.Fatal("first destination failure ignored")
	}
	if fanout.calls != 2 {
		t.Fatal("fanout should not be attempted after timeline failure")
	}
}
