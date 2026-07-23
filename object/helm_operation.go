package object

import (
	"fmt"
	"strings"
	"time"

	"xorm.io/xorm"
)

const (
	HelmOperationInstall = "install"

	HelmOperationStatusPending   = "pending"
	HelmOperationStatusRunning   = "running"
	HelmOperationStatusSucceeded = "succeeded"
	HelmOperationStatusFailed    = "failed"

	HelmOperationPhaseQueued     = "queued"
	HelmOperationPhaseLoading    = "loading"
	HelmOperationPhaseInstalling = "installing"
	HelmOperationPhaseReady      = "ready"
	HelmOperationPhaseFailed     = "failed"

	HelmOperationLogLevelInfo  = "info"
	HelmOperationLogLevelError = "error"

	helmOperationStaleAfter = 11 * time.Minute
)

type HelmOperationTask struct {
	Id          int64     `xorm:"pk autoincr" json:"id"`
	Owner       string    `xorm:"varchar(100) notnull index" json:"owner"`
	Operation   string    `xorm:"varchar(30) notnull" json:"operation"`
	ReleaseName string    `xorm:"varchar(253) notnull index" json:"releaseName"`
	Namespace   string    `xorm:"varchar(253) notnull index" json:"namespace"`
	ChartName   string    `xorm:"varchar(253) notnull" json:"chartName"`
	Version     string    `xorm:"varchar(100)" json:"version"`
	Status      string    `xorm:"varchar(30) notnull index" json:"status"`
	Phase       string    `xorm:"varchar(30) notnull" json:"phase"`
	ErrorMsg    string    `xorm:"text" json:"errorMsg"`
	CreatedAt   time.Time `json:"createdAt"`
	StartedAt   time.Time `json:"startedAt"`
	FinishedAt  time.Time `json:"finishedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type HelmOperationLog struct {
	Id        int64     `xorm:"pk autoincr" json:"id"`
	TaskId    int64     `xorm:"notnull index" json:"taskId"`
	Level     string    `xorm:"varchar(20) notnull" json:"level"`
	Message   string    `xorm:"text" json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

func CreateHelmOperationTask(owner, operation, releaseName, namespace, chartName, version string) (*HelmOperationTask, error) {
	owner = strings.TrimSpace(owner)
	operation = strings.TrimSpace(operation)
	releaseName = strings.TrimSpace(releaseName)
	namespace = strings.TrimSpace(namespace)
	chartName = strings.TrimSpace(chartName)
	if owner == "" || operation == "" || releaseName == "" || namespace == "" || chartName == "" {
		return nil, fmt.Errorf("owner, operation, releaseName, namespace, and chartName are required")
	}
	if operation != HelmOperationInstall {
		return nil, fmt.Errorf("unsupported Helm operation: %s", operation)
	}

	result, err := withHelmOperationTransaction(func(session *xorm.Session) (interface{}, error) {
		active := &HelmOperationTask{}
		found, err := session.
			Where("namespace = ? AND release_name = ? AND status IN (?, ?)", namespace, releaseName, HelmOperationStatusPending, HelmOperationStatusRunning).
			Desc("id").
			ForUpdate().
			Get(active)
		if err != nil {
			return nil, err
		}
		if found {
			if active.UpdatedAt.After(time.Now().UTC().Add(-helmOperationStaleAfter)) {
				return nil, fmt.Errorf("Helm operation task %d is already active for %s/%s", active.Id, namespace, releaseName)
			}
			now := time.Now().UTC()
			if _, err := session.ID(active.Id).
				Cols("status", "phase", "error_msg", "finished_at", "updated_at").
				Update(&HelmOperationTask{
					Status:     HelmOperationStatusFailed,
					Phase:      HelmOperationPhaseFailed,
					ErrorMsg:   "Helm operation expired before completion",
					FinishedAt: now,
					UpdatedAt:  now,
				}); err != nil {
				return nil, err
			}
		}

		now := time.Now().UTC()
		task := &HelmOperationTask{
			Owner:       owner,
			Operation:   operation,
			ReleaseName: releaseName,
			Namespace:   namespace,
			ChartName:   chartName,
			Version:     strings.TrimSpace(version),
			Status:      HelmOperationStatusPending,
			Phase:       HelmOperationPhaseQueued,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if _, err := session.Insert(task); err != nil {
			return nil, err
		}
		return task, nil
	})
	if err != nil {
		return nil, err
	}
	task, ok := result.(*HelmOperationTask)
	if !ok || task == nil {
		return nil, fmt.Errorf("create Helm operation task returned invalid result")
	}
	return task, nil
}

func GetHelmOperationTask(id int64) (*HelmOperationTask, error) {
	task := &HelmOperationTask{Id: id}
	found, err := ormer.Engine.Get(task)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return task, nil
}

func ExpireStaleHelmOperationTask(id int64) error {
	now := time.Now().UTC()
	_, err := ormer.Engine.ID(id).
		Where("status IN (?, ?) AND updated_at < ?", HelmOperationStatusPending, HelmOperationStatusRunning, now.Add(-helmOperationStaleAfter)).
		Cols("status", "phase", "error_msg", "finished_at", "updated_at").
		Update(&HelmOperationTask{
			Status:     HelmOperationStatusFailed,
			Phase:      HelmOperationPhaseFailed,
			ErrorMsg:   "Helm operation expired before completion",
			FinishedAt: now,
			UpdatedAt:  now,
		})
	return err
}

func StartHelmOperationTask(id int64, phase string) error {
	if phase != HelmOperationPhaseLoading {
		return fmt.Errorf("invalid Helm operation start phase: %s", phase)
	}
	now := time.Now().UTC()
	affected, err := ormer.Engine.ID(id).
		Where("status = ? AND phase = ?", HelmOperationStatusPending, HelmOperationPhaseQueued).
		Cols("status", "phase", "started_at", "updated_at").
		Update(&HelmOperationTask{Status: HelmOperationStatusRunning, Phase: phase, StartedAt: now, UpdatedAt: now})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("Helm operation task %d is not pending", id)
	}
	return nil
}

func UpdateHelmOperationTaskPhase(id int64, phase string) error {
	if phase != HelmOperationPhaseInstalling {
		return fmt.Errorf("invalid Helm operation phase: %s", phase)
	}
	affected, err := ormer.Engine.ID(id).
		Where("status = ? AND phase = ?", HelmOperationStatusRunning, HelmOperationPhaseLoading).
		Cols("phase", "updated_at").
		Update(&HelmOperationTask{Phase: phase, UpdatedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("Helm operation task %d is not running", id)
	}
	return nil
}

func FinishHelmOperationTask(id int64, success bool, errorMsg string) error {
	status := HelmOperationStatusSucceeded
	phase := HelmOperationPhaseReady
	if success {
		errorMsg = ""
	}
	if !success {
		status = HelmOperationStatusFailed
		phase = HelmOperationPhaseFailed
	}
	now := time.Now().UTC()
	affected, err := ormer.Engine.ID(id).
		Where("status IN (?, ?)", HelmOperationStatusPending, HelmOperationStatusRunning).
		Cols("status", "phase", "error_msg", "finished_at", "updated_at").
		Update(&HelmOperationTask{Status: status, Phase: phase, ErrorMsg: errorMsg, FinishedAt: now, UpdatedAt: now})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("Helm operation task %d is already finished", id)
	}
	return nil
}

func addHelmOperationLogs(taskID int64, logs []*HelmOperationLog) error {
	if len(logs) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for _, entry := range logs {
		if entry == nil || entry.TaskId != taskID {
			return fmt.Errorf("Helm operation log task id does not match task %d", taskID)
		}
		if entry.CreatedAt.IsZero() {
			entry.CreatedAt = now
		}
	}
	_, err := withHelmOperationTransaction(func(session *xorm.Session) (interface{}, error) {
		if _, err := session.Insert(&logs); err != nil {
			return nil, err
		}
		_, err := session.ID(taskID).
			Where("status IN (?, ?)", HelmOperationStatusPending, HelmOperationStatusRunning).
			Cols("updated_at").
			Update(&HelmOperationTask{UpdatedAt: now})
		return nil, err
	})
	return err
}

func GetHelmOperationLogs(taskID int64, limit int) ([]*HelmOperationLog, error) {
	if taskID <= 0 {
		return nil, fmt.Errorf("invalid taskId")
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 1000 {
		return nil, fmt.Errorf("limit must not exceed 1000")
	}
	logs := []*HelmOperationLog{}
	err := ormer.Engine.Where("task_id = ?", taskID).Asc("id").Limit(limit).Find(&logs)
	return logs, err
}

func isValidHelmOperationPhase(phase string) bool {
	switch phase {
	case HelmOperationPhaseQueued, HelmOperationPhaseLoading, HelmOperationPhaseInstalling, HelmOperationPhaseReady, HelmOperationPhaseFailed:
		return true
	default:
		return false
	}
}

func withHelmOperationTransaction(fn func(*xorm.Session) (interface{}, error)) (interface{}, error) {
	session := ormer.Engine.NewSession()
	defer func() {
		if v := recover(); v != nil {
			_ = session.Rollback()
			session.Close()
			panic(v)
		}
		session.Close()
	}()
	if err := session.Begin(); err != nil {
		return nil, err
	}
	result, err := fn(session)
	if err != nil {
		_ = session.Rollback()
		return nil, err
	}
	if err := session.Commit(); err != nil {
		_ = session.Rollback()
		return nil, err
	}
	return result, nil
}
