package controllers

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/server"
)

type managedNodeRequest struct {
	Id            int64             `json:"id"`
	Name          string            `json:"name"`
	Host          string            `json:"host"`
	Port          int               `json:"port"`
	Username      string            `json:"username"`
	Password      string            `json:"password"`
	Labels        map[string]string `json:"labels"`
	Unschedulable bool              `json:"unschedulable"`
}

type managedNodeSummary struct {
	Id               int64             `json:"id"`
	Name             string            `json:"name"`
	Host             string            `json:"host"`
	Port             int               `json:"port"`
	Username         string            `json:"username"`
	State            string            `json:"state"`
	KubernetesStatus string            `json:"kubernetesStatus"`
	Labels           map[string]string `json:"labels"`
	Unschedulable    bool              `json:"unschedulable"`
	OS               string            `json:"os"`
	Arch             string            `json:"arch"`
	Version          string            `json:"version"`
	LastError        string            `json:"lastError"`
	LastSeenAt       string            `json:"lastSeenAt"`
	LastDeployAt     string            `json:"lastDeployAt"`
	CreatedAt        string            `json:"createdAt"`
	UpdatedAt        string            `json:"updatedAt"`
}

type managedNodePreflightResult struct {
	Reachable    bool   `json:"reachable"`
	IsRoot       bool   `json:"isRoot"`
	SupportsOS   bool   `json:"supportsOs"`
	SupportsArch bool   `json:"supportsArch"`
	HasSystemd   bool   `json:"hasSystemd"`
	InitProcess  string `json:"initProcess"`
	SystemdState string `json:"systemdState"`
	IsWSL        bool   `json:"isWsl"`
	WindowsHost  string `json:"windowsHost"`
	OS           string `json:"os"`
	Version      string `json:"version"`
	Arch         string `json:"arch"`
	Message      string `json:"message"`
}

// AddManagedNode creates a managed node and queues automatic deployment.
// @router /api/add-managed-node [post]
func (c *ApiController) AddManagedNode() {
	if c.RequireAdmin() {
		return
	}

	var req managedNodeRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Host = strings.TrimSpace(req.Host)
	req.Username = strings.TrimSpace(req.Username)
	if req.Name == "" || req.Host == "" || req.Username == "" || req.Password == "" {
		c.ResponseError("name, host, username, and password are required")
		return
	}
	if req.Port == 0 {
		req.Port = 22
	}
	existing, err := object.GetManagedNodeByName(req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if existing != nil {
		c.ResponseError("managed node already exists")
		return
	}
	encryptedPassword, err := server.EncryptManagedNodePassword(req.Password)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	node := &object.ManagedNode{
		Name:              req.Name,
		Host:              req.Host,
		Port:              req.Port,
		Username:          req.Username,
		EncryptedPassword: encryptedPassword,
		State:             object.ManagedNodeStatePending,
		KubernetesStatus:  "Pending",
		Unschedulable:     req.Unschedulable,
	}
	if err := node.SetLabelMap(req.Labels); err != nil {
		c.ResponseError(err.Error())
		return
	}
	ok, err := object.AddManagedNode(node)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !ok {
		c.ResponseError("failed to create managed node")
		return
	}
	task := &object.NodeDeployTask{
		NodeId:   node.Id,
		Type:     object.NodeTaskTypeInstall,
		Status:   object.NodeTaskStatusPending,
		Stage:    "queued",
		Progress: 0,
	}
	ok, err = object.AddNodeDeployTask(task)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !ok {
		c.ResponseError("failed to create managed node task")
		return
	}
	_, _ = object.AddNodeDeployLog(&object.NodeDeployLog{
		TaskId:    task.Id,
		Level:     "info",
		Message:   "queued managed node deployment",
		CreatedAt: time.Now(),
	})
	c.ResponseOk(toManagedNodeSummary(node), task)
}

// PreflightManagedNode validates whether a host can be managed before deployment.
// @router /api/preflight-managed-node [post]
func (c *ApiController) PreflightManagedNode() {
	if c.RequireAdmin() {
		return
	}
	var req managedNodeRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	req.Host = strings.TrimSpace(req.Host)
	req.Username = strings.TrimSpace(req.Username)
	if req.Host == "" || req.Username == "" || req.Password == "" {
		c.ResponseError("host, username, and password are required")
		return
	}
	if req.Port == 0 {
		req.Port = 22
	}
	result, err := server.PreflightManagedNode(req.Host, req.Port, req.Username, req.Password)
	if err != nil {
		c.ResponseError(err.Error(), result)
		return
	}
	c.ResponseOk(result)
}

// GetManagedNodes lists all managed nodes.
// @router /api/get-managed-nodes [get]
func (c *ApiController) GetManagedNodes() {
	if c.RequireAdmin() {
		return
	}
	nodes, err := object.GetManagedNodes()
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]managedNodeSummary, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, toManagedNodeSummary(node))
	}
	c.ResponseOk(result)
}

