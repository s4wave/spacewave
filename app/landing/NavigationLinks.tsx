import React from 'react'
import { NavigationLink, type NavigationItem } from './NavigationLink.js'
import { isDesktop } from '@aptre/bldr'
import { useNavLinks } from '@s4wave/app/nav-links.js'

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
    <div className="relative flex flex-col items-center px-4 py-1">
      <nav className="flex w-full flex-row flex-wrap items-center justify-center gap-x-4 gap-y-1 text-sm @lg:gap-x-4 @lg:gap-y-2">
        {navigationItems.map((item) =>
          item ? <NavigationLink key={item.text} {...item} /> : null,
        )}
      </nav>
    </div>
  )
}
