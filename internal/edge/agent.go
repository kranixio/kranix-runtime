package edge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	kraneTypes "github.com/kranix-io/kranix-packages/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	PhaseConnecting   = "Connecting"
	PhaseConnected    = "Connected"
	PhaseDisconnected = "Disconnected"
	PhaseError        = "Error"
)

type EdgeAgent struct {
	spec     *kraneTypes.EdgeNodeSpec
	status   *kraneTypes.EdgeNodeStatus
	mu       sync.RWMutex
	grpcConn *grpc.ClientConn
	stopChan chan struct{}
	driver   kraneTypes.RuntimeDriver
}

type EdgeNodeServiceClient interface {
	RegisterNode(ctx context.Context, req *RegisterNodeRequest) (*RegisterNodeResponse, error)
	Heartbeat(ctx context.Context, req *HeartbeatRequest) (*HeartbeatResponse, error)
	ReceiveWorkload(ctx context.Context, req *WorkloadRequest) (*WorkloadResponse, error)
}

type RegisterNodeRequest struct {
	NodeID       string                   `json:"nodeId"`
	NodeName     string                   `json:"nodeName"`
	IPAddress    string                   `json:"ipAddress"`
	Port         int32                    `json:"port"`
	Labels       map[string]string        `json:"labels"`
	Capabilities []string                 `json:"capabilities"`
	Resources    *kraneTypes.ResourceSpec `json:"resources"`
	AuthToken    string                   `json:"authToken"`
}

type RegisterNodeResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type HeartbeatRequest struct {
	NodeID           string `json:"nodeId"`
	Timestamp        int64  `json:"timestamp"`
	AvailableGPU     int32  `json:"availableGPU"`
	TotalGPU         int32  `json:"totalGPU"`
	AvailableMemory  string `json:"availableMemory"`
	TotalMemory      string `json:"totalMemory"`
	AvailableCPU     string `json:"availableCPU"`
	TotalCPU         string `json:"totalCPU"`
	RunningWorkloads int32  `json:"runningWorkloads"`
}

type HeartbeatResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type WorkloadRequest struct {
	WorkloadID string                   `json:"workloadId"`
	Spec       *kraneTypes.WorkloadSpec `json:"spec"`
}

type WorkloadResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func NewEdgeAgent(spec *kraneTypes.EdgeNodeSpec, driver kraneTypes.RuntimeDriver) *EdgeAgent {
	return &EdgeAgent{
		spec: spec,
		status: &kraneTypes.EdgeNodeStatus{
			NodeID:           spec.NodeID,
			Phase:            PhaseConnecting,
			LastHeartbeat:    time.Now(),
			RunningWorkloads: 0,
		},
		driver:   driver,
		stopChan: make(chan struct{}),
	}
}

func (a *EdgeAgent) Start(ctx context.Context, controlPlaneURL string) error {
	a.mu.Lock()
	a.status.Phase = PhaseConnecting
	a.mu.Unlock()

	// Connect to control plane
	conn, err := grpc.Dial(controlPlaneURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		a.updateStatus(PhaseError, fmt.Sprintf("failed to connect to control plane: %v", err))
		return fmt.Errorf("failed to connect to control plane: %w", err)
	}
	a.grpcConn = conn

	// Register node
	err = a.registerNode(ctx)
	if err != nil {
		a.updateStatus(PhaseError, fmt.Sprintf("registration failed: %v", err))
		return fmt.Errorf("node registration failed: %w", err)
	}

	a.updateStatus(PhaseConnected, "Successfully connected to control plane")

	// Start heartbeat loop
	go a.heartbeatLoop(ctx)

	// Start workload listener
	go a.workloadListener(ctx)

	return nil
}

func (a *EdgeAgent) Stop() {
	close(a.stopChan)
	if a.grpcConn != nil {
		a.grpcConn.Close()
	}
}

