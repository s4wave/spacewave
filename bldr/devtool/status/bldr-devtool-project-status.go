package status

import "slices"

// BldrDevtoolProjectStatus describes the read-only project and build target snapshot.
type BldrDevtoolProjectStatus struct {
	ProjectID      string
	StartupPlugins []string
	WebStartupPath string
	ManifestIDs    []string
	BuildTargets   []BldrDevtoolBuildTargetRow
}

// BldrDevtoolBuildTargetRow describes one configured build target.
type BldrDevtoolBuildTargetRow struct {
	ID                  string
	ManifestIDs         []string
	ConfiguredTargetIDs []string
	ExplicitPlatformIDs []string
	ResolvedPlatformIDs []string
	BuildTypes          []string
	Error               string
}

func (s BldrDevtoolProjectStatus) clone() BldrDevtoolProjectStatus {
	return BldrDevtoolProjectStatus{
		ProjectID:      s.ProjectID,
		StartupPlugins: slices.Clone(s.StartupPlugins),
		WebStartupPath: s.WebStartupPath,
		ManifestIDs:    slices.Clone(s.ManifestIDs),
		BuildTargets:   cloneBldrDevtoolBuildTargetRows(s.BuildTargets),
	}
}

func bldrDevtoolProjectStatusEqual(a, b BldrDevtoolProjectStatus) bool {
	return a.ProjectID == b.ProjectID &&
		slices.Equal(a.StartupPlugins, b.StartupPlugins) &&
		a.WebStartupPath == b.WebStartupPath &&
		slices.Equal(a.ManifestIDs, b.ManifestIDs) &&
		slices.EqualFunc(a.BuildTargets, b.BuildTargets, bldrDevtoolBuildTargetRowEqual)
}

func cloneBldrDevtoolBuildTargetRows(rows []BldrDevtoolBuildTargetRow) []BldrDevtoolBuildTargetRow {
	cloned := make([]BldrDevtoolBuildTargetRow, len(rows))
	for idx, row := range rows {
		cloned[idx] = row.clone()
	}
	return cloned
}

func (r BldrDevtoolBuildTargetRow) clone() BldrDevtoolBuildTargetRow {
	return BldrDevtoolBuildTargetRow{
		ID:                  r.ID,
		ManifestIDs:         slices.Clone(r.ManifestIDs),
		ConfiguredTargetIDs: slices.Clone(r.ConfiguredTargetIDs),
		ExplicitPlatformIDs: slices.Clone(r.ExplicitPlatformIDs),
		ResolvedPlatformIDs: slices.Clone(r.ResolvedPlatformIDs),
		BuildTypes:          slices.Clone(r.BuildTypes),
		Error:               r.Error,
	}
}

func bldrDevtoolBuildTargetRowEqual(a, b BldrDevtoolBuildTargetRow) bool {
	return a.ID == b.ID &&
		slices.Equal(a.ManifestIDs, b.ManifestIDs) &&
		slices.Equal(a.ConfiguredTargetIDs, b.ConfiguredTargetIDs) &&
		slices.Equal(a.ExplicitPlatformIDs, b.ExplicitPlatformIDs) &&
		slices.Equal(a.ResolvedPlatformIDs, b.ResolvedPlatformIDs) &&
		slices.Equal(a.BuildTypes, b.BuildTypes) &&
		a.Error == b.Error
}
