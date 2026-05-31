package imageutil

import (
	"github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/internal/arch"
)

// DetectPlatform infers OCI/linux platform from an image reference.
func DetectPlatform(image string) string {
	switch arch.DetectFromImage(image) {
	case types.ArchARM64:
		return "linux/arm64"
	case types.ArchAMD64:
		return "linux/amd64"
	default:
		return ""
	}
}
