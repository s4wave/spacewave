//go:build !js && !tinygo

package spacewave

import "embed"

// SDKVendorSources retains the generated and handwritten dependency modules
// imported by the TypeScript SDK when no Go checkout exists on the build device.
//
//go:embed vendor/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.ts
//go:embed vendor/github.com/aperturerobotics/controllerbus/controller/controller.pb.ts
//go:embed vendor/github.com/aperturerobotics/controllerbus/controller/exec/exec.pb.ts
//go:embed vendor/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.ts
//go:embed vendor/github.com/aperturerobotics/util/backoff/backoff.pb.ts
//go:embed vendor/github.com/aperturerobotics/util/csync/rwmutex.ts
//go:embed vendor/github.com/aperturerobotics/util/filter/filter.pb.ts
var SDKVendorSources embed.FS
