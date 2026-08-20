package server

import (
	"context"
	"time"

	"github.com/beego/beego/logs"
	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/store"
)

// appStoreImageRefreshInterval is how often the admission gate re-reads which
// images belong to App Store releases. Helm releases change only when an
// operator installs, upgrades or removes one, so this is a cheap poll — and
// missing a change costs at most one interval of a newly installed chart going
// unblocked, which is the same fail-open the gate already takes while an
// image's first scan is still running.
const appStoreImageRefreshInterval = 30 * time.Second

// StartAppStoreImageRefresh keeps the admission gate's view of App Store images
// current. Must be called after the apiserver is ready.
func StartAppStoreImageRefresh(ctx context.Context, cfg *rest.Config) {
	go func() {
		ticker := time.NewTicker(appStoreImageRefreshInterval)
		defer ticker.Stop()
		for {
			refreshAppStoreImages(cfg)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func refreshAppStoreImages(cfg *rest.Config) {
	images, err := store.ReleaseImages(cfg)
	if err != nil {
		// Leaving the previous set in place is the safe failure: replacing it
		// with an empty one would silently stop gating every App Store image
		// for as long as Helm is unreachable.
		logs.Warning("app store image refresh: %v", err)
		return
	}
	setAppStoreImages(images)
}
