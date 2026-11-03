import React from 'react'
import { RxChevronRight } from '@s4wave/web/images/icons.js'
import { BottomBarItem } from './bottom-bar-item.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { paddingInline: 0 },
  rule2: { display: 'flex', alignItems: 'center' },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    opacity: 0.5,
  },
})

export function BottomBarBreadcrumbSeparator({
  sx,
  onClick,
}: {
  onClick?: () => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  if (onClick) {
    return (
      <BottomBarItem onClick={onClick} sx={[stylexStyles.rule1, sx]}>
        <RxChevronRight aria-hidden="true" />
      </BottomBarItem>
    )
  }

  return (
    <div sx={[stylexStyles.rule2, sx]}>
      <RxChevronRight sx={stylexStyles.rule3} aria-hidden="true" />
    </div>
  )
}
