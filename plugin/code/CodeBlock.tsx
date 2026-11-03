import * as ownerStylex from '@stylexjs/stylex'
import { codeGroupMarker as ownercodeGroupMarker } from '@s4wave/web/style/relations.stylex.js'
import { useCallback, useEffect, useRef, useState } from 'react'
import { LuCheck, LuCopy } from '@s4wave/web/images/icons.js'

import { usePromise } from '@s4wave/web/hooks/usePromise.js'

import { highlightCode } from './highlight.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '@s4wave/web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  motion as themeMotion,
  colors as themeColors,
} from '@s4wave/web/style/tokens.stylex.js'
import { codeGroupMarker as codeGroupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles2 = ownerStylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    opacity: {
      default: 0,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', codeGroupMarker3)]: 1,
      },
    },
    position: 'absolute',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    zIndex: 10,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    right: `calc(${themeLayout['--spacing']} * 2.5)`,
    top: `calc(${themeLayout['--spacing']} * 2.5)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule3: {
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      },
    },
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// CodeBlockProps are the props for CodeBlock.
export interface CodeBlockProps {
  code: string
  language?: string
  sx?: ownerStylex.StyleXStyles
  className?: never
}

/**
 * CodeBlock renders highlighted code with a copy button. It shows plain text
 * while the grammar loads and when Shiki does not know the language.
 */
export function CodeBlock({ code, language, sx }: CodeBlockProps) {
  const { data: html } = usePromise(
    useCallback(
      () => highlightCode(code, language || 'text'),
      [code, language],
    ),
  )

  const [copied, setCopied] = useState(false)
  const copyTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => {
    return () => clearTimeout(copyTimer.current)
  }, [])

  const handleCopy = useCallback(() => {
    void navigator.clipboard.writeText(code)
    setCopied(true)
    clearTimeout(copyTimer.current)
    copyTimer.current = setTimeout(() => setCopied(false), 1500)
  }, [code])

  return (
    <div sx={[codeContainerStyles.root, ownercodeGroupMarker, sx]}>
      <button
        type="button"
        onClick={handleCopy}
        sx={[
          stylexStyles2.rule1,
          copied ? stylexStyles2.rule2 : stylexStyles2.rule3,
        ]}
        title="Copy code"
      >
        {copied ? (
          <LuCheck sx={stylexStyles.rule1} />
        ) : (
          <LuCopy sx={stylexStyles.rule1} />
        )}
      </button>

      {html ? (
        <div dangerouslySetInnerHTML={{ __html: html }} />
      ) : (
        <pre>
          <code>{code}</code>
        </pre>
      )}
    </div>
  )
}

// codeContainerStyles owns the code group positioned around its copy action.
const codeContainerStyles = ownerStylex.create({
  root: { position: 'relative' },
})
