package notify

import (
	"k8s.io/client-go/util/workqueue"
	"time"
)

var eventQueue workqueue.RateLimitingInterface

type Trigger string

const (
	Dummy                 Trigger = "dummy"
	ConfigModify          Trigger = "configModify"
	LVMCheck              Trigger = "lvmCheck"
	CleanupOrphan         Trigger = "cleanupOrphan"
	LogicVolumeController Trigger = "logicVolumeController"
)

type VolumeEvents struct {
	Trigger   Trigger
	TriggerAt time.Time
}

func init() {
	eventQueue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
}

func GetQueue() workqueue.RateLimitingInterface {
	return eventQueue
}

func SendEvent(trigger Trigger) {
	eventQueue.Add(&VolumeEvents{
		Trigger:   trigger,
		TriggerAt: time.Now(),
	})
}
