import React from 'react'
import { RxChevronRight } from 'react-icons/rx'
import { BottomBarItem } from './bottom-bar-item.js'

export function BottomBarBreadcrumbSeparator({
  onClick,
}: {
  onClick?: () => void
}) {
  if (onClick) {
    return (
      <BottomBarItem onClick={onClick} className="px-0">
        <RxChevronRight aria-hidden="true" />
      </BottomBarItem>
    )
  }

  return (
    <div className="flex items-center">
      <RxChevronRight className="h-3 w-3 opacity-50" aria-hidden="true" />
    </div>
  )
}
