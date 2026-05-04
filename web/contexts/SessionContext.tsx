import { createResourceContext } from '@aptre/bldr-sdk/hooks/createResourceContext.js'
import type { Session } from '@s4wave/sdk/session/session.js'

// SessionContext provides the Session resource to child components.
export const SessionContext = createResourceContext<Session>()
