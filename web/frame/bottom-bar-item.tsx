import React, {
  CSSProperties,
  DOMAttributes,
  KeyboardEvent,
  type Ref,
} from 'react'

import { cn } from '@s4wave/web/style/utils.js'

export interface IBottomBarItemProps extends DOMAttributes<HTMLDivElement> {
  selected?: boolean
  disabled?: boolean
  children?: React.ReactNode
  className?: string
  style?: CSSProperties
  onClick?: () => void
  ref?: Ref<HTMLDivElement>
}

export function BottomBarItem({
  children,
  style,
  onClick,
  disabled,
  selected,
  className,
  ref,
  ...rest
}: IBottomBarItemProps) {
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (onClick && (event.key === 'Enter' || event.key === ' ')) {
      event.preventDefault()
      onClick()
    }
  }

  return (
    <div
      ref={ref}
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={handleKeyDown}
      aria-disabled={disabled}
      aria-selected={selected}
      {...rest}
      className={cn(
        `glow-on-hover text-bar-item-text hover:text-bar-item-text-hover relative flex h-full shrink-0 cursor-pointer flex-row items-center justify-start overflow-hidden px-[5px] whitespace-pre select-none [&>svg]:h-3 [&>svg]:w-3 [&>svg:not(:only-child)]:mr-1`,
        selected &&
          'bg-bar-item-selected text-bar-item-selected-text text-shadow-bar-item-selected border-t-primary border-t',
        className,
      )}
      style={{
        cursor: disabled ? 'not-allowed' : 'pointer',
        ...style,
      }}
    >
      {children}
    </div>
  )
}
