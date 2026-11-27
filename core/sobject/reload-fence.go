package sobject

import "os"

// ReloadFenceEnv turns off the reload fence when set to "off" at startup.
//
// With the fence on, a mount stops writing from the moment a write's outcome
// goes unknown or storage holds a state the mount did not load: the provider
// refuses each later state write with ErrMountReloading, and the Space World
// adapter refuses each later World write, until the restarted mount settles
// what storage holds. With it off, a mount that has called Reload keeps
// writing until the restart ends it, as before the fence. The compare-and-set
// against the loaded token still refuses a write over another writer's state
// either way.
const ReloadFenceEnv = "SPACEWAVE_LOCAL_RELOAD_FENCE"

// ReloadFenceOff selects ReloadFenceEnv's old behavior. It is read once, here,
// at startup, so the provider and the adapter always agree.
var ReloadFenceOff = os.Getenv(ReloadFenceEnv) == "off"
