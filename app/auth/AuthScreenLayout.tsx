import type React from 'react'
import { forwardRef } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import spacewaveIcon from '@s4wave/web/images/spacewave-icon.png'

export interface AuthScreenLayoutProps extends React.ComponentPropsWithoutRef<'div'> {
  intro: React.ReactNode
  topLeft?: React.ReactNode
  topRight?: React.ReactNode
  alwaysShowIntro?: boolean
  introClassName?: string
  contentClassName?: string
  shellClassName?: string
}

// AuthScreenLayout renders the shared auth page shell with container-aware
// intro and form placement.
export const AuthScreenLayout = forwardRef<
  HTMLDivElement,
  AuthScreenLayoutProps
>(function AuthScreenLayout(
  {
    intro,
    topLeft,
    topRight,
    alwaysShowIntro,
    introClassName,
    contentClassName,
    shellClassName,
    children,
    className,
    ...props
  },
  ref,
) {
  return (
    <div
      ref={ref}
      className={cn(
        'bg-background-landing [container-type:size] relative flex w-full flex-1 flex-col items-center justify-center gap-6 overflow-y-auto outline-none',
        'auth-very-short:justify-start',
        className,
      )}
      {...props}
    >
      <ShootingStars className="pointer-events-none fixed inset-0 opacity-60" />

      {topLeft && <div className="absolute top-4 left-4 z-20">{topLeft}</div>}
      {topRight && (
        <div className="absolute top-4 right-4 z-20">{topRight}</div>
      )}

      <div
        className={cn(
          'relative z-10 flex w-full max-w-5xl flex-col items-center gap-6',
          'auth-short:min-h-full auth-short:w-full auth-short:max-w-none auth-short:justify-center',
          'auth-very-short:justify-start auth-very-short:pt-16 auth-very-short:pb-8',
          shellClassName,
        )}
      >
        <div
          className={cn(
            'flex w-full max-w-sm flex-col items-center gap-3 text-center',
            !alwaysShowIntro && 'auth-short:hidden',
            !alwaysShowIntro && 'auth-short-narrow:hidden',
            introClassName,
          )}
        >
          {intro}
        </div>
        {!alwaysShowIntro && (
          <div className="auth-short:absolute auth-short:bottom-4 auth-short:left-4 auth-short:flex auth-short:items-center auth-short:gap-2 auth-short-narrow:hidden hidden">
            <img
              src={spacewaveIcon}
              alt="Spacewave Icon"
              className="h-4 w-4 rounded-sm"
            />
            <span className="text-foreground text-sm font-medium tracking-wide">
              Spacewave
            </span>
          </div>
        )}
        <div
          className={cn(
            'auth-short:mx-auto auth-short:max-w-md w-full max-w-sm shrink-0',
            contentClassName,
          )}
        >
          {children}
        </div>
      </div>
    </div>
  )
})
