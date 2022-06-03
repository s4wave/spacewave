package bucket_event

import (
	"google.golang.org/protobuf/proto"
)

// UnmarshalBucketEvent unmarshals a bucket event from binary.
func UnmarshalBucketEvent(dat []byte) (*Event, error) {
	e := &Event{}
	if err := proto.Unmarshal(dat, e); err != nil {
		return nil, err
	}
	return e, nil
}
