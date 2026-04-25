import { useCallback } from 'react'
import { LuCircleAlert } from 'react-icons/lu'

import { useNavigate } from '@s4wave/web/router/router.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { cn } from '@s4wave/web/style/utils.js'
import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'

export interface DeletedAccountOverlayProps {
  metadata: SessionMetadata
  onRemove: () => Promise<void>
}

// DeletedAccountOverlay renders a full-page error screen for sessions whose cloud account was deleted.
export function DeletedAccountOverlay({
  metadata,
  onRemove,
}: DeletedAccountOverlayProps) {
  const navigate = useNavigate()

  const handleBack = useCallback(() => {
    navigate({ path: '/sessions' })
  }, [navigate])

  const handleRemove = useCallback(async () => {
    await onRemove()
    navigate({ path: '/sessions' })
  }, [onRemove, navigate])

  const handleRemoveClick = useCallback(() => {
    void handleRemove()
  }, [handleRemove])

  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center gap-6 overflow-y-auto p-6 outline-none md:p-10">
      <BackButton floating onClick={handleBack}>
        Sessions
      </BackButton>

      <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />

      <div className="relative z-10 flex w-full max-w-sm flex-col gap-6">
        <div className="flex flex-col items-center gap-2">
          <AnimatedLogo followMouse={false} />
        </div>

        <div className="border-foreground/20 bg-background-get-started relative overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
          <div className="space-y-4 p-6">
            <div className="flex flex-col items-center gap-2">
              <div className="bg-destructive/10 flex h-12 w-12 items-center justify-center rounded-full">
                <LuCircleAlert className="text-destructive h-6 w-6" />
              </div>
              <h2 className="text-foreground text-lg font-medium">
                {metadata.displayName || 'Deleted Session'}
              </h2>
              {metadata.providerDisplayName && (
                <p className="text-foreground-alt text-xs">
                  {metadata.providerDisplayName}
                </p>
              )}
            </div>

            <p className="text-foreground-alt text-center text-sm leading-relaxed">
              This account no longer exists on the cloud. The session cannot
              connect. You can remove it from your session list.
            </p>

            <button
              onClick={handleRemoveClick}
              className={cn(
                'group w-full rounded-md border transition-all duration-300',
                'border-destructive/30 bg-destructive/10 hover:bg-destructive/20',
                'flex h-10 items-center justify-center gap-2',
              )}
            >
              <span className="text-foreground text-sm">Remove Session</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
