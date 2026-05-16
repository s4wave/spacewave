package status

import "github.com/pkg/errors"

// BldrDevtoolStatusService streams BldrDevtoolStatus snapshots over SRPC.
type BldrDevtoolStatusService struct {
	// producer owns the immutable status snapshots.
	producer *BldrDevtoolStatusProducer
}

// NewBldrDevtoolStatusService creates a BldrDevtoolStatusService.
func NewBldrDevtoolStatusService(producer *BldrDevtoolStatusProducer) *BldrDevtoolStatusService {
	return &BldrDevtoolStatusService{producer: producer}
}

// WatchDevtoolStatus streams the current status snapshot and later changes.
func (s *BldrDevtoolStatusService) WatchDevtoolStatus(
	_ *WatchDevtoolStatusRequest,
	strm SRPCBldrDevtoolStatusService_WatchDevtoolStatusStream,
) error {
	if s.producer == nil {
		return errors.New("bldr devtool status producer is required")
	}

	ctx := strm.Context()
	statusCtr := s.producer.GetStatusCtr()
	current := statusCtr.GetValue()
	if err := strm.Send(newWatchDevtoolStatusResponse(current)); err != nil {
		return err
	}

	for {
		next, err := statusCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return err
		}
		current = next
		if err := strm.Send(newWatchDevtoolStatusResponse(next)); err != nil {
			return err
		}
	}
}

// _ is a type assertion
var _ SRPCBldrDevtoolStatusServiceServer = ((*BldrDevtoolStatusService)(nil))
