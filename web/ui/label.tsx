import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import * as LabelPrimitive from '@radix-ui/react-label'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'
import { groupMarker, peerMarker } from '../style/relations.stylex.js'

const styles = stylex.create({
  label: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-leading': '1',
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    cursor: {
      default: null,
      [stylex.when.siblingBefore(':is(:disabled)', peerMarker)]: 'not-allowed',
    },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 1,
    opacity: {
      default: null,
      [stylex.when.ancestor(':is([data-disabled="true"])', groupMarker)]: 0.5,
      [stylex.when.siblingBefore(':is(:disabled)', peerMarker)]: 0.5,
    },
    pointerEvents: {
      default: null,
      [stylex.when.ancestor(':is([data-disabled="true"])', groupMarker)]:
        'none',
    },
    userSelect: 'none',
  },
})

function Label({
  sx,
  ...props
}: Omit<React.ComponentProps<typeof LabelPrimitive.Root>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <LabelPrimitive.Root
      data-slot="label"
      {...stylexProps([styles.label, sx], props)}
    />
  )
}

export { Label }
