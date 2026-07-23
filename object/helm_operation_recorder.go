package object

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	helmOperationLogBatchSize     = 50
	helmOperationLogFlushInterval = 200 * time.Millisecond
)

type HelmOperationRecorder struct {
	taskID int64
	queue  chan *HelmOperationLog
	done   chan struct{}

	mu        sync.Mutex
	closed    bool
	batchErr  error
	writers   sync.WaitGroup
	finish    sync.Once
	finishErr error
}

func NewHelmOperationRecorder(taskID int64) *HelmOperationRecorder {
	recorder := &HelmOperationRecorder{
		taskID: taskID,
		queue:  make(chan *HelmOperationLog, helmOperationLogBatchSize*2),
		done:   make(chan struct{}),
	}
	go recorder.run()
	return recorder
}

func (r *HelmOperationRecorder) StartLoading() error {
	return StartHelmOperationTask(r.taskID, HelmOperationPhaseLoading)
}

func (r *HelmOperationRecorder) MarkInstalling() error {
	return UpdateHelmOperationTaskPhase(r.taskID, HelmOperationPhaseInstalling)
}

func (r *HelmOperationRecorder) RecordLog(line string) error {
	level := HelmOperationLogLevelInfo
	if strings.HasPrefix(line, "ERROR:") {
		level = HelmOperationLogLevelError
	}
	entry := &HelmOperationLog{
		TaskId:    r.taskID,
		Level:     level,
		Message:   line,
		CreatedAt: time.Now().UTC(),
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return fmt.Errorf("Helm operation recorder is closed")
	}
	r.writers.Add(1)
	r.mu.Unlock()
	defer r.writers.Done()
	r.queue <- entry
	return nil
}

func (r *HelmOperationRecorder) Finish(installErr error) error {
	r.finish.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()

		r.writers.Wait()
		close(r.queue)
		<-r.done

		success := installErr == nil
		errorMsg := ""
		if installErr != nil {
			errorMsg = installErr.Error()
		}
		finishErr := FinishHelmOperationTask(r.taskID, success, errorMsg)
		r.mu.Lock()
		batchErr := r.batchErr
		r.mu.Unlock()
		if batchErr != nil {
			if finishErr != nil {
				r.finishErr = fmt.Errorf("persist Helm operation logs: %v; finish task: %w", batchErr, finishErr)
				return
			}
			r.finishErr = fmt.Errorf("persist Helm operation logs: %w", batchErr)
			return
		}
		r.finishErr = finishErr
	})
	return r.finishErr
}

func (r *HelmOperationRecorder) run() {
	defer close(r.done)
	ticker := time.NewTicker(helmOperationLogFlushInterval)
	defer ticker.Stop()
	batch := make([]*HelmOperationLog, 0, helmOperationLogBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := addHelmOperationLogs(r.taskID, batch); err != nil {
			r.mu.Lock()
			if r.batchErr == nil {
				r.batchErr = err
			}
			r.mu.Unlock()
		}
		batch = make([]*HelmOperationLog, 0, helmOperationLogBatchSize)
	}

	for {
		select {
		case entry, ok := <-r.queue:
			if !ok {
				flush()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= helmOperationLogBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