func (a *EdgeAgent) registerNode(ctx context.Context) error {
	_ = &RegisterNodeRequest{
		NodeID:       a.spec.NodeID,
		NodeName:     a.spec.NodeName,
		IPAddress:    a.spec.IPAddress,
		Port:         a.spec.Port,
		Labels:       a.spec.Labels,
		Capabilities: a.spec.Capabilities,
		Resources:    a.spec.Resources,
		AuthToken:    a.spec.AuthToken,
	}

	// In a real implementation, this would call the gRPC service
	// For now, we'll simulate successful registration
	log.Printf("Registering edge node: %s at %s:%d", a.spec.NodeID, a.spec.IPAddress, a.spec.Port)

	return nil
}

func (a *EdgeAgent) heartbeatLoop(ctx context.Context) {
	interval, err := time.ParseDuration(a.spec.HeartbeatInterval)
	if err != nil {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.sendHeartbeat(ctx)
		case <-ctx.Done():
			return
		case <-a.stopChan:
			return
		}
	}
}

func (a *EdgeAgent) sendHeartbeat(ctx context.Context) {
	a.mu.RLock()
	status := a.status
	a.mu.RUnlock()

	_ = &HeartbeatRequest{
		NodeID:           a.spec.NodeID,
		Timestamp:        time.Now().Unix(),
		AvailableGPU:     status.AvailableGPU,
		TotalGPU:         status.TotalGPU,
		AvailableMemory:  status.AvailableMemory,
		TotalMemory:      status.TotalMemory,
		AvailableCPU:     status.AvailableCPU,
		TotalCPU:         status.TotalCPU,
		RunningWorkloads: status.RunningWorkloads,
	}

	// In a real implementation, this would call the gRPC service
	log.Printf("Sending heartbeat from edge node: %s", a.spec.NodeID)

	a.mu.Lock()
	a.status.LastHeartbeat = time.Now()
	a.mu.Unlock()
}

func (a *EdgeAgent) workloadListener(ctx context.Context) {
	// In a real implementation, this would listen for workload deployment requests from the control plane
	// For now, this is a placeholder for the gRPC stream handler
	log.Printf("Edge node %s listening for workload assignments", a.spec.NodeID)
}

func (a *EdgeAgent) DeployWorkload(ctx context.Context, spec *kraneTypes.WorkloadSpec) error {
	_, err := a.driver.Deploy(ctx, spec)
	if err != nil {
		return fmt.Errorf("failed to deploy workload on edge node: %w", err)
	}

	a.mu.Lock()
	a.status.RunningWorkloads++
	a.mu.Unlock()

	return nil
}

func (a *EdgeAgent) DestroyWorkload(ctx context.Context, workloadID string) error {
	err := a.driver.Destroy(ctx, workloadID)
	if err != nil {
		return fmt.Errorf("failed to destroy workload on edge node: %w", err)
	}

	a.mu.Lock()
	if a.status.RunningWorkloads > 0 {
		a.status.RunningWorkloads--
	}
	a.mu.Unlock()

	return nil
}

func (a *EdgeAgent) GetStatus() *kraneTypes.EdgeNodeStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}

func (a *EdgeAgent) updateStatus(phase string, message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status.Phase = phase
	a.status.Message = message
}

// DiscoverLocalResources discovers available resources on the edge node
func (a *EdgeAgent) DiscoverLocalResources() error {
	// In a real implementation, this would query the local system for CPU, memory, GPU
	// For now, we'll set default values
	a.mu.Lock()
	a.status.TotalCPU = "4"
	a.status.AvailableCPU = "4"
	a.status.TotalMemory = "16Gi"
	a.status.AvailableMemory = "16Gi"

	// Detect GPU
	if a.spec.Resources != nil && a.spec.Resources.GPU != nil {
		a.status.TotalGPU = a.spec.Resources.GPU.Count
		a.status.AvailableGPU = a.spec.Resources.GPU.Count
	}
	a.mu.Unlock()

	return nil
}

// MarshalJSON marshals the edge agent status to JSON
func (a *EdgeAgent) MarshalJSON() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return json.Marshal(a.status)
}
