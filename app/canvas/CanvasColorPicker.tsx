import { controlTargetStyles } from '@s4wave/web/style/ControlStylesProvider.js'
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
    borderBlockStyle: themeUtilityState['--tw-border-style'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    borderBlockWidth: '1px',
    display: 'flex',
    flexDirection: 'column',
  },
  rule2: {
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: 'var(--canvas-swatch-color)',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    '--tw-scale-x': '110%',
    '--tw-scale-y': '110%',
    '--tw-scale-z': '110%',
    borderColor: themeColors['--color-foreground'],
    scale: 'var(--tw-scale-x) var(--tw-scale-y)',
  },
  rule4: {
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
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    scale: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-scale-x) var(--tw-scale-y)',
      },
    },
  },
  rule5: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule6: {
    margin: '-1px',
    padding: 0,
    borderWidth: 0,
    overflow: 'hidden',
    clipPath: 'inset(50%)',
    position: 'absolute',
    whiteSpace: 'nowrap',
    height: '1px',
    width: '1px',
  },
  rule7: {
    inset: `calc(${themeLayout['--spacing']} * -2)`,
    padding: 0,
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: 0,
    backgroundColor: '#0000',
    cursor: 'pointer',
    position: 'absolute',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    width: `calc(${themeLayout['--spacing']} * 9)`,
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
            controlTargetStyles.minimum,
          ]}
          style={{ '--canvas-swatch-color': swatch }}
          onClick={() => onColorChange(swatch)}
        />
      ))}
      <label
        sx={[stylexStyles.rule5, controlTargetStyles.minimum]}
        title="Custom drawing color"
      >
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
