package status

import "github.com/aperturerobotics/util/ccontainer"

// BldrDevtoolStatusProducer publishes immutable BldrDevtoolStatus snapshots.
type BldrDevtoolStatusProducer struct {
	ctr *ccontainer.CContainer[*BldrDevtoolStatus]
}

// NewBldrDevtoolStatusProducer creates a BldrDevtoolStatusProducer.
func NewBldrDevtoolStatusProducer(initial *BldrDevtoolStatus) *BldrDevtoolStatusProducer {
	return &BldrDevtoolStatusProducer{
		ctr: ccontainer.NewCContainerWithEqual(
			normalizeBldrDevtoolStatus(initial),
			bldrDevtoolStatusEqual,
		),
	}
}

// GetStatus returns the current immutable snapshot.
func (p *BldrDevtoolStatusProducer) GetStatus() *BldrDevtoolStatus {
	return p.ctr.GetValue()
}

// GetStatusCtr returns the watchable status container.
func (p *BldrDevtoolStatusProducer) GetStatusCtr() ccontainer.Watchable[*BldrDevtoolStatus] {
	return p.ctr
}

// SetStatus publishes the next immutable snapshot.
func (p *BldrDevtoolStatusProducer) SetStatus(snapshot *BldrDevtoolStatus) {
	p.ctr.SetValue(normalizeBldrDevtoolStatus(snapshot))
}

// UpdateStatus atomically builds and publishes the next immutable snapshot.
func (p *BldrDevtoolStatusProducer) UpdateStatus(
	update func(current *BldrDevtoolStatus) *BldrDevtoolStatus,
) *BldrDevtoolStatus {
	if update == nil {
		return p.GetStatus()
	}
	return p.ctr.SwapValue(func(current *BldrDevtoolStatus) *BldrDevtoolStatus {
		return normalizeBldrDevtoolStatus(update(current))
	})
}
