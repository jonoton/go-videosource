package videosource

import (
	"fmt"
	"sync"
	"time"

	pubsubmutex "github.com/jonoton/go-pubsubmutex"
)

const topicVideoStatsAddAccepted = "topic-video-stats-add-accepted"
const topicVideoStatsAddDropped = "topic-video-stats-add-dropped"
const topicVideoStatsCurrentStats = "topic-video-stats-current-stats"
const topicVideoStatsGetCurrentStats = "topic-video-stats-get-current-stats"

type FrameStats struct {
	AcceptedTotal     int
	AcceptedPerSecond int
	DroppedTotal      int
	DroppedPerSecond  int
}

func (f FrameStats) String() string {
	total := f.AcceptedTotal + f.DroppedTotal
	return fmt.Sprintf("Accepted: [%3d/%-3d] %3d fps    Dropped: [%3d/%-3d] %3d fps",
		f.AcceptedTotal, total, f.AcceptedPerSecond, f.DroppedTotal, total, f.DroppedPerSecond)
}

type FrameStatsCombo struct {
	In  FrameStats
	Out FrameStats
}

// VideoStats contains video statistics
type VideoStats struct {
	acceptedTotal     int
	acceptedPerSecond int
	droppedTotal      int
	droppedPerSecond  int
	acceptedTmp       int
	droppedTmp        int
	pubsub            pubsubmutex.PubSub
	cancel            chan bool
	cancelOnce        sync.Once
}

// NewVideoStats creates a new VideoStats
func NewVideoStats() *VideoStats {
	v := &VideoStats{
		acceptedTotal:     0,
		acceptedPerSecond: 0,
		droppedTotal:      0,
		droppedPerSecond:  0,
		acceptedTmp:       0,
		droppedTmp:        0,
		pubsub:            *pubsubmutex.NewPubSub(),
		cancel:            make(chan bool),
	}
	return v
}

func (v *VideoStats) Start() {
	go func() {
		defer v.pubsub.Close()
		defer v.clearPerSecond()
		tick := time.NewTicker(1 * time.Second)
		defer tick.Stop()
		addAcceptedSub := v.pubsub.Subscribe(topicVideoStatsAddAccepted, v.pubsub.GetUniqueSubscriberID(), 10)
		defer addAcceptedSub.Unsubscribe()
		addDroppedSub := v.pubsub.Subscribe(topicVideoStatsAddDropped, v.pubsub.GetUniqueSubscriberID(), 10)
		defer addDroppedSub.Unsubscribe()
		getFrameStatsSub := v.pubsub.Subscribe(topicVideoStatsGetCurrentStats, v.pubsub.GetUniqueSubscriberID(), 10)
		defer getFrameStatsSub.Unsubscribe()
	Loop:
		for {
			select {
			case <-tick.C:
				v.tick()
			case <-v.cancel:
				break Loop
			case _, ok := <-getFrameStatsSub.Ch:
				if !ok {
					continue
				}
				v.publishCurrentStats()
			case _, ok := <-addAcceptedSub.Ch:
				if !ok {
					continue
				}
				v.addAccepted()
			case _, ok := <-addDroppedSub.Ch:
				if !ok {
					continue
				}
				v.addDropped()
			}
		}
	}()
}

func (v *VideoStats) Close() {
	v.cancelOnce.Do(func() {
		close(v.cancel)
	})
}

func (v *VideoStats) tick() {
	v.acceptedPerSecond = v.acceptedTmp
	v.acceptedTmp = 0
	v.droppedPerSecond = v.droppedTmp
	v.droppedTmp = 0
	v.publishCurrentStats()
}

// AddAccepted adds an accepted image
func (v *VideoStats) AddAccepted() {
	v.pubsub.Publish(pubsubmutex.Message{Topic: topicVideoStatsAddAccepted, Data: nil})
}

func (v *VideoStats) addAccepted() {
	v.acceptedTotal++
	v.acceptedTmp++
}

// AddDropped adds a dropped image
func (v *VideoStats) AddDropped() {
	v.pubsub.Publish(pubsubmutex.Message{Topic: topicVideoStatsAddDropped, Data: nil})
}

func (v *VideoStats) addDropped() {
	v.droppedTotal++
	v.droppedTmp++
}

// GetStats returns the FrameStats using pubsub
func (v *VideoStats) GetStats(timeoutMs int) (result *FrameStats) {
	r := v.pubsub.SendReceive(topicVideoStatsGetCurrentStats, topicVideoStatsCurrentStats,
		nil, timeoutMs)
	if r != nil {
		result = r.(*FrameStats)
	}
	return
}

// GetFrameStats returns the FrameStats directly
func (v *VideoStats) GetFrameStats() *FrameStats {
	f := &FrameStats{
		AcceptedTotal:     v.acceptedTotal,
		AcceptedPerSecond: v.acceptedPerSecond,
		DroppedTotal:      v.droppedTotal,
		DroppedPerSecond:  v.droppedPerSecond,
	}
	return f
}

func (v *VideoStats) clearPerSecond() {
	v.acceptedPerSecond = 0
	v.droppedPerSecond = 0
}

func (v *VideoStats) publishCurrentStats() {
	v.pubsub.Publish(pubsubmutex.Message{Topic: topicVideoStatsCurrentStats, Data: v.GetFrameStats()})
}

// GetStatsSub returns the subscriber
func (v *VideoStats) GetStatsSub() (result *pubsubmutex.Subscriber) {
	result = v.pubsub.Subscribe(topicVideoStatsCurrentStats, v.pubsub.GetUniqueSubscriberID(), 10)
	return
}
