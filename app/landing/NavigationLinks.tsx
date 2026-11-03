import React from 'react'
import { NavigationLink, type NavigationItem } from './NavigationLink.js'
import { isDesktop } from '@aptre/bldr'
import { useNavLinks } from '@s4wave/app/nav-links.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: themeLayout['--spacing'],
  },
  rule2: {
    display: 'flex',
    width: '100%',
    flexDirection: 'row',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'center',
    columnGap: {
      default: themeLayout['--spacing'],
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 2)`,
    },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
})

// NavigationLinks renders the top navigation link bar.
export const NavigationLinks: React.FC = () => {
  const nav = useNavLinks()

  const navigationItems: (NavigationItem | false)[] = [
    !isDesktop && { text: 'Download app', onClick: nav.download },
    { text: 'Docs', onClick: nav.docs },
    { text: 'Blog', onClick: nav.blog },
    { text: 'Changelog', onClick: nav.changelog },
    { text: 'Cloud', onClick: nav.cloud },
    { text: 'Support', onClick: nav.support },
  ]

  return (
    <div sx={stylexStyles.rule1}>
      <nav sx={stylexStyles.rule2}>
        {navigationItems.map((item) =>
          item ? (
            <NavigationLink key={item.text} {...item} sx={stylexStyles.rule3} />
          ) : null,
        )}
      </nav>
    </div>
  )
}
