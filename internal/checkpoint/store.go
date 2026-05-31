package checkpoint

import (
	"context"
	"fmt"
	"time"

	"github.com/kranix-io/kranix-packages/types"
)

// Store tracks checkpoint metadata in memory.
type Store struct {
	records map[string]types.CheckpointResult
}

func NewStore() *Store {
	return &Store{records: make(map[string]types.CheckpointResult)}
}

func (s *Store) key(workloadID, checkpointID string) string {
	return workloadID + "/" + checkpointID
}

func (s *Store) Save(workloadID string, result types.CheckpointResult) types.CheckpointResult {
	if result.CheckpointID == "" {
		result.CheckpointID = fmt.Sprintf("ckpt-%d", time.Now().UnixNano())
	}
	result.CreatedAt = time.Now().UTC()
	s.records[s.key(workloadID, result.CheckpointID)] = result
	return result
}

func (s *Store) List(workloadID string) []types.CheckpointResult {
	out := make([]types.CheckpointResult, 0)
	prefix := workloadID + "/"
	for k, v := range s.records {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			out = append(out, v)
		}
	}
	return out
}

func (s *Store) Get(workloadID, checkpointID string) (types.CheckpointResult, bool) {
	v, ok := s.records[s.key(workloadID, checkpointID)]
	return v, ok
}

func ValidateCheckpoint(req types.CheckpointRequest) error {
	if req.WorkloadID == "" {
		return fmt.Errorf("workloadId is required")
	}
	return nil
}

func ValidateRestore(req types.RestoreRequest) error {
	if req.WorkloadID == "" {
		return fmt.Errorf("workloadId is required")
	}
	return nil
}

// NoopExtended is a stub for backends without extended operations.
type NoopExtended struct{}

func (NoopExtended) CheckpointWorkload(ctx context.Context, req types.CheckpointRequest) (*types.CheckpointResult, error) {
	return nil, fmt.Errorf("checkpoint not supported by this backend")
}
func (NoopExtended) RestoreWorkload(ctx context.Context, req types.RestoreRequest) (*types.RestoreResult, error) {
	return nil, fmt.Errorf("restore not supported by this backend")
}
func (NoopExtended) ListCheckpoints(ctx context.Context, workloadID, namespace string) ([]types.CheckpointResult, error) {
	return nil, nil
}
func (NoopExtended) ProvisionVolumes(ctx context.Context, spec *types.WorkloadSpec) (*types.VolumeLifecycleResult, error) {
	if spec == nil {
		return &types.VolumeLifecycleResult{}, nil
	}
	return &types.VolumeLifecycleResult{WorkloadID: spec.Name}, nil
}
func (NoopExtended) CleanupVolumes(ctx context.Context, spec *types.WorkloadSpec) error {
	return nil
}
