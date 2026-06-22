// Copyright 2023 The casbin Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package object

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"xorm.io/xorm"
)

const (
	ManagedNodeStatePending   = "pending"
	ManagedNodeStateDeploying = "deploying"
	ManagedNodeStateReady     = "ready"
	ManagedNodeStateDegraded  = "degraded"
	ManagedNodeStateRepairing = "repairing"
	ManagedNodeStateFailed    = "failed"
	ManagedNodeStateRemoving  = "removing"
	ManagedNodeStateRemoved   = "removed"

	NodeTaskTypeInstall = "install"
	NodeTaskTypeRepair  = "repair"
	NodeTaskTypeRemove  = "remove"

	NodeTaskStatusPending   = "pending"
	NodeTaskStatusRunning   = "running"
	NodeTaskStatusSucceeded = "succeeded"
	NodeTaskStatusFailed    = "failed"
	NodeTaskStatusCanceled  = "canceled"
)

type ManagedNode struct {
	Id                int64     `xorm:"pk autoincr" json:"id"`
	Name              string    `xorm:"varchar(100) notnull unique" json:"name"`
	Host              string    `xorm:"varchar(255) notnull" json:"host"`
	Port              int       `xorm:"notnull default 22" json:"port"`
	Username          string    `xorm:"varchar(64) notnull" json:"username"`
	EncryptedPassword string    `xorm:"text 'encrypted_password'" json:"-"`
	PrivateKey        string    `xorm:"text 'private_key'" json:"-"`
	PublicKey         string    `xorm:"text 'public_key'" json:"-"`
	State             string    `xorm:"varchar(32) notnull" json:"state"`
	KubernetesStatus  string    `xorm:"varchar(32) 'kubernetes_status'" json:"kubernetesStatus"`
	Labels            string    `xorm:"text" json:"-"`
	Unschedulable     bool      `xorm:"notnull default false" json:"unschedulable"`
	OS                string    `xorm:"varchar(64)" json:"os"`
	Arch              string    `xorm:"varchar(64)" json:"arch"`
	Version           string    `xorm:"varchar(64)" json:"version"`
	LastError         string    `xorm:"text 'last_error'" json:"lastError"`
	LastSeenAt        time.Time `json:"lastSeenAt"`
	LastDeployAt      time.Time `json:"lastDeployAt"`
	CreatedAt         time.Time `xorm:"created" json:"createdAt"`
	UpdatedAt         time.Time `xorm:"updated" json:"updatedAt"`
}

