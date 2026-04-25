import { type ReactNode } from 'react'
import { cn } from '../style/utils.js'

interface MenuButtonGroupProps {
  children: ReactNode
  className?: string
}

export function MenuButtonGroup({ children, className }: MenuButtonGroupProps) {
  return <div className={cn('flex gap-px', className)}>{children}</div>
}