// GetManagedNode returns one managed node.
// @router /api/get-managed-node [get]
func (c *ApiController) GetManagedNode() {
	if c.RequireAdmin() {
		return
	}
	id, err := c.GetInt64("id")
	if err != nil {
		c.ResponseError("invalid id")
		return
	}
	node, err := object.GetManagedNode(id)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if node == nil {
		c.ResponseError("managed node not found")
		return
	}
	c.ResponseOk(toManagedNodeSummary(node))
}

// GetNodeDeployTasks lists deployment tasks for one managed node.
// @router /api/get-node-deploy-tasks [get]
func (c *ApiController) GetNodeDeployTasks() {
	if c.RequireAdmin() {
		return
	}
	nodeId, err := c.GetInt64("nodeId")
	if err != nil {
		c.ResponseError("invalid nodeId")
		return
	}
	tasks, err := object.GetNodeDeployTasks(nodeId, 20)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(tasks)
}

// GetNodeDeployLogs lists logs for one deployment task.
// @router /api/get-node-deploy-logs [get]
func (c *ApiController) GetNodeDeployLogs() {
	if c.RequireAdmin() {
		return
	}
	taskId, err := c.GetInt64("taskId")
	if err != nil {
		c.ResponseError("invalid taskId")
		return
	}
	logs, err := object.GetNodeDeployLogs(taskId, 500)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(logs)
}

// RepairManagedNode queues a repair task for a managed node.
// @router /api/repair-managed-node [post]
func (c *ApiController) RepairManagedNode() {
	if c.RequireAdmin() {
		return
	}
	var req managedNodeRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body")
		return
	}
	node, err := object.GetManagedNode(req.Id)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if node == nil {
		c.ResponseError("managed node not found")
		return
	}
	active, err := object.HasActiveNodeDeployTask(node.Id)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if active {
		c.ResponseError("managed node already has a running task")
		return
	}
	task := &object.NodeDeployTask{
		NodeId:   node.Id,
		Type:     object.NodeTaskTypeRepair,
		Status:   object.NodeTaskStatusPending,
		Stage:    "queued",
		Progress: 0,
	}
	ok, err := object.AddNodeDeployTask(task)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !ok {
		c.ResponseError("failed to create repair task")
		return
	}
	_, _ = object.AddNodeDeployLog(&object.NodeDeployLog{
		TaskId:    task.Id,
		Level:     "info",
		Message:   "queued managed node repair",
		CreatedAt: time.Now(),
	})
	c.ResponseOk(task)
}

// RemoveManagedNode deletes the managed node record and stops tracking it.
// @router /api/remove-managed-node [post]
func (c *ApiController) RemoveManagedNode() {
	if c.RequireAdmin() {
		return
	}
	var req managedNodeRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body")
		return
	}
	node, err := object.GetManagedNode(req.Id)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if node == nil {
		c.ResponseError("managed node not found")
		return
	}
	active, err := object.HasActiveNodeDeployTask(node.Id)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if active {
		c.ResponseError("managed node already has a running task")
		return
	}
	node.State = object.ManagedNodeStateRemoving
	if _, err := object.UpdateManagedNode(node); err != nil {
		c.ResponseError(err.Error())
		return
	}
	task := &object.NodeDeployTask{
		NodeId:   node.Id,
		Type:     object.NodeTaskTypeRemove,
		Status:   object.NodeTaskStatusPending,
		Stage:    "queued",
		Progress: 0,
	}
	ok, err := object.AddNodeDeployTask(task)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !ok {
		c.ResponseError("failed to create remove task")
		return
	}
	_, err = object.AddNodeDeployLog(&object.NodeDeployLog{
		TaskId:    task.Id,
		Level:     "info",
		Message:   "queued managed node removal",
		CreatedAt: time.Now(),
	})
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(task)
}

func toManagedNodeSummary(node *object.ManagedNode) managedNodeSummary {
	if node == nil {
		return managedNodeSummary{}
	}
	return managedNodeSummary{
		Id:               node.Id,
		Name:             node.Name,
		Host:             node.Host,
		Port:             node.Port,
		Username:         node.Username,
		State:            node.State,
		KubernetesStatus: node.KubernetesStatus,
		Labels:           node.GetLabelMap(),
		Unschedulable:    node.Unschedulable,
		OS:               node.OS,
		Arch:             node.Arch,
		Version:          node.Version,
		LastError:        node.LastError,
		LastSeenAt:       formatManagedNodeTime(node.LastSeenAt),
		LastDeployAt:     formatManagedNodeTime(node.LastDeployAt),
		CreatedAt:        formatManagedNodeTime(node.CreatedAt),
		UpdatedAt:        formatManagedNodeTime(node.UpdatedAt),
	}
}

func formatManagedNodeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}
