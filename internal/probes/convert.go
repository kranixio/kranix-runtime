package probes

import (
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/kranix-io/kranix-packages/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ApplyKubernetesProbes sets startup, liveness, and readiness probes on a container.
// Startup probes block liveness/readiness evaluation until the app is initialized.
func ApplyKubernetesProbes(c *corev1.Container, probes *types.WorkloadProbes) {
	if c == nil || probes == nil {
		return
	}
	if probes.Startup != nil {
		c.StartupProbe = toK8sProbe(probes.Startup)
	}
	if probes.Liveness != nil {
		c.LivenessProbe = toK8sProbe(probes.Liveness)
	}
	if probes.Readiness != nil {
		c.ReadinessProbe = toK8sProbe(probes.Readiness)
	}
}

func toK8sProbe(spec *types.ProbeSpec) *corev1.Probe {
	if spec == nil {
		return nil
	}
	probe := &corev1.Probe{
		InitialDelaySeconds: spec.InitialDelaySeconds,
		PeriodSeconds:       defaultInt32(spec.PeriodSeconds, 10),
		TimeoutSeconds:      defaultInt32(spec.TimeoutSeconds, 1),
		FailureThreshold:    defaultInt32(spec.FailureThreshold, 3),
		SuccessThreshold:    defaultInt32(spec.SuccessThreshold, 1),
	}
	probe.ProbeHandler = probeHandler(spec)
	return probe
}

func probeHandler(spec *types.ProbeSpec) corev1.ProbeHandler {
	switch strings.ToLower(strings.TrimSpace(spec.Type)) {
	case "tcp":
		return corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt32(defaultInt32(spec.Port, 8080)),
			},
		}
	case "exec":
		return corev1.ProbeHandler{
			Exec: &corev1.ExecAction{Command: spec.Command},
		}
	default:
		scheme := corev1.URISchemeHTTP
		if strings.EqualFold(spec.Scheme, "HTTPS") {
			scheme = corev1.URISchemeHTTPS
		}
		path := spec.Path
		if path == "" {
			path = "/"
		}
		return corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   path,
				Port:   intstr.FromInt32(defaultInt32(spec.Port, 8080)),
				Scheme: scheme,
			},
		}
	}
}

// ApplyDockerHealthcheck configures a Docker healthcheck on the container config.
// Startup timing maps to StartPeriod so failures during startup do not mark the container unhealthy.
func ApplyDockerHealthcheck(cfg *container.Config, probes *types.WorkloadProbes) {
	if cfg == nil || probes == nil {
		return
	}
	spec := probes.Liveness
	if spec == nil {
		spec = probes.Startup
	}
	if spec == nil && probes.Readiness != nil {
		spec = probes.Readiness
	}
	if spec == nil {
		return
	}

	startPeriod := int64(0)
	if probes.Startup != nil {
		startPeriod = int64(defaultInt32(probes.Startup.InitialDelaySeconds, 30))
		if probes.Startup.PeriodSeconds > 0 && probes.Startup.FailureThreshold > 0 {
			startPeriod += int64(probes.Startup.PeriodSeconds * probes.Startup.FailureThreshold)
		}
	}

	cfg.Healthcheck = &container.HealthConfig{
		Test:        dockerHealthTest(spec),
		Interval:    time.Duration(defaultInt32(spec.PeriodSeconds, 10)) * time.Second,
		Timeout:     time.Duration(defaultInt32(spec.TimeoutSeconds, 1)) * time.Second,
		Retries:     int(defaultInt32(spec.FailureThreshold, 3)),
		StartPeriod: time.Duration(startPeriod) * time.Second,
	}
}

func dockerHealthTest(spec *types.ProbeSpec) []string {
	switch strings.ToLower(strings.TrimSpace(spec.Type)) {
	case "tcp":
		port := defaultInt32(spec.Port, 8080)
		return []string{"CMD-SHELL", fmt.Sprintf("nc -z localhost %d || exit 1", port)}
	case "exec":
		if len(spec.Command) == 0 {
			return []string{"CMD-SHELL", "exit 0"}
		}
		return append([]string{"CMD-SHELL"}, strings.Join(spec.Command, " "))
	default:
		path := spec.Path
		if path == "" {
			path = "/"
		}
		port := defaultInt32(spec.Port, 8080)
		scheme := "http"
		if strings.EqualFold(spec.Scheme, "HTTPS") {
			scheme = "https"
		}
		return []string{"CMD-SHELL", fmt.Sprintf("curl -fsS %s://localhost:%d%s || exit 1", scheme, port, path)}
	}
}

func defaultInt32(v, fallback int32) int32 {
	if v <= 0 {
		return fallback
	}
	return v
}