type NodeDeployTask struct {
	Id         int64     `xorm:"pk autoincr" json:"id"`
	NodeId     int64     `xorm:"index 'node_id'" json:"nodeId"`
	Type       string    `xorm:"varchar(32) notnull" json:"type"`
	Status     string    `xorm:"varchar(32) notnull" json:"status"`
	Stage      string    `xorm:"varchar(64)" json:"stage"`
	Progress   int       `xorm:"notnull default 0" json:"progress"`
	ErrorMsg   string    `xorm:"text 'error_msg'" json:"errorMsg"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	CreatedAt  time.Time `xorm:"created" json:"createdAt"`
	UpdatedAt  time.Time `xorm:"updated" json:"updatedAt"`
}

type NodeDeployLog struct {
	Id        int64     `xorm:"pk autoincr" json:"id"`
	TaskId    int64     `xorm:"index 'task_id'" json:"taskId"`
	Level     string    `xorm:"varchar(16) notnull" json:"level"`
	Message   string    `xorm:"text" json:"message"`
	CreatedAt time.Time `xorm:"created" json:"createdAt"`
}

func (n *ManagedNode) GetLabelMap() map[string]string {
	labels := map[string]string{}
	if n == nil || n.Labels == "" {
		return labels
	}
	_ = json.Unmarshal([]byte(n.Labels), &labels)
	return labels
}

func (n *ManagedNode) SetLabelMap(labels map[string]string) error {
	if len(labels) == 0 {
		n.Labels = ""
		return nil
	}
	data, err := json.Marshal(labels)
	if err != nil {
		return err
	}
	n.Labels = string(data)
	return nil
}

func GetManagedNodes() ([]*ManagedNode, error) {
	nodes := []*ManagedNode{}
	err := ormer.Engine.Asc("id").Find(&nodes)
	return nodes, err
}

func GetManagedNode(id int64) (*ManagedNode, error) {
	node := &ManagedNode{}
	existed, err := ormer.Engine.ID(id).Get(node)
	if err != nil {
		return nil, err
	}
	if !existed {
		return nil, nil
	}
	return node, nil
}

func GetManagedNodeByName(name string) (*ManagedNode, error) {
	node := &ManagedNode{Name: name}
	existed, err := ormer.Engine.Get(node)
	if err != nil {
		return nil, err
	}
	if !existed {
		return nil, nil
	}
	return node, nil
}

func AddManagedNode(node *ManagedNode) (bool, error) {
	affected, err := ormer.Engine.Insert(node)
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

func UpdateManagedNode(node *ManagedNode) (bool, error) {
	affected, err := ormer.Engine.ID(node.Id).AllCols().Update(node)
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

func DeleteManagedNode(id int64) error {
	session := ormer.Engine.NewSession()
	defer session.Close()
	if err := session.Begin(); err != nil {
		return err
	}
	tasks := []*NodeDeployTask{}
	if err := session.Where("node_id = ?", id).Find(&tasks); err != nil {
		_ = session.Rollback()
		return err
	}
	taskIds := make([]int64, 0, len(tasks))
	for _, task := range tasks {
		taskIds = append(taskIds, task.Id)
	}
	if len(taskIds) > 0 {
		if _, err := session.In("task_id", taskIds).Delete(&NodeDeployLog{}); err != nil {
			_ = session.Rollback()
			return err
		}
	}
	if _, err := session.Where("node_id = ?", id).Delete(&NodeDeployTask{}); err != nil {
		_ = session.Rollback()
		return err
	}
	if _, err := session.ID(id).Delete(&ManagedNode{}); err != nil {
		_ = session.Rollback()
		return err
	}
	return session.Commit()
}

func AddNodeDeployTask(task *NodeDeployTask) (bool, error) {
	affected, err := ormer.Engine.Insert(task)
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

func GetNodeDeployTask(id int64) (*NodeDeployTask, error) {
	task := &NodeDeployTask{}
	existed, err := ormer.Engine.ID(id).Get(task)
	if err != nil {
		return nil, err
	}
	if !existed {
		return nil, nil
	}
	return task, nil
}

func GetLatestNodeDeployTask(nodeId int64) (*NodeDeployTask, error) {
	task := &NodeDeployTask{}
	existed, err := ormer.Engine.Where("node_id = ?", nodeId).Desc("id").Get(task)
	if err != nil {
		return nil, err
	}
	if !existed {
		return nil, nil
	}
	return task, nil
}

func GetNodeDeployTasks(nodeId int64, limit int) ([]*NodeDeployTask, error) {
	tasks := []*NodeDeployTask{}
	session := ormer.Engine.Where("node_id = ?", nodeId).Desc("id")
	if limit > 0 {
		session = session.Limit(limit)
	}
	err := session.Find(&tasks)
	return tasks, err
}

func GetPendingNodeDeployTasks(limit int) ([]*NodeDeployTask, error) {
	tasks := []*NodeDeployTask{}
	session := ormer.Engine.Where("status = ?", NodeTaskStatusPending).Asc("id")
	if limit > 0 {
		session = session.Limit(limit)
	}
	err := session.Find(&tasks)
	return tasks, err
}

func ClaimNodeDeployTask(id int64) (*NodeDeployTask, error) {
	task := &NodeDeployTask{}
	startedAt := time.Now()
	affected, err := ormer.Engine.ID(id).
		And("status = ?", NodeTaskStatusPending).
		Cols("status", "started_at").
		Update(&NodeDeployTask{
			Status:    NodeTaskStatusRunning,
			StartedAt: startedAt,
		})
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, nil
	}
	existed, err := ormer.Engine.ID(id).Get(task)
	if err != nil {
		return nil, err
	}
	if !existed {
		return nil, nil
	}
	return task, nil
}

func HasActiveNodeDeployTask(nodeId int64) (bool, error) {
	task := &NodeDeployTask{}
	return ormer.Engine.Where("node_id = ?", nodeId).
		In("status", NodeTaskStatusPending, NodeTaskStatusRunning).
		Desc("id").
		Get(task)
}

func FailStaleRunningNodeDeployTasks(staleBefore time.Time, reason string) (int64, error) {
	if strings.TrimSpace(reason) == "" {
		reason = "managed node task marked failed because the worker process was interrupted"
	}

	tasks := []*NodeDeployTask{}
	if err := ormer.Engine.
		Where("status = ?", NodeTaskStatusRunning).
		And("updated_at < ?", staleBefore).
		Find(&tasks); err != nil {
		return 0, err
	}

	var affected int64
	for _, task := range tasks {
		task.Status = NodeTaskStatusFailed
		task.ErrorMsg = reason
		task.FinishedAt = time.Now()
		if _, err := UpdateNodeDeployTask(task); err != nil {
			return affected, err
		}
		_, _ = AddNodeDeployLog(&NodeDeployLog{
			TaskId:    task.Id,
			Level:     "warning",
			Message:   reason,
			CreatedAt: time.Now(),
		})
		if node, err := GetManagedNode(task.NodeId); err == nil && node != nil {
			node.State = ManagedNodeStateFailed
			node.LastError = fmt.Sprintf("task #%d %s", task.Id, reason)
			_, _ = UpdateManagedNode(node)
		}
		affected++
	}

	return affected, nil
}

func UpdateNodeDeployTask(task *NodeDeployTask) (bool, error) {
	affected, err := ormer.Engine.ID(task.Id).AllCols().Update(task)
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

func AddNodeDeployLog(log *NodeDeployLog) (bool, error) {
	affected, err := ormer.Engine.Insert(log)
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

func GetNodeDeployLogs(taskId int64, limit int) ([]*NodeDeployLog, error) {
	logs := []*NodeDeployLog{}
	session := ormer.Engine.Where("task_id = ?", taskId).Asc("id")
	if limit > 0 {
		session = session.Limit(limit)
	}
	err := session.Find(&logs)
	return logs, err
}

func WithSession(fn func(session *xorm.Session) error) error {
	session := ormer.Engine.NewSession()
	defer session.Close()
	return fn(session)
}
