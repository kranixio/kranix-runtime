package kubernetes

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/config"
	"github.com/kranix-io/kranix-runtime/internal/health"
)

type Driver struct {
	clientset *kubernetes.Clientset
	cfg       *config.Config
	namespace string
	tracker   *health.BackendTracker
}

func New(cfg *config.Config) (types.RuntimeDriver, error) {
	var k8sConfig *rest.Config
	var err error

	if cfg.Kubernetes.Kubeconfig != "" {
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", cfg.Kubernetes.Kubeconfig)
	} else {
		k8sConfig, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	if cfg.Kubernetes.Context != "" {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{
			CurrentContext: cfg.Kubernetes.Context,
		}
		k8sConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to use context %s: %w", cfg.Kubernetes.Context, err)
		}
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	namespace := cfg.Kubernetes.DefaultNamespace
	if namespace == "" {
		namespace = "default"
	}

	return &Driver{
		clientset: clientset,
		cfg:       cfg,
		namespace: namespace,
		tracker:   health.NewBackendTracker(5*time.Minute, 100),
	}, nil
}

func (d *Driver) Deploy(ctx context.Context, spec *types.WorkloadSpec) (*types.WorkloadStatus, error) {
	if cronScheduleActive(spec) {
		if _, err := d.createCronJob(ctx, spec); err != nil {
			return nil, err
		}
		return d.GetStatus(ctx, spec.Name)
	}

	deployment, err := d.createDeployment(ctx, spec)
	if err != nil {
		return nil, err
	}

	return d.GetStatus(ctx, deployment.Name)
}

func (d *Driver) Destroy(ctx context.Context, workloadID string) error {
	namespace := d.namespace
	err := d.deleteCronJob(ctx, workloadID, namespace)
	if err == nil || !apierrors.IsNotFound(err) {
		return err
	}
	return d.deleteDeployment(ctx, workloadID)
}

func (d *Driver) Restart(ctx context.Context, workloadID string) error {
	namespace := d.namespace
	cj, err := d.clientset.BatchV1().CronJobs(namespace).Get(ctx, workloadID, metav1.GetOptions{})
	if err == nil {
		return d.restartCronJob(ctx, cj)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	return d.restartDeployment(ctx, workloadID)
}

func (d *Driver) GetStatus(ctx context.Context, workloadID string) (*types.WorkloadStatus, error) {
	namespace := d.namespace
	cj, err := d.clientset.BatchV1().CronJobs(namespace).Get(ctx, workloadID, metav1.GetOptions{})
	if err == nil {
		return d.cronJobWorkloadStatus(ctx, cj)
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}
	return d.getDeploymentStatus(ctx, workloadID)
}

func (d *Driver) ListWorkloads(ctx context.Context, namespace string) ([]*types.WorkloadStatus, error) {
	if namespace == "" {
		namespace = d.namespace
	}
	return d.listDeployments(ctx, namespace)
}

func (d *Driver) StreamLogs(ctx context.Context, podID string, opts *types.LogOptions) (<-chan string, error) {
	return d.streamPodLogs(ctx, podID, opts)
}

func (d *Driver) Ping(ctx context.Context) error {
	start := time.Now()
	_, err := d.clientset.ServerVersion()
	if d.tracker != nil {
		d.tracker.Record(float64(time.Since(start).Milliseconds()), err != nil)
	}
	return err
}

func (d *Driver) Backend() string {
	return "kubernetes"
}
