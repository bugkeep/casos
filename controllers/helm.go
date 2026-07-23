package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/store"
)

// ---------- ArtifactHub proxy ----------

type ahSearchResult struct {
	Packages []json.RawMessage `json:"packages"`
}

// SearchArtifactHub proxies a search to the ArtifactHub REST API.
// @router /api/search-artifact-hub [get]
func (c *ApiController) SearchArtifactHub() {
	if c.RequireSignedIn() {
		return
	}
	q := c.GetString("q")
	page, _ := c.GetInt("page", 1)
	limit, _ := c.GetInt("limit", 20)
	offset := (page - 1) * limit

	url := fmt.Sprintf(
		"https://artifacthub.io/api/v1/packages/search?kind=0&limit=%d&offset=%d",
		limit, offset,
	)
	if q != "" {
		url += "&ts_query_web=" + q
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	var result ahSearchResult
	if err := json.Unmarshal(body, &result); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(result.Packages)
}

// ---------- Custom repo CRUD (persisted via object/DB) ----------

// GetHelmRepos returns all persisted custom Helm repos.
// @router /api/get-helm-repos [get]
func (c *ApiController) GetHelmRepos() {
	if c.RequireSignedIn() {
		return
	}
	repos, err := object.GetHelmRepos()
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(repos)
}

// AddHelmRepo persists a new custom Helm repo.
// @router /api/add-helm-repo [post]
func (c *ApiController) AddHelmRepo() {
	if c.RequireAdmin() {
		return
	}
	var repo object.HelmRepo
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &repo); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if err := object.AddHelmRepo(&repo); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}

// DeleteHelmRepo deletes a custom Helm repo by id.
// @router /api/delete-helm-repo [post]
func (c *ApiController) DeleteHelmRepo() {
	if c.RequireAdmin() {
		return
	}
	id, err := c.GetInt("id")
	if err != nil {
		c.ResponseError("invalid id")
		return
	}
	if err := object.DeleteHelmRepo(id); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}

// ---------- Repo index browsing (via store/Helm SDK) ----------

// GetRepoCharts fetches and returns a chart listing from a Helm repo's index.yaml.
// @router /api/get-repo-charts [get]
func (c *ApiController) GetRepoCharts() {
	if c.RequireSignedIn() {
		return
	}
	repoURL := c.GetString("url")
	if repoURL == "" {
		c.ResponseError("url is required")
		return
	}
	charts, err := store.FetchRepoIndex(repoURL)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(charts)
}

// ---------- Chart values (via store/Helm SDK) ----------

// GetHelmChartValues fetches the default values.yaml for a chart.
// @router /api/get-helm-chart-values [get]
func (c *ApiController) GetHelmChartValues() {
	if c.RequireSignedIn() {
		return
	}
	chartName := c.GetString("chart")
	repoURL := c.GetString("repo")
	version := c.GetString("version")
	if chartName == "" || repoURL == "" {
		c.ResponseError("chart and repo are required")
		return
	}
	values, err := store.GetHelmChartDefaultValues(chartName, repoURL, version)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(values)
}

// ---------- Release lifecycle (via store/Helm SDK) ----------

// GetHelmReleases lists installed Helm releases.
// @router /api/get-helm-releases [get]
func (c *ApiController) GetHelmReleases() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("cluster not ready")
		return
	}
	namespace := c.GetString("namespace", "all")
	releases, err := store.GetHelmReleases(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(releases)
}

type helmInstallReq struct {
	ReleaseName string `json:"releaseName"`
	Namespace   string `json:"namespace"`
	ChartName   string `json:"chartName"`
	RepoURL     string `json:"repoURL"`
	Version     string `json:"version"`
	ValuesYAML  string `json:"valuesYAML"`
}

// InstallHelmChart installs a new Helm release.
// @router /api/install-helm-chart [post]
func (c *ApiController) InstallHelmChart() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("cluster not ready")
		return
	}
	var req helmInstallReq
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if err := store.InstallHelmChart(cfg, req.ReleaseName, req.Namespace, req.ChartName, req.RepoURL, req.Version, req.ValuesYAML); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}

