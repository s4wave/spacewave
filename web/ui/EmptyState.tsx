import { ReactNode } from 'react'
import { LuFolderOpen, LuPlus } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from './button.js'

interface EmptyStateProps {
  icon?: ReactNode
  title: string
  description?: string
  action?: {
    label: string
    onClick: () => void
  }
  className?: string
  variant?: 'default' | 'compact'
}

// EmptyState renders a placeholder for empty lists or panels.
export function EmptyState({
  icon,
  title,
  description,
  action,
  className,
  variant = 'default',
}: EmptyStateProps) {
  const isCompact = variant === 'compact'

  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center text-center',
        isCompact ? 'p-4' : 'p-8',
        className,
      )}
    >
      <div
        className={cn(
          'bg-muted/50 flex items-center justify-center rounded-full',
          isCompact ? 'size-10' : 'size-14',
        )}
      >
        {icon ?? (
          <LuFolderOpen
            className={cn(
              'text-foreground-alt',
              isCompact ? 'size-5' : 'size-7',
            )}
          />
        )}
      </div>
      <h3
        className={cn(
          'text-foreground font-medium',
          isCompact ? 'mt-2 text-sm' : 'mt-4 text-base',
        )}
      >
        {title}
      </h3>
      {description && (
        <p
          className={cn(
            'text-foreground-alt',
            isCompact ? 'mt-1 text-xs' : 'mt-2 text-sm',
          )}
        >
          {description}
        </p>
      )}
      {action && (
        <Button
          variant="outline"
          size={isCompact ? 'sm' : 'default'}
          onClick={action.onClick}
          className={isCompact ? 'mt-3' : 'mt-4'}
        >
          <LuPlus className="mr-1.5 size-4" />
          {action.label}
        </Button>
      )}
    </div>
  )
}
