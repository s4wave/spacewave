// Package s4wave_session is the client contract for a mounted Session: one
// device's signed-in access to one provider account.
//
// SessionResourceService lists the Session's Spaces and SharedObjects, creates,
// renames, leaves and deletes Spaces, mounts a SharedObject as a child resource
// for the s4wave_sobject services, and watches SharedObject health, sync status
// and storage use. It controls the Session's lock (WatchLockState, SetLockMode,
// LockSession, UnlockSession) and runs device pairing (GeneratePairingCode
// through ConfirmPairing, the steps of core/pairing). LocalSessionResourceService
// adds what only a local-provider Session can do, such as managing entity
// keypairs, and SharedObjectSelfEnrollmentResourceService drives a Session's
// enrollment into SharedObjects it was invited to.
package s4wave_session
