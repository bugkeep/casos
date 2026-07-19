package object

import (
	"testing"
	"time"

	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestMachineNodeDeployActiveKeyEnforcesOneActiveTask(t *testing.T) {
	previousOrmer := ormer
	engine, err := xorm.NewEngine("sqlite", "file:machine-node-deploy-active-key-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create test engine: %v", err)
	}
	engine.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = engine.Close()
		ormer = previousOrmer
	})
	ormer = &Ormer{Engine: engine}

	if err := engine.Sync2(new(Machine), new(MachineNodeDeployTask)); err != nil {
		t.Fatalf("create task tables: %v", err)
	}
	if _, err := engine.Insert(&Machine{Owner: "admin", Name: "worker-1"}); err != nil {
		t.Fatalf("create machine: %v", err)
	}

	first, err := CreateMachineNodeDeployTask("admin", "worker-1", "worker-1", "https://127.0.0.1:6443")
	if err != nil {
		t.Fatalf("create first task: %v", err)
	}
	if first.ActiveKey == nil {
		t.Fatal("expected active task to have an arbitration key")
	}
	duplicate := &MachineNodeDeployTask{
		ActiveKey:   first.ActiveKey,
		Owner:       "admin",
		MachineName: "worker-1",
		NodeName:    "worker-1",
		Status:      MachineNodeDeployStatusPending,
		Phase:       MachineNodeDeployPhaseQueued,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if _, err := engine.Insert(duplicate); err == nil {
		t.Fatal("expected the database to reject a duplicate active key")
	}

	if err := FinishMachineNodeDeployTask(first.Id, false, MachineNodeDeployPhaseFailed, "failed"); err != nil {
		t.Fatalf("finish first task: %v", err)
	}
	stored, err := GetMachineNodeDeployTask(first.Id)
	if err != nil {
		t.Fatalf("read finished task: %v", err)
	}
	if stored.ActiveKey != nil {
		t.Fatalf("expected a finished task to release its active key, got %q", *stored.ActiveKey)
	}
	if _, err := CreateMachineNodeDeployTask("admin", "worker-1", "worker-1", "https://127.0.0.1:6443"); err != nil {
		t.Fatalf("create replacement task: %v", err)
	}
}
