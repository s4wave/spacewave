import { useCallback } from 'react'
import { LuDatabase, LuShieldCheck } from 'react-icons/lu'

import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'

import { StorageHealth } from './StorageHealth.js'

interface StorageHealthPageProps {
  recovery?: boolean
}

// StorageHealthPage renders the session storage settings and dedicated recovery route.
export function StorageHealthPage({
  recovery = false,
}: StorageHealthPageProps) {
  const navigateSession = useSessionNavigate()
  const handleBack = useCallback(() => {
    navigateSession({ path: '' })
  }, [navigateSession])
  const handleOpenRecovery = useCallback(() => {
    navigateSession({ path: 'settings/storage/recovery' })
  }, [navigateSession])
  const handleLinkDevice = useCallback(() => {
    navigateSession({ path: 'setup/link-device' })
  }, [navigateSession])
  const handleUseCloud = useCallback(() => {
    navigateSession({ path: 'plan' })
  }, [navigateSession])

  const Icon = recovery ? LuShieldCheck : LuDatabase
  return (
    <div className="bg-background-primary relative flex h-full w-full items-start justify-center overflow-y-auto pt-16 pb-10">
      <main className="w-full max-w-md px-4">
        <BackButton floating onClick={handleBack}>
          Back
        </BackButton>
        <div className="mb-6 flex items-center gap-2">
          <Icon
            className="text-foreground size-5 shrink-0"
            aria-hidden="true"
          />
          <div>
            <h1 className="text-foreground text-lg font-semibold tracking-tight">
              {recovery ? 'Storage recovery' : 'Storage health'}
            </h1>
            <p className="text-foreground-alt/60 mt-1 text-xs">
              {recovery
                ? 'Review browser protection and backup options.'
                : 'See what this browser, this device, and sync activity report.'}
            </p>
          </div>
        </div>

        <StorageHealth
          mode={recovery ? 'recovery' : 'settings'}
          onOpenRecovery={recovery ? undefined : handleOpenRecovery}
          onLinkDevice={handleLinkDevice}
          onUseCloud={handleUseCloud}
        />
      </main>
    </div>
  )
}
