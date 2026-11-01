import { cn } from '@s4wave/web/style/utils.js'

// Figure is one number in a FigureStrip.
export interface Figure {
  label: string
  value: string | number
  // unit follows the value in a smaller muted size, such as "of 4".
  unit?: string
  // detail is one line of context under the value.
  detail?: string
}

// FigureStripProps are the props for FigureStrip.
export interface FigureStripProps {
  figures: Figure[]
  className?: string
}

// FigureStrip shows a few figures side by side in one panel, each with a
// capital label, the value, and an optional line of context.
export function FigureStrip({ figures, className }: FigureStripProps) {
  return (
    <div
      className={cn(
        'border-foreground/6 bg-background-card/30 divide-foreground/6 grid auto-cols-fr grid-flow-col divide-x rounded-lg border',
        className,
      )}
    >
      {figures.map((figure) => (
        <div key={figure.label} className="min-w-0 px-3.5 py-3">
          <div className="text-foreground-alt micro-ten truncate font-semibold tracking-widest uppercase select-none">
            {figure.label}
          </div>
          <div className="text-foreground mt-1.5 text-2xl leading-tight font-medium tabular-nums select-none">
            {figure.value}
            {figure.unit && (
              <span className="text-foreground-alt ml-1 text-sm font-normal">
                {figure.unit}
              </span>
            )}
          </div>
          {figure.detail && (
            <div className="text-foreground-alt/70 mt-1 truncate text-xs select-none">
              {figure.detail}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}
