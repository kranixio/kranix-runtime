package kubernetes

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"

	"github.com/kranix-io/kranix-packages/types"
)

func (d *Driver) streamPodLogs(ctx context.Context, podID string, opts *types.LogOptions) (<-chan string, error) {
	logChan := make(chan string, 100)

	namespace := d.namespace

	req := d.clientset.CoreV1().Pods(namespace).GetLogs(podID, &corev1.PodLogOptions{
		Follow:     true,
		TailLines:  int64Ptr(opts.TailLines),
		Timestamps: true,
	})

	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(logChan)
		defer stream.Close()

		buf := make([]byte, 1024)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := stream.Read(buf)
				if err != nil {
					if err != io.EOF {
						return
					}
					return
				}
				if n > 0 {
					logChan <- string(buf[:n])
				}
			}
		}
	}()

	return logChan, nil
}

func int64Ptr(i int) *int64 {
	i64 := int64(i)
	return &i64
}