// InstallHelmChartStream streams helm install progress as Server-Sent Events.
// @router /api/install-helm-chart-stream [post]
func (c *ApiController) InstallHelmChartStream() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.Ctx.ResponseWriter.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(c.Ctx.ResponseWriter.ResponseWriter, "data: ERROR: cluster not ready\n\n")
		c.StopRun()
		return
	}
	var req helmInstallReq
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.Ctx.ResponseWriter.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(c.Ctx.ResponseWriter.ResponseWriter, "data: ERROR: %s\n\n", err.Error())
		c.StopRun()
		return
	}

	w := c.Ctx.ResponseWriter.ResponseWriter
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)
	ctx := c.Ctx.Request.Context()
	task, err := object.CreateHelmOperationTask(helmOperationOwner(c), object.HelmOperationInstall, req.ReleaseName, req.Namespace, req.ChartName, req.Version)
	if err != nil {
		fmt.Fprintf(w, "data: ERROR: %s\n\n", err.Error())
		c.StopRun()
		return
	}
	if _, err := fmt.Fprintf(w, "data: TASK_ID:%d\n\n", task.Id); err != nil {
		_ = object.FinishHelmOperationTask(task.Id, false, fmt.Sprintf("failed to send Helm operation task id: %v", err))
		c.StopRun()
		return
	}
	if canFlush {
		flusher.Flush()
	}
	recorder := object.NewHelmOperationRecorder(task.Id)
	logCh := store.InstallHelmChartStream(ctx, recorder, cfg, req.ReleaseName, req.Namespace, req.ChartName, req.RepoURL, req.Version, req.ValuesYAML)
	for line := range logCh {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", line); err != nil {
			break
		}
		if canFlush {
			flusher.Flush()
		}
	}
	c.StopRun()
}

// GetHelmOperationTask returns a persisted install task and its log history so
// an administrator can reconnect after an SSE stream is interrupted.
// @router /api/get-helm-operation-task [get]
func (c *ApiController) GetHelmOperationTask() {
	if c.RequireAdmin() {
		return
	}
	id, err := strconv.ParseInt(c.GetString("id"), 10, 64)
	if err != nil || id <= 0 {
		c.ResponseError("invalid task id")
		return
	}
	if err := object.ExpireStaleHelmOperationTask(id); err != nil {
		c.ResponseError(err.Error())
		return
	}
	task, err := object.GetHelmOperationTask(id)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if task == nil || task.Owner != helmOperationOwner(c) {
		c.ResponseError("Helm operation task not found")
		return
	}
	logs, err := object.GetHelmOperationLogs(id, 1000)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(task, logs)
}

func helmOperationOwner(c *ApiController) string {
	if user := c.GetSessionUser(); user != nil {
		if user.Name != "" {
			return user.Name
		}
		return user.Id
	}
	return ""
}

// UpgradeHelmRelease upgrades an existing Helm release.
// @router /api/upgrade-helm-release [post]
func (c *ApiController) UpgradeHelmRelease() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("cluster not ready")
		return
	}
	var req helmInstallReq
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if err := store.UpgradeHelmRelease(cfg, req.ReleaseName, req.Namespace, req.ChartName, req.RepoURL, req.Version, req.ValuesYAML); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}

type helmRollbackReq struct {
	ReleaseName string `json:"releaseName"`
	Namespace   string `json:"namespace"`
	Revision    int    `json:"revision"`
}

// RollbackHelmRelease rolls back a Helm release to a previous revision.
// @router /api/rollback-helm-release [post]
func (c *ApiController) RollbackHelmRelease() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("cluster not ready")
		return
	}
	var req helmRollbackReq
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if err := store.RollbackHelmRelease(cfg, req.ReleaseName, req.Namespace, req.Revision); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}

type helmUninstallReq struct {
	ReleaseName string `json:"releaseName"`
	Namespace   string `json:"namespace"`
}

// UninstallHelmRelease removes a Helm release from the cluster.
// @router /api/uninstall-helm-release [post]
func (c *ApiController) UninstallHelmRelease() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("cluster not ready")
		return
	}
	var req helmUninstallReq
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if err := store.UninstallHelmRelease(cfg, req.ReleaseName, req.Namespace); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}

// GetHelmReleaseHistory returns the revision history of a release.
// @router /api/get-helm-release-history [get]
func (c *ApiController) GetHelmReleaseHistory() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("cluster not ready")
		return
	}
	name := c.GetString("name")
	namespace := c.GetString("namespace")
	if name == "" || namespace == "" {
		c.ResponseError("name and namespace are required")
		return
	}
	history, err := store.GetHelmReleaseHistory(cfg, name, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(history)
}
