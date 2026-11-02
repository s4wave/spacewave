import { LuCircleCheck, LuTriangleAlert } from '@s4wave/web/images/icons.js'

import { CopyButton } from '@s4wave/web/ui/CopyButton.js'

import type { StorageCheckView, StorageCorsRule } from './storage-backend.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule2: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-warning'],
  },
})

interface StorageCheckResultProps {
  check: StorageCheckView
  // corsRule is shown when the check suggests the bucket lacks it.
  corsRule?: StorageCorsRule
}

// StorageCheckResult shows a bucket check as a result line, or as the
// failure's cause and the change that fixes it.
export function StorageCheckResult({
  check,
  corsRule,
}: StorageCheckResultProps) {
  if (check.ok) {
    return (
      <p
        className="text-success flex items-center gap-1.5 text-xs"
        role="status"
      >
        <LuCircleCheck sx={stylexStyles.rule1} aria-hidden="true" />
        Connected. A test object was written, read, and deleted.
        <span className="text-foreground-alt/70">Holds {check.usage}.</span>
      </p>
    )
  }

  return (
    <div
      className="border-warning/20 bg-warning/5 rounded-lg border p-3"
      role="alert"
    >
      <div className="flex items-start gap-2">
        <LuTriangleAlert sx={stylexStyles.rule2} aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <h3 className="text-foreground text-xs font-medium">{check.title}</h3>
          <p className="text-foreground-alt/70 mt-1 text-xs leading-relaxed">
            {check.action}
          </p>

          {check.showCors && corsRule && (
            <div className="mt-2">
              <div className="flex items-center justify-between gap-2">
                <p className="text-foreground-alt/70 text-xs">
                  {corsRule.where}
                </p>
                <CopyButton text={corsRule.text} label="Copy CORS rule" />
              </div>
              <pre className="bg-background/40 border-foreground/10 text-foreground mt-1.5 max-h-48 overflow-auto rounded-md border p-2 text-xs leading-snug">
                {corsRule.text}
              </pre>
            </div>
          )}

          {check.detail && (
            <details className="mt-2">
              <summary className="text-foreground-alt hover:text-foreground cursor-pointer text-xs [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:py-3.5">
                Details
              </summary>
              <p className="text-foreground-alt/60 mt-1 text-xs break-words">
                {check.detail}
              </p>
            </details>
          )}
        </div>
      </div>
    </div>
  )
}
