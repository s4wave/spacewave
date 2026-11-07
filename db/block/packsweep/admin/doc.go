// Package block_packsweep_admin is the admin service a backend serves for a
// pack sweep: the packs' sweep states and the write log, the placement and
// child lists of blocks, and the moves of packs between the states. A backend
// mounts its HTTP handler behind its own admin authorization, and passes a
// switch that refuses every move while it is off, so the service can ship for
// a dry run that changes nothing.
package block_packsweep_admin
