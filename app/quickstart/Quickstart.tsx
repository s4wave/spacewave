/**
 * Quickstart Component
 *
 * Handles the automatic setup of a session and space for quick onboarding.
 * This component orchestrates the creation of a local provider account,
 * mounting a session, and creating a new space with a specific quickstart ID.
 */

import { Redirect } from '@s4wave/web/router/Redirect.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'

import { createQuickstartSetup, createLocalSession } from './create.js'
import { LoadingScreen } from './LoadingScreen.js'
import {
  isQuickstartCreateId,
  type QuickstartSpaceCreateId,
  type QuickstartId,
} from './options.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'

interface QuickstartProps {
  quickstartId: QuickstartId
}

interface QuickstartErrorStateProps {
  message: string
  onRetry?: () => void
}

function QuickstartErrorState({ message, onRetry }: QuickstartErrorStateProps) {
  const navigate = useNavigate()

  return (
    <ErrorState
      variant="fullscreen"
      className="relative"
      title="Setup Failed"
      message={message}
      onRetry={onRetry}
    >
      <BackButton floating onClick={() => navigate({ path: '../../' })}>
        Back to home
      </BackButton>
    </ErrorState>
  )
}

/**
 * Quickstart component that automatically sets up a session and space.
 *
 * Flow:
 * 1. Load the root resource
 * 2. Create a local provider account
 * 3. Mount a session using the account
 * 4. Create a space with the quickstart ID (unless 'local')
 * 5. Provide the session context to children
 */
export const Quickstart: React.FC<QuickstartProps> = ({ quickstartId }) => {
  // isCreate indicates this is an option that should call createQuickstartSetup.
  // otherwise we redirect below.
  const isCreate = isQuickstartCreateId(quickstartId)
  const isLocal = quickstartId === 'local'
  const rootResource = useRootResource()

  // For 'local' (login page "Continue without account"), always create new session.
  const localSessionResource = useResource(
    rootResource,
    async (root, signal, cleanup) => {
      return createLocalSession(root, signal, cleanup, true)
    },
    [],
    { enabled: isLocal },
  )

  // For other create options, we create account/session/space
  const setupResource = useResource(
    rootResource,
    async (root, signal, cleanup) => {
      if (!isCreate || isLocal) return null
      return createQuickstartSetup(
        root,
        quickstartId as QuickstartSpaceCreateId,
        signal,
        cleanup,
      )
    },
    [isCreate, isLocal, quickstartId],
    { enabled: isCreate && !isLocal },
  )

  // account option: redirect to /login
  if (quickstartId === 'account') {
    return <NavigatePath to="/login" />
  }

  if (!isCreate) {
    // We shouldn't have gotten here
    console.error(`unknown quickstart option: ${String(quickstartId)}`)
    return <NavigatePath to="/" />
  }

  // Handle 'local' quickstart
  if (isLocal) {
    if (localSessionResource.error) {
      return (
        <QuickstartErrorState
          message={localSessionResource.error.message}
          onRetry={localSessionResource.retry}
        />
      )
    }

    const localSetup = localSessionResource.value

    if (localSessionResource.loading || !localSetup) {
      return <LoadingScreen quickstartId={quickstartId} />
    }

    return <Redirect to={`/u/${localSetup.sessionIndex}`} />
  }

  // Handle other create options (with space)
  if (setupResource.error) {
    return (
      <QuickstartErrorState
        message={setupResource.error.message}
        onRetry={setupResource.retry}
      />
    )
  }

  const setup = setupResource.value
  const spaceID = setup?.spaceResp.sharedObjectRef?.providerResourceRef?.id

  if (setupResource.loading || !setup || !spaceID) {
    return <LoadingScreen quickstartId={quickstartId} />
  }

  return <Redirect to={`/u/${setup.sessionIndex}/so/${spaceID}`} />
}
