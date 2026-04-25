import { type ReactNode } from 'react'
import { cn } from '../style/utils.js'

interface MenuButtonProps {
  children: ReactNode
  onClick?: () => void
}

export function MenuButton({ children, onClick }: MenuButtonProps) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'rounded-menu-button text-topbar-button-text text-topbar-menu text-shadow-glow',
        'hover:text-topbar-button-text-hi hover:bg-pulldown-hover',
        'flex h-5 items-center justify-center px-[4px] whitespace-nowrap transition-colors',
      )}
    >
      {children}
    </button>
  )
}
