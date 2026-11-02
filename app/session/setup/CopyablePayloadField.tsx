import { useState } from 'react'
import { LuCopy } from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: { color: themeColors['--color-brand'] },
  rule3: { color: themeColors['--color-foreground-alt'] },
})

export interface CopyablePayloadFieldProps {
  value: string
  label: string
}

// CopyablePayloadField shows a pairing payload in a select-all field with a
// button that copies it to the clipboard.
export function CopyablePayloadField({
  value,
  label,
}: CopyablePayloadFieldProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    void navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="flex w-full items-center gap-2">
      <input
        readOnly
        value={value}
        aria-label={label}
        className={cn(
          'border-foreground/20 bg-foreground/5 text-foreground flex-1 rounded-md border px-2 py-1.5 font-mono text-xs',
          'select-all focus:outline-none',
        )}
        onClick={(e) => e.currentTarget.select()}
      />
      <button
        type="button"
        onClick={handleCopy}
        className={cn(
          'rounded-md border px-2 py-1.5 transition-all duration-300',
          'border-foreground/20 hover:border-foreground/40',
        )}
        title="Copy to clipboard"
      >
        <LuCopy
          sx={[
            stylexStyles.rule1,
            copied ? stylexStyles.rule2 : stylexStyles.rule3,
          ]}
        />
      </button>
    </div>
  )
}
