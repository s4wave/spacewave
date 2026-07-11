import { cn } from '@s4wave/web/style/utils.js'

const CANVAS_COLOR_SWATCHES = [
  '#2563eb',
  '#dc2626',
  '#16a34a',
  '#ca8a04',
  '#9333ea',
  '#0f172a',
]

interface CanvasColorPickerProps {
  color: string
  onColorChange: (color: string) => void
}

// CanvasColorPicker selects the persisted color for drawings and shapes.
export function CanvasColorPicker({
  color,
  onColorChange,
}: CanvasColorPickerProps) {
  return (
    <div
      className="border-foreground/6 flex flex-col items-center gap-1 border-y py-1.5"
      aria-label="Drawing color"
    >
      {CANVAS_COLOR_SWATCHES.map((swatch) => (
        <button
          key={swatch}
          type="button"
          aria-label={`Use color ${swatch}`}
          aria-pressed={color === swatch}
          className={cn(
            'size-4 rounded-full border transition-transform',
            color === swatch
              ? 'border-foreground scale-110'
              : 'border-foreground/20 hover:scale-110',
          )}
          style={{ backgroundColor: swatch }}
          onClick={() => onColorChange(swatch)}
        />
      ))}
      <label
        className="border-foreground/20 relative size-5 overflow-hidden rounded-md border"
        title="Custom drawing color"
      >
        <span className="sr-only">Custom drawing color</span>
        <input
          type="color"
          value={color}
          aria-label="Custom drawing color"
          className="absolute -inset-2 size-9 cursor-pointer border-0 bg-transparent p-0"
          onChange={(event) => onColorChange(event.target.value)}
        />
      </label>
    </div>
  )
}
