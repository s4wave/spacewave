package cli

import (
	"sort"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/net/pubsub/floodsub"
	floodsub_controller "github.com/s4wave/spacewave/net/pubsub/floodsub/controller"
)

// pubsubFactories contains the static compiled-in pubsub factories
var pubsubFactories [](func(b bus.Bus) controller.Factory)

// pubsubProviders contains the static compiled-in pubsub provider presets
var pubsubProviders = map[string](func(args *DaemonArgs) (config.Config, error)){
	"floodsub": func(args *DaemonArgs) (config.Config, error) {
		return &floodsub_controller.Config{
			FloodsubConfig: &floodsub.Config{},
		}, nil
	},
}

// buildPubsubUsage returns the pubsub usage string
func buildPubsubUsage() string {
	var strb strings.Builder
	_, _ = strb.WriteString("if set, will configure pubsub from options: [")
	keys := make([]string, 0, len(pubsubProviders))
	for k := range pubsubProviders {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		strb.WriteString(k)
		if i != len(keys)-1 {
			strb.WriteString(", ")
		}
	}
	strb.WriteString("]")
	return strb.String()
}
