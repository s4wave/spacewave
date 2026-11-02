import { useState } from 'react'
import { LuPlus, LuShieldAlert } from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import { groupMarker } from '../../web/style/relations.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    textAlign: 'left',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    flexShrink: 0,
    color: themeColors['--color-brand'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// WindowsBypassNotice renders a collapsible "How to run on Windows" block
// describing how to bypass SmartScreen for the interim unsigned build.
// Collapsed by default. Keyboard accessible as a <button> disclosure.
export function WindowsBypassNotice() {
  const [isOpen, setOpen] = useState(false)

  return (
    <div
      className={cn(
        'rounded-lg border transition-all',
        isOpen
          ? 'border-foreground/12 bg-background-card/60'
          : 'border-foreground/6 bg-background-card/30 hover:border-foreground/12',
      )}
    >
      <button
        type="button"
        aria-expanded={isOpen}
        onClick={() => setOpen((v) => !v)}
        sx={[stylexStyles2.rule1, groupMarker]}
      >
        <div className="flex items-start gap-3">
          <LuShieldAlert sx={stylexStyles.rule1} />
          <h3 className="text-foreground group-hover:text-brand text-sm leading-relaxed font-semibold transition-colors @lg:text-base">
            How to run on Windows
          </h3>
        </div>
        <div
          className={cn(
            'mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-md transition-all',
            isOpen
              ? 'bg-brand/12 text-brand rotate-45'
              : 'bg-foreground/6 text-foreground-alt group-hover:bg-brand/8 group-hover:text-brand',
          )}
        >
          <LuPlus sx={stylexStyles.rule2} />
        </div>
      </button>
      <div
        className={cn(
          'grid transition-all duration-300 ease-in-out',
          isOpen
            ? 'grid-rows-expand opacity-100'
            : 'grid-rows-collapse opacity-0',
        )}
      >
        <div className="overflow-hidden">
          <div className="text-foreground-alt flex flex-col gap-3 px-5 pb-5 text-sm leading-relaxed">
            <ol className="ml-4 list-decimal space-y-2">
              <li>
                Run the downloaded{' '}
                <span className="font-mono text-xs">.exe</span>. If SmartScreen
                shows{' '}
                <strong className="text-foreground">
                  &quot;Windows protected your PC&quot;
                </strong>
                , click <strong className="text-foreground">More info</strong>,
                then <strong className="text-foreground">Run anyway</strong>.
              </li>
              <li>Spacewave installs itself, so this is only needed once.</li>
            </ol>
            <p className="text-xs">
              The Windows build is unsigned during Microsoft&apos;s code-signing
              identity validation. The SmartScreen prompt disappears once
              validation completes.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
