package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kranix-io/kranix-packages/types"
)

func cronScheduleActive(spec *types.WorkloadSpec) bool {
	return spec != nil && spec.CronSchedule != nil &&
		!spec.CronSchedule.Suspended &&
		strings.TrimSpace(spec.CronSchedule.Schedule) != ""
}

func (d *Driver) createCronJob(ctx context.Context, spec *types.WorkloadSpec) (*batchv1.CronJob, error) {
	if err := d.ensureNamespace(ctx, spec.Namespace); err != nil {
		return nil, fmt.Errorf("failed to ensure namespace: %w", err)
	}

	namespace := spec.Namespace
	if namespace == "" {
		namespace = d.namespace
	}

	podSpec, err := d.workloadPodSpec(spec)
	if err != nil {
		return nil, err
	}

	replicas := int32(spec.Replicas)
	if replicas < 1 {
		replicas = 1
	}

	concurrencyPolicy := batchv1.AllowConcurrent
	if cp := strings.ToLower(strings.TrimSpace(spec.CronSchedule.ConcurrencyPolicy)); cp != "" {
		switch cp {
		case "forbid":
			concurrencyPolicy = batchv1.ForbidConcurrent
		case "replace":
			concurrencyPolicy = batchv1.ReplaceConcurrent
		case "allow":
			concurrencyPolicy = batchv1.AllowConcurrent
		}
	}

	historyLimit := int32(3)
	backoff := int32(6)

	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":        spec.Name,
				"managed-by": "kranix",
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                 strings.TrimSpace(spec.CronSchedule.Schedule),
			ConcurrencyPolicy:      concurrencyPolicy,
			SuccessfulJobsHistoryLimit: &historyLimit,
			FailedJobsHistoryLimit:   &historyLimit,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: batchv1.JobSpec{
					Parallelism:  &replicas,
					Completions:  &replicas,
					BackoffLimit: &backoff,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": spec.Name},
						},
						Spec: podSpec,
					},
				},
			},
		},
	}

	if tz := strings.TrimSpace(spec.CronSchedule.TimeZone); tz != "" {
		cronJob.Spec.TimeZone = &tz
	}

	out, err := d.clientset.BatchV1().CronJobs(namespace).Create(ctx, cronJob, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	if err := ensureCrossNamespaceNetworkPolicy(ctx, d.clientset, namespace, spec.Name,
		out.Spec.JobTemplate.Spec.Template.Labels, spec.CrossNamespaceTraffic); err != nil {
		return nil, err
	}

	return out, nil
}

func (d *Driver) cronJobWorkloadStatus(ctx context.Context, cj *batchv1.CronJob) (*types.WorkloadStatus, error) {
	_ = ctx

	containers := cj.Spec.JobTemplate.Spec.Template.Spec.Containers
	image := ""
	if len(containers) > 0 {
		image = containers[0].Image
	}

	parallelism := int32(1)
	if cj.Spec.JobTemplate.Spec.Parallelism != nil {
		parallelism = *cj.Spec.JobTemplate.Spec.Parallelism
	}

	state := ""
	if lst := cj.Status.LastScheduleTime; lst != nil {
		state = "Scheduled"
	} else {
		state = "PendingSchedule"
	}

	st := &types.WorkloadStatus{
		ID:            string(cj.UID),
		Name:          cj.Name,
		Namespace:     cj.Namespace,
		State:         state,
		Image:         image,
		Replicas:      int(parallelism),
		Phase:         types.WorkloadPhaseRunning,
		ReadyReplicas: len(cj.Status.Active),
		LastUpdated:   time.Now(),
	}

	if lst := cj.Status.LastScheduleTime; lst != nil {
		t := lst.Time.UTC()
		st.Cron = &types.CronScheduleStatus{LastScheduleTime: &t}
	}

	return st, nil
}

func (d *Driver) deleteCronJob(ctx context.Context, name, namespace string) error {
	if namespace == "" {
		namespace = d.namespace
	}
	return d.clientset.BatchV1().CronJobs(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (d *Driver) restartCronJob(ctx context.Context, cj *batchv1.CronJob) error {
	if cj.Spec.JobTemplate.ObjectMeta.Annotations == nil {
		cj.Spec.JobTemplate.ObjectMeta.Annotations = map[string]string{}
	}
	cj.Spec.JobTemplate.ObjectMeta.Annotations["kranix/restartedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	_, err := d.clientset.BatchV1().CronJobs(cj.Namespace).Update(ctx, cj, metav1.UpdateOptions{})
	return err
}
