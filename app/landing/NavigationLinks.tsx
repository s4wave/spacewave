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
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
  },
  rule2: {
    alignItems: 'center',
    columnGap: {
      default: themeLayout['--spacing'],
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 2)`,
    },
    display: 'flex',
    flexDirection: 'row',
    flexWrap: 'wrap',
    fontSize: themeFonts['--text-sm'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    width: '100%',
  },
  rule3: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
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
