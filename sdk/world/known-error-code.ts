import { WorldErrorCode } from './world.pb.js'

// KNOWN_ERROR_CODE is the highest WorldErrorCode this build decodes. Each
// request that can return a refusal specifies it as its knownErrorCode, so a
// server returns a higher code, which this build would read as success, as
// the call's error instead.
export const KNOWN_ERROR_CODE = WorldErrorCode.DURABILITY_UNKNOWN
