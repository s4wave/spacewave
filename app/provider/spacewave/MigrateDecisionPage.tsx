import { useCallback, useState } from 'react'
import {
  LuArrowLeft,
  LuArrowRight,
  LuCloud,
  LuHardDrive,
  LuUnlink,
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import {
  CLOUD_FAQ,
  FaqAccordion,
  PageFooter,
  PageWrapper,
} from './CloudConfirmationPage.js'

// MigrateDecisionPage renders the migration decision UI at /plan/migrate.
// Shown when the user has an active subscription and a linked local session
// with content. Offers two choices: migrate data to cloud, or keep sessions
// separate (unlink the local session).
export function MigrateDecisionPage() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const navigate = useNavigate()
  const [unlinking, setUnlinking] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  const handleMigrate = useCallback(() => {
    navigate({ path: '../../settings/migration' })
  }, [navigate])

  const handleKeepSeparate = useCallback(async () => {
    if (!session) return
    setUnlinking(true)
    setError(null)
    try {
      await session.spacewave.unlinkLocalSession()
      navigate({ path: '../../' })
    } catch (err) {
      const msg =
        err instanceof Error ? err.message : 'Failed to unlink session'
      setError(msg)
      setUnlinking(false)
    }
  }, [session, navigate])

  return (
    <PageWrapper
      backButton={
        <button
          onClick={handleBack}
          className="text-foreground-alt hover:text-foreground flex cursor-pointer items-center gap-2 text-sm transition-colors"
        >
          <LuArrowLeft className="h-4 w-4" />
          Back
        </button>
      }
    >
      {/* Header */}
      <div className="flex flex-col items-center gap-2">
        <AnimatedLogo followMouse={false} />
        <h1 className="mt-2 text-xl font-bold tracking-wide">
          You have local data
        </h1>
        <p className="text-foreground-alt text-center text-sm">
          Your subscription is active. Choose what to do with your existing
          local session data.
        </p>
      </div>

      {/* Migrate card */}
      <button
        onClick={handleMigrate}
        disabled={unlinking}
        className={cn(
          'border-brand/30 bg-background-card/50 hover:border-brand/50 hover:shadow-brand/5 group cursor-pointer overflow-hidden rounded-lg border p-6 text-left backdrop-blur-sm transition-all duration-300 hover:shadow-md',
          'disabled:cursor-not-allowed disabled:opacity-50',
        )}
      >
        <div className="flex items-start gap-4">
          <div className="bg-brand/10 flex h-10 w-10 shrink-0 items-center justify-center rounded-lg">
            <LuCloud className="text-brand h-5 w-5" />
          </div>
          <div className="flex-1">
            <h2 className="text-foreground text-sm font-bold">
              Migrate my data to Cloud
            </h2>
            <p className="text-foreground-alt mt-1 text-xs leading-relaxed">
              Move your local files and settings to Spacewave Cloud. Your data
              will be synced across all your devices.
            </p>
          </div>
          <LuArrowRight className="text-foreground-alt group-hover:text-brand mt-0.5 h-4 w-4 shrink-0 transition-colors" />
        </div>
      </button>

      {/* Keep separate card */}
      <button
        onClick={() => void handleKeepSeparate()}
        disabled={unlinking || !session}
        className={cn(
          'border-foreground/10 bg-background-card/30 hover:border-foreground/20 group cursor-pointer overflow-hidden rounded-lg border p-6 text-left backdrop-blur-sm transition-all duration-300 hover:shadow-md',
          'disabled:cursor-not-allowed disabled:opacity-50',
        )}
      >
        <div className="flex items-start gap-4">
          <div className="bg-foreground/5 flex h-10 w-10 shrink-0 items-center justify-center rounded-lg">
            {unlinking ?
              <Spinner size="md" className="text-foreground-alt" />
            : <LuUnlink className="text-foreground-alt h-5 w-5" />}
          </div>
          <div className="flex-1">
            <h2 className="text-foreground text-sm font-bold">
              Keep sessions separate
            </h2>
            <p className="text-foreground-alt mt-1 text-xs leading-relaxed">
              Unlink the local session from your cloud account. Both sessions
              remain accessible independently.
            </p>
          </div>
          <LuHardDrive className="text-foreground-alt group-hover:text-foreground mt-0.5 h-4 w-4 shrink-0 transition-colors" />
        </div>
      </button>

      {error && <p className="text-destructive text-center text-xs">{error}</p>}

      <FaqAccordion items={CLOUD_FAQ} />

      <PageFooter />
    </PageWrapper>
  )
}
