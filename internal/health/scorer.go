package health

import "math"

// ScoreFromSignals computes a 0-100 health score from latency (ms) and error rate (0-1).
func ScoreFromSignals(latencyMs, errorRate float64) int {
	score := 100.0

	switch {
	case latencyMs > 2000:
		score -= 40
	case latencyMs > 1000:
		score -= 25
	case latencyMs > 500:
		score -= 15
	case latencyMs > 200:
		score -= 8
	case latencyMs > 100:
		score -= 4
	}

	score -= errorRate * 100
	if errorRate > 0.5 {
		score -= 20
	}

	return clampScore(score)
}

// ScoreNodeReady adjusts a base score for Kubernetes node conditions.
func ScoreNodeReady(base int, ready, memoryPressure, diskPressure, pidPressure, unschedulable, draining bool) int {
	score := float64(base)
	if !ready {
		score -= 50
	}
	if memoryPressure {
		score -= 15
	}
	if diskPressure {
		score -= 15
	}
	if pidPressure {
		score -= 10
	}
	if unschedulable {
		score -= 10
	}
	if draining {
		score -= 25
	}
	return clampScore(score)
}

func clampScore(v float64) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return int(math.Round(v))
}
