import { useCallback } from 'react'
import * as stylex from '@stylexjs/stylex'

import {
  colors,
  fonts,
  layout,
  radii,
  motion,
} from '@s4wave/web/style/tokens.stylex.js'

const styles = stylex.create({
  chip: {
    borderColor: {
      default: `color-mix(in oklab, ${colors['--color-white']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${colors['--color-brand']} 30%, transparent)`,
      },
    },
    borderRadius: radii['--radius-md'],
    borderStyle: 'solid',
    borderWidth: 1,
    paddingBlock: `calc(${layout['--spacing']} * 0.5)`,
    paddingInline: `calc(${layout['--spacing']} * 2)`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${colors['--color-brand']} 5%, transparent)`,
      },
    },
    color: {
      default: `color-mix(in oklab, ${colors['--color-foreground-alt']} 70%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': colors['--color-brand'],
      },
    },
    cursor: 'pointer',
    fontSize: fonts['--text-xs'],
    fontWeight: fonts['--font-weight-medium'],
    lineHeight: fonts['--text-xs--line-height'],
    position: 'relative',
    transitionDuration: '200ms',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: motion['--default-transition-timing-function'],
    zIndex: 10,
  },
})

/** TagChipProps supplies the tag and its selection callback. */
interface TagChipProps {
  tag: string
  onSelectTag?: (tag: string) => void
}

/** TagChip renders a clickable tag pill. */
export function TagChip({ tag, onSelectTag }: TagChipProps) {
  const handleTagSelect = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation()
      onSelectTag?.(tag)
    },
    [onSelectTag, tag],
  )

  return (
    <button type="button" onClick={handleTagSelect} sx={styles.chip}>
      {tag}
    </button>
  )
}
