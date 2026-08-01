package service

import (
	"context"
	"log"
)

const (
	backgroundWorkerCount = 8
	backgroundQueueSize   = 256
)

type backgroundTask struct {
	ctx context.Context
	run func(context.Context)
}

var interactionBackgroundTasks = newBackgroundTaskQueue(backgroundWorkerCount, backgroundQueueSize)

func newBackgroundTaskQueue(workerCount, queueSize int) chan backgroundTask {
	queue := make(chan backgroundTask, queueSize)
	for i := 0; i < workerCount; i++ {
		go func() {
			for task := range queue {
				task.run(task.ctx)
			}
		}()
	}
	return queue
}

// submitInteractionBackgroundTask bounds view and interaction work without losing statistics.
// Saturation applies backpressure by executing on the caller instead of spawning or dropping work.
func submitInteractionBackgroundTask(ctx context.Context, run func(context.Context)) {
	task := backgroundTask{ctx: context.WithoutCancel(ctx), run: run}
	select {
	case interactionBackgroundTasks <- task:
	default:
		log.Printf("[background-task] interaction queue full; applying synchronous backpressure")
		task.run(task.ctx)
	}
}
