import { useCallback } from 'react'
import * as stylex from '@stylexjs/stylex'

import {
  colors,
  fonts,
  layout,
  radii,
  motion,
} from '../../../web/style/tokens.stylex.js'

const styles = stylex.create({
  chip: {
    color: `color-mix(in oklab, ${colors['--color-foreground-alt']} 70%, transparent)`,
    position: 'relative',
    zIndex: 10,
    cursor: 'pointer',
    borderRadius: radii['--radius-md'],
    borderStyle: 'solid',
    borderWidth: 1,
    borderColor: `color-mix(in oklab, ${colors['--color-white']} 8%, transparent)`,
    paddingInline: `calc(${layout['--spacing']} * 2)`,
    paddingBlock: `calc(${layout['--spacing']} * 0.5)`,
    fontSize: fonts['--text-xs'],
    lineHeight: fonts['--text-xs--line-height'],
    fontWeight: fonts['--font-weight-medium'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionDuration: '200ms',
    transitionTimingFunction: motion['--default-transition-timing-function'],
    '@media (hover: hover)': {
      ':hover': {
        color: colors['--color-brand'],
        borderColor: `color-mix(in oklab, ${colors['--color-brand']} 30%, transparent)`,
        backgroundColor: `color-mix(in oklab, ${colors['--color-brand']} 5%, transparent)`,
      },
    },
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
