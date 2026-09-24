import type { ReactNode, Ref } from 'react'
import { LuMaximize2 } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import { toneDotClass, toneLabel } from './tone.js'
import type { SystemTone } from './useSystemModel.js'

// SystemTile is one live subsystem tile of the system dashboard. Its title
// button stretches over the whole tile, so a click anywhere opens the
// subsystem inspector while the readouts stay ordinary markup.
export function SystemTile({
  title,
  icon,
  tone,
  headline,
  className,
  buttonRef,
  onOpen,
  children,
}: {
  title: string
  icon: ReactNode
  tone: SystemTone
  headline: ReactNode
  className?: string
  buttonRef?: Ref<HTMLButtonElement>
  onOpen: () => void
  children?: ReactNode
}) {
  return (
    <section
      className={cn(
        'group border-foreground/8 bg-background-card/30 relative flex min-h-36 flex-col rounded-lg border p-4 transition-colors duration-150',
        'hover:border-foreground/15 hover:bg-background-card/60',
        'has-[button:focus-visible]:ring-brand/50 has-[button:focus-visible]:ring-2',
        className,
      )}
    >
      <h2 className="text-foreground-alt/60 flex items-center gap-2">
        <span className="shrink-0 [&>svg]:size-3.5" aria-hidden="true">
          {icon}
        </span>
        <button
          ref={buttonRef}
          type="button"
          onClick={onOpen}
          className="text-metadata font-semibold tracking-wider uppercase outline-none after:absolute after:inset-0 after:rounded-lg"
        >
          {title}
          <span className="sr-only">, {toneLabel[tone]}. Open inspector.</span>
        </button>
        <span
          className={cn('size-1.5 rounded-full', toneDotClass[tone])}
          aria-hidden="true"
        />
        <LuMaximize2
          className="text-foreground-alt/50 ml-auto size-3.5 opacity-0 transition-opacity group-hover:opacity-100 group-has-[button:focus-visible]:opacity-100"
          aria-hidden="true"
        />
      </h2>

      <div className="text-foreground mt-3 text-lg font-semibold tracking-tight">
        {headline}
      </div>

      {children && (
        <div className="text-foreground-alt/70 mt-auto pt-3 text-xs">
          {children}
        </div>
      )}
    </section>
  )
}
