package psecho

import (
	"time"

	"github.com/aperturerobotics/bifrost/pubsub"
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
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
