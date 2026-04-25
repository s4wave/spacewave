import { useCallback } from 'react'
import { LuArrowLeft, LuCpu } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'

// MigrationPage renders a placeholder settings page for provider migration.
// Shows the current provider type and a coming-soon message.
export function MigrationPage() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const navigate = useNavigate()

  const { providerId } = useSessionInfo(session)
  const isLocal = providerId === 'local'

  const handleBack = useCallback(() => {
    navigate({ path: '../../' })
  }, [navigate])

  return (
    <div className="bg-background-landing flex flex-1 flex-col overflow-y-auto p-6 md:p-10">
      <div className="mx-auto w-full max-w-lg">
        <button
          onClick={handleBack}
          className="text-foreground-alt hover:text-foreground mb-6 flex items-center gap-1.5 text-sm transition-colors"
        >
          <LuArrowLeft className="h-4 w-4" />
          Back to dashboard
        </button>

        <div className="mb-6">
          <h1 className="text-foreground text-lg font-bold tracking-wide">
            Provider Migration
          </h1>
          <p className="text-foreground-alt mt-1 text-sm">
            Manage your data storage provider.
          </p>
        </div>

        <div className="border-foreground/20 bg-background-get-started overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
          <div className="space-y-4 p-6">
            <div className="flex items-center gap-3">
              <div className="bg-foreground/5 flex h-10 w-10 shrink-0 items-center justify-center rounded-lg">
                <LuCpu className="text-foreground-alt h-5 w-5" />
              </div>
              <div>
                <p className="text-foreground text-sm font-medium">
                  Current provider
                </p>
                <p className="text-foreground-alt text-xs">
                  {isLocal ? 'Local (browser storage)' : 'Spacewave Cloud'}
                </p>
              </div>
            </div>

            <div
              className={cn(
                'border-foreground/10 rounded-md border p-4 text-center',
              )}
            >
              <p className="text-foreground-alt text-sm">
                Provider migration is coming soon.
              </p>
              <p className="text-foreground-alt/60 mt-1 text-xs">
                You will be able to switch between Local and Spacewave Cloud
                providers here.
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
