import { cn } from '@s4wave/web/style/utils.js'

export interface RadioOptionProps {
  selected: boolean
  onSelect: () => void
  icon?: React.ReactNode
  label: React.ReactNode
  tag?: React.ReactNode
  description?: React.ReactNode
  className?: string
}

// RadioOption renders a selectable radio button card with optional icon and tag.
function RadioOption({
  selected,
  onSelect,
  icon,
  label,
  tag,
  description,
  className,
}: RadioOptionProps) {
  return (
    <button
      onClick={onSelect}
      className={cn(
        'w-full rounded-md border p-2.5 text-left transition-all duration-200',
        selected ?
          'border-brand/30 bg-brand/5'
        : 'border-foreground/10 bg-background/20 hover:border-foreground/20',
        className,
      )}
    >
      <div className="flex items-center gap-3">
        <div
          className={cn(
            'flex h-4 w-4 shrink-0 items-center justify-center rounded-full border-2',
            selected ? 'border-brand' : 'border-foreground/30',
          )}
        >
          {selected && <div className="bg-brand h-2 w-2 rounded-full" />}
        </div>
        {icon && (
          <div className={cn('text-foreground-alt', selected && 'text-brand')}>
            {icon}
          </div>
        )}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <p className="text-foreground text-xs font-medium">{label}</p>
            {tag && (
              <span
                className={cn(
                  'rounded px-1.5 py-0.5 text-[10px] font-medium',
                  selected ?
                    'bg-brand/20 text-brand'
                  : 'bg-foreground/10 text-foreground-alt',
                )}
              >
                {tag}
              </span>
            )}
          </div>
          {description && (
            <p className="text-foreground-alt text-xs">{description}</p>
          )}
        </div>
      </div>
    </button>
  )
}

export { RadioOption }
