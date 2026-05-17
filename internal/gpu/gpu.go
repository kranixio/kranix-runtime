package gpu

import (
	"fmt"
	"strings"

	kraneTypes "github.com/kranix-io/kranix-packages/types"
)

const (
	// VendorNVIDIA represents NVIDIA GPUs
	VendorNVIDIA = "nvidia"
	// VendorAMD represents AMD GPUs
	VendorAMD = "amd"
)

// GetGPUResourceName returns the Kubernetes resource name for a GPU vendor.
func GetGPUResourceName(vendor string) (string, error) {
	switch strings.ToLower(vendor) {
	case VendorNVIDIA:
		return "nvidia.com/gpu", nil
	case VendorAMD:
		return "amd.com/gpu", nil
	default:
		return "", fmt.Errorf("unsupported GPU vendor: %s", vendor)
	}
}

// ValidateGPUSpec validates a GPU specification.
func ValidateGPUSpec(spec *kraneTypes.GPUSpec) error {
	if spec == nil {
		return nil
	}

	if spec.Count <= 0 {
		return fmt.Errorf("GPU count must be positive, got: %d", spec.Count)
	}

	vendor := strings.ToLower(spec.Vendor)
	if vendor != VendorNVIDIA && vendor != VendorAMD {
		return fmt.Errorf("unsupported GPU vendor: %s (must be 'nvidia' or 'amd')", spec.Vendor)
	}

	return nil
}

// BuildDockerDeviceRequests builds Docker device requests for GPU resources.
func BuildDockerDeviceRequests(spec *kraneTypes.GPUSpec) ([]map[string]string, error) {
	if spec == nil {
		return nil, nil
	}

	if err := ValidateGPUSpec(spec); err != nil {
		return nil, err
	}

	vendor := strings.ToLower(spec.Vendor)
	var devices []map[string]string

	switch vendor {
	case VendorNVIDIA:
		// NVIDIA GPU device request
		devices = append(devices, map[string]string{
			"Driver":       "nvidia",
			"DeviceIDs":    "all",
			"Capabilities": "gpu,compute,utility,display",
		})
	case VendorAMD:
		// AMD GPU device request
		devices = append(devices, map[string]string{
			"Driver":    "amdgpu",
			"DeviceIDs": "all",
		})
	}

	return devices, nil
}

// BuildKubernetesResourceRequirements builds Kubernetes resource requirements for GPU.
func BuildKubernetesResourceRequirements(spec *kraneTypes.GPUSpec) (map[string]string, error) {
	if spec == nil {
		return nil, nil
	}

	if err := ValidateGPUSpec(spec); err != nil {
		return nil, err
	}

	resourceName, err := GetGPUResourceName(spec.Vendor)
	if err != nil {
		return nil, err
	}

	resources := make(map[string]string)
	resources[resourceName] = fmt.Sprintf("%d", spec.Count)

	return resources, nil
}

// DetectGPUVendor detects the GPU vendor from node labels or system info.
func DetectGPUVendor(nodeLabels map[string]string) string {
	if _, ok := nodeLabels["nvidia.com/gpu.present"]; ok {
		return VendorNVIDIA
	}
	if _, ok := nodeLabels["amd.com/gpu.present"]; ok {
		return VendorAMD
	}
	return ""
}
