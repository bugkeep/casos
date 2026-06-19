package controllers

import (
	"encoding/json"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

type cronJobSummary struct {
	Namespace               string `json:"namespace"`
	Name                    string `json:"name"`
	Schedule                string `json:"schedule"`
	Suspend                 bool   `json:"suspend"`
	LastScheduleTime        string `json:"lastScheduleTime"`
	CreatedAt               string `json:"createdAt"`
	ResourceVersion         string `json:"resourceVersion"`
	Image                   string `json:"image"`
	Command                 string `json:"command"`
	ConcurrencyPolicy       string `json:"concurrencyPolicy"`
	SuccessfulJobsHistLimit int32  `json:"successfulJobsHistLimit"`
	FailedJobsHistLimit     int32  `json:"failedJobsHistLimit"`
}

func toCronJobSummary(cj batchv1.CronJob) cronJobSummary {
	image := ""
	command := ""
	containers := cj.Spec.JobTemplate.Spec.Template.Spec.Containers
	if len(containers) > 0 {
		image = containers[0].Image
		if len(containers[0].Command) > 0 {
			command = fmt.Sprintf("%v", containers[0].Command)
		}
	}
	lastSchedule := ""
	if cj.Status.LastScheduleTime != nil {
		lastSchedule = cj.Status.LastScheduleTime.UTC().Format("2006-01-02 15:04:05")
	}
	successLimit := int32(3)
	if cj.Spec.SuccessfulJobsHistoryLimit != nil {
		successLimit = *cj.Spec.SuccessfulJobsHistoryLimit
	}
	failedLimit := int32(1)
	if cj.Spec.FailedJobsHistoryLimit != nil {
		failedLimit = *cj.Spec.FailedJobsHistoryLimit
	}
	return cronJobSummary{
		Namespace:               cj.Namespace,
		Name:                    cj.Name,
		Schedule:                cj.Spec.Schedule,
		Suspend:                 cj.Spec.Suspend != nil && *cj.Spec.Suspend,
		LastScheduleTime:        lastSchedule,
		CreatedAt:               cj.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion:         cj.ResourceVersion,
		Image:                   image,
		Command:                 command,
		ConcurrencyPolicy:       string(cj.Spec.ConcurrencyPolicy),
		SuccessfulJobsHistLimit: successLimit,
		FailedJobsHistLimit:     failedLimit,
	}
}

// GetCronJobs
// @router /api/get-cronjobs [get]
func (c *ApiController) GetCronJobs() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	cjs, err := object.GetCronJobs(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]cronJobSummary, 0, len(cjs))
	for _, cj := range cjs {
		result = append(result, toCronJobSummary(cj))
	}
	c.ResponseOk(result)
}

// GetCronJob
// @router /api/get-cronjob [get]
func (c *ApiController) GetCronJob() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	cj, err := object.GetCronJob(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toCronJobSummary(*cj))
}

type cronJobRequest struct {
	Namespace               string   `json:"namespace"`
	Name                    string   `json:"name"`
	Schedule                string   `json:"schedule"`
	Image                   string   `json:"image"`
	Command                 []string `json:"command"`
	ConcurrencyPolicy       string   `json:"concurrencyPolicy"`
	Suspend                 bool     `json:"suspend"`
	SuccessfulJobsHistLimit int32    `json:"successfulJobsHistLimit"`
	FailedJobsHistLimit     int32    `json:"failedJobsHistLimit"`
	ResourceVersion         string   `json:"resourceVersion"`
}

func buildCronJob(req cronJobRequest) *batchv1.CronJob {
	suspend := req.Suspend
	successLimit := req.SuccessfulJobsHistLimit
	failedLimit := req.FailedJobsHistLimit
	concurrencyPolicy := batchv1.ConcurrencyPolicy(req.ConcurrencyPolicy)
	if concurrencyPolicy == "" {
		concurrencyPolicy = batchv1.AllowConcurrent
	}
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:            req.Name,
			Namespace:       req.Namespace,
			ResourceVersion: req.ResourceVersion,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   req.Schedule,
			ConcurrencyPolicy:          concurrencyPolicy,
			Suspend:                    &suspend,
			SuccessfulJobsHistoryLimit: &successLimit,
			FailedJobsHistoryLimit:     &failedLimit,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name:    req.Name,
									Image:   req.Image,
									Command: req.Command,
								},
							},
						},
					},
				},
			},
		},
	}
}

// AddCronJob
// @router /api/add-cronjob [post]
func (c *ApiController) AddCronJob() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req cronJobRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	cj := buildCronJob(req)
	created, err := object.AddCronJob(cfg, cj)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toCronJobSummary(*created))
}

// UpdateCronJob
// @router /api/update-cronjob [post]
func (c *ApiController) UpdateCronJob() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req cronJobRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	cj := buildCronJob(req)
	updated, err := object.UpdateCronJob(cfg, cj)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toCronJobSummary(*updated))
}

type jobSummary struct {
	Namespace      string `json:"namespace"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	StartTime      string `json:"startTime"`
	CompletionTime string `json:"completionTime"`
	Duration       string `json:"duration"`
	PodName        string `json:"podName"`
	Manual         bool   `json:"manual"`
}

func toJobSummary(job batchv1.Job, podName string) jobSummary {
	status := "pending"
	if job.Status.Active > 0 {
		status = "running"
	} else if job.Status.Succeeded > 0 {
		status = "succeeded"
	} else if job.Status.Failed > 0 {
		status = "failed"
	}
	start := ""
	completion := ""
	duration := ""
	if job.Status.StartTime != nil {
		start = job.Status.StartTime.UTC().Format("2006-01-02 15:04:05")
		if job.Status.CompletionTime != nil {
			completion = job.Status.CompletionTime.UTC().Format("2006-01-02 15:04:05")
			d := job.Status.CompletionTime.Sub(job.Status.StartTime.Time).Round(time.Second)
			duration = d.String()
		} else if status == "running" {
			d := time.Since(job.Status.StartTime.Time).Round(time.Second)
			duration = d.String()
		}
	}
	manual := job.Labels["casos.io/triggered-by"] == "manual"
	return jobSummary{
		Namespace:      job.Namespace,
		Name:           job.Name,
		Status:         status,
		StartTime:      start,
		CompletionTime: completion,
		Duration:       duration,
		PodName:        podName,
		Manual:         manual,
	}
}

// GetCronJobJobs returns the execution history (Jobs) for a CronJob.
// @router /api/get-cronjob-jobs [get]
func (c *ApiController) GetCronJobJobs() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	cronJobName := c.GetString("name")
	if namespace == "" {
		namespace = "default"
	}
	jobs, err := object.GetJobsByCronJob(cfg, namespace, cronJobName)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]jobSummary, 0, len(jobs))
	for _, job := range jobs {
		podName, _ := object.GetJobPodName(cfg, namespace, job.Name)
		result = append(result, toJobSummary(job, podName))
	}
	c.ResponseOk(result)
}

// TriggerCronJob manually creates a Job from a CronJob immediately.
// @router /api/trigger-cronjob [post]
func (c *ApiController) TriggerCronJob() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	job, err := object.TriggerCronJob(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	podName, _ := object.GetJobPodName(cfg, req.Namespace, job.Name)
	c.ResponseOk(toJobSummary(*job, podName))
}

// DeleteCronJob
// @router /api/delete-cronjob [post]
func (c *ApiController) DeleteCronJob() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req cronJobRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.DeleteCronJob(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
