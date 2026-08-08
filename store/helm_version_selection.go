package store

import (
	"sort"

	semver "github.com/Masterminds/semver/v3"
	"helm.sh/helm/v3/pkg/repo"
)

// preferredHelmChartVersion keeps the repository's first valid entry as the
// fallback, but prefers the highest stable SemVer when one is available.
func preferredHelmChartVersion(versions []*repo.ChartVersion) *repo.ChartVersion {
	valid := make([]*repo.ChartVersion, 0, len(versions))
	stable := make([]*repo.ChartVersion, 0, len(versions))
	for _, version := range versions {
		if version == nil || version.Metadata == nil {
			continue
		}
		valid = append(valid, version)
		parsed, err := semver.NewVersion(version.Version)
		if err == nil && parsed.Prerelease() == "" {
			stable = append(stable, version)
		}
	}
	if len(stable) == 0 {
		if len(valid) == 0 {
			return nil
		}
		return valid[0]
	}
	sort.SliceStable(stable, func(i, j int) bool {
		left, _ := semver.NewVersion(stable[i].Version)
		right, _ := semver.NewVersion(stable[j].Version)
		return left.GreaterThan(right)
	})
	return stable[0]
}
