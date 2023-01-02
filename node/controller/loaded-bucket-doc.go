package node_controller

// Jobs:
// - acquire bucket handles to volumes PushVolume
// - cancel bucket handles when ClearVolume
// - when a bucket handle arrives:
//   - determine the latest bucket config so far
//   - if the latest is newer or eq to the one in the BH
//     - assert a ApplyBucketConfig optionally before continuing
//     - push the bucket config to the running lookup controller
//   - otherwise, push newer bucket config, re-emit state w/ new config.
//     - re-emit state handle with new config
//     - restart the lookup controller w/ the updated bucket config
// - when a bucket handle is removed:
//    - remove the bucket handle from the working set
//    - if the lookup controller is running: push the updated set

// Code layout:
// Execute:
//   - lock mtx, put ctx=ctx, execute volume trackers already created
//   - wait for wake -> lock mtx
//     - if latest bucket conf > previous state conf
//         - cancel prev lookup controller ctx
//         - clear lookup controller ref
//     - if bucketHandleSetDirty && lookupControllerRef
//        - push updated bucket handle set to lookup controller instance
//     - if stateDirty -> emit state.
//     - start lookup controller routine again w/ new conf if necessary
// PushVolume: lock mtx, if ctx set, start volume tracker
// ClearVolume: lock mtx, if volume tracker started-cancel.
// execLookupController(ctx, bucketConf) -> run lookup controller
//   - once lookup controller ready ->
//     - lock mtx, set ref, if any bucket handles set bucketHandleSetDirty + wake()
//   - once lookup controller dead ->
//     - lock mtx, unset ref if ref == ours, unset bucketHandleSetDirty
