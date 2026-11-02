import { useCallback } from 'react'
import { LuCircleAlert } from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      },
    },
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-destructive'],
  },
})

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
      <div className="relative z-10 flex w-full max-w-sm flex-col gap-6">
        <div className="flex flex-col items-center gap-2">
          <AnimatedLogo followMouse={false} />
        </div>

        <div className="border-foreground/20 bg-background-get-started relative overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
          <div className="space-y-4 p-6">
            <div className="flex flex-col items-center gap-2">
              <div className="bg-destructive/10 flex size-12 items-center justify-center rounded-full">
                <LuCircleAlert sx={stylexStyles.rule1} />
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
              type="button"
              onClick={handleRemoveClick}
              sx={[[stylexStyles2.rule1, groupMarker]]}
            >
              <span className="text-foreground text-sm">Remove Session</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
