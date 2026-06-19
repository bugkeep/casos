package object

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func GetJobsByCronJob(cfg *rest.Config, namespace, cronJobName string) ([]batchv1.Job, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	list, err := client.BatchV1().Jobs(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var result []batchv1.Job
	for _, job := range list.Items {
		for _, ref := range job.OwnerReferences {
			if ref.Kind == "CronJob" && ref.Name == cronJobName {
				result = append(result, job)
				break
			}
		}
	}
	return result, nil
}

func GetJobPodName(cfg *rest.Config, namespace, jobName string) (string, error) {
	client, err := newClient(cfg)
	if err != nil {
		return "", err
	}
	pods, err := client.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("batch.kubernetes.io/job-name=%s", jobName),
	})
	if err != nil {
		return "", err
	}
	if len(pods.Items) == 0 {
		return "", nil
	}
	return pods.Items[0].Name, nil
}

func TriggerCronJob(cfg *rest.Config, namespace, cronJobName string) (*batchv1.Job, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	cj, err := client.BatchV1().CronJobs(namespace).Get(context.Background(), cronJobName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	ts := time.Now().Unix()
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cronJobName + "-manual-",
			Namespace:    namespace,
			Labels: map[string]string{
				"casos.io/triggered-by": "manual",
				"casos.io/cronjob":      cronJobName,
			},
			Annotations: map[string]string{
				"casos.io/triggered-at": fmt.Sprintf("%d", ts),
			},
		},
		Spec: cj.Spec.JobTemplate.Spec,
	}
	return client.BatchV1().Jobs(namespace).Create(context.Background(), job, metav1.CreateOptions{})
}
