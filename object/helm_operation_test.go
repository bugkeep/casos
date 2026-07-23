package object

import (
	"fmt"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestHelmOperationTaskStateMachine(t *testing.T) {
	previousOrmer := ormer
	engine, err := xorm.NewEngine("sqlite", "file:helm-operation-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create test engine: %v", err)
	}
	engine.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = engine.Close()
		ormer = previousOrmer
	})
	ormer = &Ormer{Engine: engine}

	if err := engine.Sync2(new(HelmOperationTask), new(HelmOperationLog)); err != nil {
		t.Fatalf("create task tables: %v", err)
	}
	if _, err := CreateHelmOperationTask("admin", "delete", "demo", "default", "demo", "1.0.0"); err == nil {
		t.Fatal("expected an unsupported operation to be rejected")
	}

	task, err := CreateHelmOperationTask("admin", HelmOperationInstall, "demo", "default", "demo", "1.0.0")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Status != HelmOperationStatusPending || task.Phase != HelmOperationPhaseQueued {
		t.Fatalf("unexpected initial state: %s/%s", task.Status, task.Phase)
	}
	if err := StartHelmOperationTask(task.Id, HelmOperationPhaseReady); err == nil {
		t.Fatal("expected a pending task to reject the terminal ready phase")
	}
	if err := StartHelmOperationTask(task.Id, HelmOperationPhaseLoading); err != nil {
		t.Fatalf("start task: %v", err)
	}
	if err := UpdateHelmOperationTaskPhase(task.Id, HelmOperationPhaseLoading); err == nil {
		t.Fatal("expected a running task to reject a repeated loading phase")
	}
	if err := UpdateHelmOperationTaskPhase(task.Id, HelmOperationPhaseReady); err == nil {
		t.Fatal("expected a running task to reject the terminal ready phase")
	}
	if err := UpdateHelmOperationTaskPhase(task.Id, HelmOperationPhaseInstalling); err != nil {
		t.Fatalf("advance task: %v", err)
	}
	if err := FinishHelmOperationTask(task.Id, true, ""); err != nil {
		t.Fatalf("finish task: %v", err)
	}

	stored, err := GetHelmOperationTask(task.Id)
	if err != nil {
		t.Fatalf("read task: %v", err)
	}
	if stored.Status != HelmOperationStatusSucceeded || stored.Phase != HelmOperationPhaseReady {
		t.Fatalf("unexpected terminal state: %s/%s", stored.Status, stored.Phase)
	}
	if err := FinishHelmOperationTask(task.Id, false, "should not overwrite completion"); err == nil {
		t.Fatal("expected a terminal task to reject a second finish")
	}
}

func TestExpireStaleHelmOperationTask(t *testing.T) {
	previousOrmer := ormer
	engine, err := xorm.NewEngine("sqlite", "file:helm-operation-expiry-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create test engine: %v", err)
	}
	engine.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = engine.Close()
		ormer = previousOrmer
	})
	ormer = &Ormer{Engine: engine}

	if err := engine.Sync2(new(HelmOperationTask), new(HelmOperationLog)); err != nil {
		t.Fatalf("create task tables: %v", err)
	}

	fresh, err := CreateHelmOperationTask("admin", HelmOperationInstall, "fresh", "default", "demo", "1.0.0")
	if err != nil {
		t.Fatalf("create fresh task: %v", err)
	}
	stale, err := CreateHelmOperationTask("admin", HelmOperationInstall, "stale", "default", "demo", "1.0.0")
	if err != nil {
		t.Fatalf("create stale task: %v", err)
	}
	old := time.Now().UTC().Add(-helmOperationStaleAfter - time.Minute)
	if _, err := engine.ID(stale.Id).Cols("updated_at").Update(&HelmOperationTask{UpdatedAt: old}); err != nil {
		t.Fatalf("age stale task: %v", err)
	}

	if err := ExpireStaleHelmOperationTask(fresh.Id); err != nil {
		t.Fatalf("check fresh task: %v", err)
	}
	if err := ExpireStaleHelmOperationTask(stale.Id); err != nil {
		t.Fatalf("expire stale task: %v", err)
	}
	fresh, _ = GetHelmOperationTask(fresh.Id)
	stale, _ = GetHelmOperationTask(stale.Id)
	if fresh.Status != HelmOperationStatusPending {
		t.Fatalf("fresh task was changed to %q", fresh.Status)
	}
	if stale.Status != HelmOperationStatusFailed || stale.Phase != HelmOperationPhaseFailed {
		t.Fatalf("stale task did not expire: %s/%s", stale.Status, stale.Phase)
	}
}

func TestHelmOperationRecorderFlushesLogsBeforeFinish(t *testing.T) {
	previousOrmer := ormer
	engine, err := xorm.NewEngine("sqlite", "file:helm-operation-recorder-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create test engine: %v", err)
	}
	engine.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = engine.Close()
		ormer = previousOrmer
	})
	ormer = &Ormer{Engine: engine}

	if err := engine.Sync2(new(HelmOperationTask), new(HelmOperationLog)); err != nil {
		t.Fatalf("create task tables: %v", err)
	}
	task, err := CreateHelmOperationTask("admin", HelmOperationInstall, "recorded", "default", "demo", "1.0.0")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	recorder := NewHelmOperationRecorder(task.Id)
	if err := recorder.StartLoading(); err != nil {
		t.Fatalf("start task: %v", err)
	}
	if err := addHelmOperationLogs(task.Id, []*HelmOperationLog{{TaskId: task.Id + 1, Message: "wrong task"}}); err == nil {
		t.Fatal("expected a mismatched log task id to be rejected")
	}
	for _, line := range []string{"loading", "installing", "ERROR: failed"} {
		if err := recorder.RecordLog(line); err != nil {
			t.Fatalf("record log %q: %v", line, err)
		}
	}
	installErr := fmt.Errorf("failed")
	if err := recorder.Finish(installErr); err != nil {
		t.Fatalf("finish recorder: %v", err)
	}
	if err := recorder.Finish(nil); err != nil {
		t.Fatalf("finish recorder a second time: %v", err)
	}
	if err := recorder.RecordLog("late log"); err == nil {
		t.Fatal("expected a finished recorder to reject new logs")
	}

	logs, err := GetHelmOperationLogs(task.Id, 10)
	if err != nil {
		t.Fatalf("read logs: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 persisted logs, got %d", len(logs))
	}
	stored, err := GetHelmOperationTask(task.Id)
	if err != nil {
		t.Fatalf("read task: %v", err)
	}
	if stored.Status != HelmOperationStatusFailed || stored.ErrorMsg != installErr.Error() {
		t.Fatalf("unexpected terminal task: %s/%q", stored.Status, stored.ErrorMsg)
	}
}
