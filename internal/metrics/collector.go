package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	kraneTypes "github.com/kranix-io/kranix-packages/types"
)

type Collector struct {
	mu               sync.RWMutex
	metricsCache     map[string]*kraneTypes.ResourceMetrics
	collectionInterval time.Duration
	stopChan         chan struct{}
}

func NewCollector(collectionInterval time.Duration) *Collector {
	return &Collector{
		metricsCache:        make(map[string]*kraneTypes.ResourceMetrics),
		collectionInterval: collectionInterval,
		stopChan:           make(chan struct{}),
	}
}

func (c *Collector) Start(ctx context.Context) {
	ticker := time.NewTicker(c.collectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.collectAllMetrics(ctx)
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		}
	}
}

func (c *Collector) Stop() {
	close(c.stopChan)
}

func (c *Collector) GetMetrics(workloadID string) (*kraneTypes.ResourceMetrics, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metrics, exists := c.metricsCache[workloadID]
	if !exists {
		return nil, fmt.Errorf("metrics not found for workload: %s", workloadID)
	}

	return metrics, nil
}

func (c *Collector) ListMetrics() []*kraneTypes.ResourceMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metrics := make([]*kraneTypes.ResourceMetrics, 0, len(c.metricsCache))
	for _, m := range c.metricsCache {
		metrics = append(metrics, m)
	}

	return metrics
}

func (c *Collector) CollectMetrics(ctx context.Context, workloadID, workloadName, namespace string, backend string) (*kraneTypes.ResourceMetrics, error) {
	metrics := &kraneTypes.ResourceMetrics{
		WorkloadID:   workloadID,
		WorkloadName: workloadName,
		Namespace:    namespace,
		Timestamp:    time.Now(),
	}

	// Collect metrics based on backend
	switch backend {
	case "docker":
		return c.collectDockerMetrics(ctx, metrics)
	case "kubernetes":
		return c.collectKubernetesMetrics(ctx, metrics)
	default:
		return metrics, nil
	}
}

func (c *Collector) collectDockerMetrics(ctx context.Context, metrics *kraneTypes.ResourceMetrics) (*kraneTypes.ResourceMetrics, error) {
	// In a real implementation, this would use Docker stats API
	// For now, we'll return mock data
	metrics.CPUUsage = kraneTypes.CPUMetrics{
		UsageCores:   0.5,
		UsagePercent: 25.0,
		RequestCores: "2",
		LimitCores:   "2",
	}

	metrics.MemoryUsage = kraneTypes.MemoryMetrics{
		UsageBytes:   512 * 1024 * 1024, // 512MB
		UsagePercent: 25.0,
		RequestBytes: 2 * 1024 * 1024 * 1024, // 2GB
		LimitBytes:   2 * 1024 * 1024 * 1024, // 2GB
		CacheBytes:   64 * 1024 * 1024, // 64MB
	}

	return metrics, nil
}

func (c *Collector) collectKubernetesMetrics(ctx context.Context, metrics *kraneTypes.ResourceMetrics) (*kraneTypes.ResourceMetrics, error) {
	// In a real implementation, this would use Kubernetes metrics API
	// For now, we'll return mock data
	metrics.CPUUsage = kraneTypes.CPUMetrics{
		UsageCores:   1.2,
		UsagePercent: 60.0,
		RequestCores: "2",
		LimitCores:   "2",
	}

	metrics.MemoryUsage = kraneTypes.MemoryMetrics{
		UsageBytes:   1024 * 1024 * 1024, // 1GB
		UsagePercent: 50.0,
		RequestBytes: 2 * 1024 * 1024 * 1024, // 2GB
		LimitBytes:   2 * 1024 * 1024 * 1024, // 2GB
		CacheBytes:   128 * 1024 * 1024, // 128MB
	}

	metrics.GPUUsage = []kraneTypes.GPUMetrics{
		{
			DeviceID:      0,
			DeviceName:    "NVIDIA A100",
			Utilization:   75.0,
			MemoryUsedMB:  30 * 1024, // 30GB
			MemoryTotalMB: 40 * 1024, // 40GB
			TemperatureC:  65.0,
			PowerUsageW:   250.0,
		},
	}

	metrics.NetworkMetrics = kraneTypes.NetworkMetrics{
		ReceiveBytesPerSecond:  10 * 1024 * 1024,  // 10MB/s
		TransmitBytesPerSecond: 5 * 1024 * 1024,   // 5MB/s
		ReceivePacketsPerSec:   1000,
		TransmitPacketsPerSec:   500,
		ErrorsPerSec:           0,
	}

	metrics.StorageMetrics = kraneTypes.StorageMetrics{
		ReadBytesPerSecond:  50 * 1024 * 1024,  // 50MB/s
		WriteBytesPerSecond: 20 * 1024 * 1024,  // 20MB/s
		ReadOpsPerSecond:    500,
		WriteOpsPerSecond:   200,
		DiskUsageBytes:      10 * 1024 * 1024 * 1024, // 10GB
		DiskTotalBytes:      100 * 1024 * 1024 * 1024, // 100GB
	}

	return metrics, nil
}

func (c *Collector) collectAllMetrics(ctx context.Context) {
	// In a real implementation, this would collect metrics for all workloads
	// For now, this is a placeholder
}

func (c *Collector) UpdateMetrics(workloadID string, metrics *kraneTypes.ResourceMetrics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	metrics.Timestamp = time.Now()
	c.metricsCache[workloadID] = metrics
}

func (c *Collector) RemoveMetrics(workloadID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.metricsCache, workloadID)
}
