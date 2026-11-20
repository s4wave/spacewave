// Package s4wave_sobject is the client contract for a mounted SharedObject.
//
// SharedObjectResourceService watches the SharedObject's health and the
// authority this Session holds in it, opens the World history retained before
// a departure (OpenReadCheckpoint), reports how far its history can be trimmed
// and its agreed operation order, and mounts its body, such as a Space, as a
// child resource. A body mount returns either the child's resource ID or a typed
// health value saying why the body is not available, so a caller acts on the
// health code rather than on error text.
package s4wave_sobject
