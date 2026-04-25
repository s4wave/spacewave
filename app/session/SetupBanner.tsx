import { useCallback, useMemo } from 'react'
import { LuArrowRight, LuX } from 'react-icons/lu'

import {
  useNavigate,
  useParentPaths,
  usePath,
} from '@s4wave/web/router/router.js'
import { useLocalSessionOnboardingContext } from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'

// SetupBanner renders a persistent inline banner while local session onboarding is incomplete.
export function SetupBanner() {
  const navigate = useNavigate()
  const parentPaths = useParentPaths()
  const path = usePath()
  const basePath = parentPaths[parentPaths.length - 1] ?? path
  const onboarding = useLocalSessionOnboardingContext()

  const onSetupPage = path.startsWith(`${basePath}/setup`)

  const visible = useMemo(() => {
    if (onboarding.loading) return false
    if (!onboarding.metadataLoaded) return false
    if (onSetupPage) return false
    if (onboarding.isComplete) return false
    return !onboarding.onboarding.dismissed
  }, [onboarding, onSetupPage])

  const handleSetupClick = useCallback(() => {
    if (!onboarding.providerChoiceComplete) {
      navigate({ path: `${basePath}/plan` })
      return
    }
    if (!onboarding.onboarding.backupComplete) {
      navigate({ path: `${basePath}/setup` })
      return
    }
    if (!onboarding.onboarding.lockComplete) {
      navigate({ path: `${basePath}/setup` })
    }
  }, [
    navigate,
    onboarding.providerChoiceComplete,
    onboarding.onboarding.backupComplete,
    onboarding.onboarding.lockComplete,
    basePath,
  ])

  const handleDismiss = useCallback(() => {
    onboarding.dismiss()
  }, [onboarding])

  if (!visible) return null

  return (
    <div className="border-brand/20 bg-brand/5 relative flex items-center border-b">
      <button
        onClick={handleSetupClick}
        className="group flex min-w-0 flex-1 cursor-pointer items-center gap-1.5 px-3 py-1.5 transition-colors"
      >
        <p className="text-foreground/80 group-hover:text-foreground text-xs font-medium transition-colors select-none">
          Finish setting up your account
        </p>
        <LuArrowRight className="text-foreground-alt group-hover:text-foreground h-3 w-3 shrink-0 transition-colors" />
      </button>
      <button
        onClick={handleDismiss}
        className="text-foreground-alt hover:text-foreground shrink-0 px-2 py-1.5 transition-colors"
        aria-label="Dismiss setup banner"
      >
        <LuX className="h-3 w-3" />
      </button>
    </div>
  )
}
