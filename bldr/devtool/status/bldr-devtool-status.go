package status

import "slices"

// BldrDevtoolStatus is an immutable snapshot of current bldr devtool status.
type BldrDevtoolStatus struct {
	command           BldrDevtoolCommandStatus
	project           BldrDevtoolProjectStatus
	manifestFetchRows []BldrDevtoolManifestFetchRow
	manifestBuildRows []BldrDevtoolManifestBuildRow
	pluginRows        []BldrDevtoolPluginRow
	controllerRows    []BldrDevtoolControllerRow
	attentionRows     []BldrDevtoolAttentionRow
}

// NewBldrDevtoolStatus creates an immutable status snapshot.
func NewBldrDevtoolStatus(
	command BldrDevtoolCommandStatus,
	manifestFetchRows []BldrDevtoolManifestFetchRow,
	manifestBuildRows []BldrDevtoolManifestBuildRow,
	pluginRows []BldrDevtoolPluginRow,
	controllerRows []BldrDevtoolControllerRow,
	attentionRows []BldrDevtoolAttentionRow,
) *BldrDevtoolStatus {
	return &BldrDevtoolStatus{
		command:           command,
		manifestFetchRows: slices.Clone(manifestFetchRows),
		manifestBuildRows: slices.Clone(manifestBuildRows),
		pluginRows:        slices.Clone(pluginRows),
		controllerRows:    slices.Clone(controllerRows),
		attentionRows:     slices.Clone(attentionRows),
	}
}

// EmptyBldrDevtoolStatus creates an empty status snapshot.
func EmptyBldrDevtoolStatus() *BldrDevtoolStatus {
	return &BldrDevtoolStatus{}
}

// GetCommand returns the command status.
func (s *BldrDevtoolStatus) GetCommand() BldrDevtoolCommandStatus {
	return s.command
}

// GetProject returns the project and target status.
func (s *BldrDevtoolStatus) GetProject() BldrDevtoolProjectStatus {
	return s.project.clone()
}

// GetManifestFetchRows returns manifest fetch rows.
func (s *BldrDevtoolStatus) GetManifestFetchRows() []BldrDevtoolManifestFetchRow {
	return slices.Clone(s.manifestFetchRows)
}

// GetManifestBuildRows returns manifest build rows.
func (s *BldrDevtoolStatus) GetManifestBuildRows() []BldrDevtoolManifestBuildRow {
	return slices.Clone(s.manifestBuildRows)
}

// GetPluginRows returns plugin rows.
func (s *BldrDevtoolStatus) GetPluginRows() []BldrDevtoolPluginRow {
	return slices.Clone(s.pluginRows)
}

// GetControllerRows returns controller load and exec rows.
func (s *BldrDevtoolStatus) GetControllerRows() []BldrDevtoolControllerRow {
	return slices.Clone(s.controllerRows)
}

// GetAttentionRows returns recent attention and error rows.
func (s *BldrDevtoolStatus) GetAttentionRows() []BldrDevtoolAttentionRow {
	return slices.Clone(s.attentionRows)
}

// WithCommand returns a copy with command status replaced.
func (s *BldrDevtoolStatus) WithCommand(command BldrDevtoolCommandStatus) *BldrDevtoolStatus {
	next := s.Clone()
	next.command = command
	return next
}

// WithProject returns a copy with project status replaced.
func (s *BldrDevtoolStatus) WithProject(project BldrDevtoolProjectStatus) *BldrDevtoolStatus {
	next := s.Clone()
	next.project = project.clone()
	return next
}

// WithManifestFetchRows returns a copy with manifest fetch rows replaced.
func (s *BldrDevtoolStatus) WithManifestFetchRows(rows []BldrDevtoolManifestFetchRow) *BldrDevtoolStatus {
	next := s.Clone()
	next.manifestFetchRows = enrichManifestFetchRows(slices.Clone(rows), next.manifestBuildRows)
	return next
}

// WithManifestBuildRows returns a copy with manifest build rows replaced.
func (s *BldrDevtoolStatus) WithManifestBuildRows(rows []BldrDevtoolManifestBuildRow) *BldrDevtoolStatus {
	next := s.Clone()
	next.manifestBuildRows = slices.Clone(rows)
	next.manifestFetchRows = enrichManifestFetchRows(next.manifestFetchRows, next.manifestBuildRows)
	return next
}

// WithPluginRows returns a copy with plugin rows replaced.
func (s *BldrDevtoolStatus) WithPluginRows(rows []BldrDevtoolPluginRow) *BldrDevtoolStatus {
	next := s.Clone()
	next.pluginRows = slices.Clone(rows)
	return next
}

// WithControllerRows returns a copy with controller rows replaced.
func (s *BldrDevtoolStatus) WithControllerRows(rows []BldrDevtoolControllerRow) *BldrDevtoolStatus {
	next := s.Clone()
	next.controllerRows = slices.Clone(rows)
	return next
}

// WithAttentionRows returns a copy with recent attention and error rows replaced.
func (s *BldrDevtoolStatus) WithAttentionRows(rows []BldrDevtoolAttentionRow) *BldrDevtoolStatus {
	next := s.Clone()
	next.attentionRows = slices.Clone(rows)
	return next
}

// Clone returns a copy of the snapshot.
func (s *BldrDevtoolStatus) Clone() *BldrDevtoolStatus {
	next := NewBldrDevtoolStatus(
		s.command,
		s.manifestFetchRows,
		s.manifestBuildRows,
		s.pluginRows,
		s.controllerRows,
		s.attentionRows,
	)
	next.project = s.project.clone()
	return next
}

func normalizeBldrDevtoolStatus(snapshot *BldrDevtoolStatus) *BldrDevtoolStatus {
	if snapshot == nil {
		return EmptyBldrDevtoolStatus()
	}
	return snapshot.Clone()
}

func bldrDevtoolStatusEqual(a, b *BldrDevtoolStatus) bool {
	return bldrDevtoolCommandStatusEqual(a.command, b.command) &&
		bldrDevtoolProjectStatusEqual(a.project, b.project) &&
		slices.EqualFunc(a.manifestFetchRows, b.manifestFetchRows, bldrDevtoolManifestFetchRowEqual) &&
		slices.EqualFunc(a.manifestBuildRows, b.manifestBuildRows, bldrDevtoolManifestBuildRowEqual) &&
		slices.EqualFunc(a.pluginRows, b.pluginRows, bldrDevtoolPluginRowEqual) &&
		slices.EqualFunc(a.controllerRows, b.controllerRows, bldrDevtoolControllerRowEqual) &&
		slices.EqualFunc(a.attentionRows, b.attentionRows, bldrDevtoolAttentionRowEqual)
}
