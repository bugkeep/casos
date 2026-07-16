package server

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
)

const platformBootstrapRetryInterval = 5 * time.Second

// BootstrapUntilReady retries idempotent platform bootstrap after transient
// API, image, or resource-convergence failures until shutdown.
func BootstrapUntilReady(ctx context.Context, cfg *rest.Config, srvCfg Config) error {
	return retryBootstrap(ctx, platformBootstrapRetryInterval, func() error {
		return Bootstrap(ctx, cfg, srvCfg)
	})
}

func retryBootstrap(ctx context.Context, interval time.Duration, attempt func() error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = platformBootstrapRetryInterval
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := attempt(); err == nil {
			return nil
		} else {
			logrus.Warnf("platform bootstrap failed; retrying in %s: %v", interval, err)
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
