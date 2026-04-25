import { useCallback } from 'react'
import { LuArrowRight, LuCloud, LuHardDrive } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useLocalSessionOnboardingContext } from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'

// ProviderSetup renders the provider choice step of the setup wizard.
// Users select between Spacewave Cloud (paid, multi-device) and Local Only (free, browser-only).
export function ProviderSetup() {
  const navigate = useNavigate()
  const onboarding = useLocalSessionOnboardingContext()

  const handleLocal = useCallback(() => {
    onboarding.markProviderChoiceComplete()
    navigate({ path: '../' })
  }, [onboarding, navigate])

  const handleCloud = useCallback(() => {
    navigate({ path: '../../plan' })
  }, [navigate])

  return (
    <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center gap-6 overflow-y-auto p-6 outline-none md:p-10">
      <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />

      <div className="relative z-10 flex w-full max-w-md flex-col gap-6">
        <div className="flex flex-col items-center gap-2">
          <AnimatedLogo followMouse={false} />
          <h1 className="text-xl font-bold tracking-wide">
            Choose your provider
          </h1>
          <p className="text-foreground-alt text-sm">
            How would you like to store your data?
          </p>
        </div>

        <div className="border-foreground/20 bg-background-get-started relative overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
          <div className="space-y-3 p-6">
            <RadioOption
              selected={false}
              onSelect={handleCloud}
              icon={<LuCloud className="h-4 w-4" />}
              label="Spacewave Cloud"
              tag="$8/mo"
              description="Cloud backup, multi-device sync, encrypted storage."
            />

            <button
              onClick={handleLocal}
              className={cn(
                'group mt-2 w-full rounded-md border transition-all duration-300',
                'border-brand/30 bg-brand/10 hover:bg-brand/20',
                'flex h-10 items-center justify-center gap-2',
              )}
            >
              <LuHardDrive className="text-foreground-alt h-4 w-4" />
              <span className="text-foreground text-sm">
                Continue with local storage
              </span>
              <span className="text-foreground-alt text-xs">(Free)</span>
              <LuArrowRight className="text-foreground-alt h-4 w-4" />
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
