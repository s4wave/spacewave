import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  motion as themeMotion,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderBlockStyle: themeUtilityState['--tw-border-style'],
    borderBlockWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule2: {
    backgroundColor: 'var(--canvas-swatch-color)',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: {
    '--tw-scale-x': '110%',
    '--tw-scale-y': '110%',
    '--tw-scale-z': '110%',
    scale: 'var(--tw-scale-x) var(--tw-scale-y)',
    borderColor: themeColors['--color-foreground'],
  },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    '--tw-scale-x': {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': '110%' },
    },
    '--tw-scale-y': {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': '110%' },
    },
    '--tw-scale-z': {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': '110%' },
    },
    scale: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-scale-x) var(--tw-scale-y)',
      },
    },
  },
  rule5: {
    position: 'relative',
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
  },
  rule6: {
    clipPath: 'inset(50%)',
    whiteSpace: 'nowrap',
    borderWidth: 0,
    width: '1px',
    height: '1px',
    margin: '-1px',
    padding: 0,
    position: 'absolute',
    overflow: 'hidden',
  },
  rule7: {
    position: 'absolute',
    inset: `calc(${themeLayout['--spacing']} * -2)`,
    width: `calc(${themeLayout['--spacing']} * 9)`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    cursor: 'pointer',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: 0,
    backgroundColor: '#0000',
    padding: 0,
  },
})

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
    <div sx={stylexStyles.rule1} aria-label="Drawing color">
      {CANVAS_COLOR_SWATCHES.map((swatch) => (
        <button
          key={swatch}
          type="button"
          aria-label={`Use color ${swatch}`}
          aria-pressed={color === swatch}
          sx={[
            stylexStyles.rule2,
            color === swatch ? stylexStyles.rule3 : stylexStyles.rule4,
          ]}
          style={{ '--canvas-swatch-color': swatch }}
          onClick={() => onColorChange(swatch)}
        />
      ))}
      <label sx={stylexStyles.rule5} title="Custom drawing color">
        <span sx={stylexStyles.rule6}>Custom drawing color</span>
        <input
          type="color"
          value={color}
          aria-label="Custom drawing color"
          sx={stylexStyles.rule7}
          onChange={(event) => onColorChange(event.target.value)}
        />
      </label>
    </div>
  )
}
