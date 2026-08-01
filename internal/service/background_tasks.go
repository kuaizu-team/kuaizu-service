package service

import (
	"context"
	"log"
	"time"
)

const (
	backgroundWorkerCount = 8
	backgroundQueueSize   = 256
	backgroundTaskTimeout = 15 * time.Second
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
				taskCtx, cancel := context.WithTimeout(task.ctx, backgroundTaskTimeout)
				task.run(taskCtx)
				cancel()
			}
		}()
	}
	return queue
}

// submitInteractionBackgroundTask bounds best-effort view and interaction work.
// A full queue drops only the notification/log side effect, never the user request.
func submitInteractionBackgroundTask(ctx context.Context, run func(context.Context)) {
	task := backgroundTask{ctx: context.WithoutCancel(ctx), run: run}
	select {
	case interactionBackgroundTasks <- task:
	default:
		log.Printf("[background-task] interaction queue full; dropping best-effort task")
	}
}
