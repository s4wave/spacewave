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
  onClick,
}: {
  onClick?: () => void
}) {
  if (onClick) {
    return (
      <BottomBarItem onClick={onClick} {...stylex.props(stylexStyles.rule1)}>
        <RxChevronRight aria-hidden="true" />
      </BottomBarItem>
    )
  }

  return (
    <div sx={stylexStyles.rule2}>
      <RxChevronRight
        {...stylex.props(stylexStyles.rule3)}
        aria-hidden="true"
      />
    </div>
  )
}
