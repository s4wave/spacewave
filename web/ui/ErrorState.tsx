import { type ReactNode } from 'react'
import { LuCircleAlert, LuRefreshCw } from 'react-icons/lu'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from './button.js'

interface ErrorStateProps {
  title?: string
  message: string
  onRetry?: () => void
  className?: string
  variant?: 'inline' | 'card' | 'fullscreen'
  children?: ReactNode
}

// ErrorState renders an error message with optional retry functionality.
export function ErrorState({
  title = 'Error',
  message,
  onRetry,
  className,
  variant = 'card',
  children,
}: ErrorStateProps) {
  const content = (
    <>
      <div
        className={cn(
          'flex items-center justify-center rounded-full',
          variant === 'fullscreen' ?
            'bg-error-bg h-16 w-16'
          : 'bg-error-bg h-10 w-10',
        )}
      >
        <LuCircleAlert
          className={cn(
            'text-error',
            variant === 'fullscreen' ? 'h-8 w-8' : 'h-5 w-5',
          )}
        />
      </div>
      <div
        className={cn(
          'text-center',
          variant === 'fullscreen' ? 'mt-4' : 'mt-3',
        )}
      >
        <h3
          className={cn(
            'text-error-text font-semibold',
            variant === 'fullscreen' ? 'text-xl' : 'text-sm',
          )}
        >
          {title}
        </h3>
        <p
          className={cn(
            'text-foreground-alt mt-1',
            variant === 'fullscreen' ? 'text-sm' : 'text-xs',
          )}
        >
          {message}
        </p>
      </div>
      {onRetry && (
        <Button
          variant="outline"
          size={variant === 'fullscreen' ? 'default' : 'sm'}
          onClick={onRetry}
          className={cn(
            'border-error-border text-error hover:bg-error-bg',
            variant === 'fullscreen' ? 'mt-6' : 'mt-4',
          )}
        >
          <LuRefreshCw className="mr-2 h-4 w-4" />
          Retry
        </Button>
      )}
      {children}
    </>
  )

  if (variant === 'inline') {
    return (
      <div
        className={cn(
          'border-error-border bg-error-bg flex items-center gap-3 rounded-md border p-3',
          className,
        )}
      >
        <LuCircleAlert className="text-error h-4 w-4 shrink-0" />
        <p className="text-error-text text-sm">{message}</p>
        {onRetry && (
          <Button
            variant="ghost"
            size="sm"
            onClick={onRetry}
            className="text-error hover:bg-error-bg ml-auto shrink-0"
          >
            <LuRefreshCw className="h-3 w-3" />
          </Button>
        )}
      </div>
    )
  }

  if (variant === 'fullscreen') {
    return (
      <div
        className={cn(
          'flex min-h-screen flex-col items-center justify-center p-4',
          className,
        )}
      >
        {content}
      </div>
    )
  }

  return (
    <div
      className={cn('flex flex-col items-center justify-center p-6', className)}
    >
      {content}
    </div>
  )
}
