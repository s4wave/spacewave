package psecho

import (
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/pubsub"
)

// publishWantList marshals and publishes the current wantlist snapshot.
func (c *Controller) publishWantList(
	sub pubsub.Subscription,
	refs map[string]*block.BlockRef,
) error {
	msg := &PubSubMessage{
		TimestampUnixNano: time.Now().UnixNano(),
	}

	if len(refs) == 0 {
		msg.WantEmpty = true
	}
	for _, ref := range refs {
		msg.WantRefs = append(msg.WantRefs, ref)
	}

	data, err := msg.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal pubsub message")
	}

	return sub.Publish(data)
}
