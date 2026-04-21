package floodsub

import (
	"github.com/s4wave/spacewave/net/pubsub"
)

// subscriptionHandler contains a handler added with AddHandler
type subscriptionHandler struct {
	cb func(m pubsub.Message)
}
