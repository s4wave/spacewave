import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { useCallback, type ReactNode } from 'react'
import {
  LuArrowRight,
  LuBox,
  LuCircle,
  LuImagePlus,
  LuLayoutGrid,
  LuMaximize2,
  LuMinus,
  LuMousePointer2,
  LuPencil,
  LuSquare,
  LuSquarePlus,
  LuType,
  LuZoomIn,
  LuZoomOut,
} from '@s4wave/web/images/icons.js'

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import { CanvasColorPicker } from './CanvasColorPicker.js'
import { DEFAULT_CANVAS_COLOR } from './geometry.js'
import type { CanvasAction, CanvasTool } from './types.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  motion as themeMotion,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import { utilityState as themeUtilityState } from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    padding: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-dark']} 80%, transparent)`,
    display: 'flex',
    flexDirection: 'column',
    borderRightStyle: themeUtilityState['--tw-border-style'],
    borderRightWidth: '1px',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    '--tw-duration': '.15s',
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule3: {
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      },
    },
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
  },
  rule4: {
    marginBlock: themeLayout['--spacing'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    height: '1px',
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
})

interface CanvasToolbarProps {
  className?: never
  tool: CanvasTool
  color?: string
  onToolChange: (tool: CanvasTool) => void
  onColorChange?: (color: string) => void
  actions: Record<CanvasAction, () => void>
  onAddObject?: () => void
  onAddImage?: () => void
  sx?: ComponentStyles
}

function ToolButton({
  label,
  active,
  onClick,
  children,
}: {
  label: string
  active?: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          sx={[
            stylexStyles.rule1,
            active ? stylexStyles.rule2 : stylexStyles.rule3,
          ]}
          onClick={onClick}
          aria-label={label}
        >
          {children}
        </button>
      </TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  )
}

// CanvasToolbar renders the drawing, insertion, and viewport controls.
export function CanvasToolbar({
  tool,
  color = DEFAULT_CANVAS_COLOR,
  onToolChange,
  onColorChange,
  actions,
  onAddObject,
  onAddImage,
  sx,
}: Omit<CanvasToolbarProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  const setSelect = useCallback(() => onToolChange('select'), [onToolChange])
  const setDraw = useCallback(() => onToolChange('draw'), [onToolChange])
  const setLine = useCallback(() => onToolChange('line'), [onToolChange])
  const setArrow = useCallback(() => onToolChange('arrow'), [onToolChange])
  const setRectangle = useCallback(
    () => onToolChange('rectangle'),
    [onToolChange],
  )
  const setEllipse = useCallback(() => onToolChange('ellipse'), [onToolChange])
  const setText = useCallback(() => onToolChange('text'), [onToolChange])
  const setObject = useCallback(() => onToolChange('object'), [onToolChange])
  const handleColorChange = useCallback(
    (nextColor: string) => onColorChange?.(nextColor),
    [onColorChange],
  )

  return (
    <div sx={[stylexStyles2.rule1, sx]}>
      <ToolButton
        label="Select (V)"
        active={tool === 'select'}
        onClick={setSelect}
      >
        <LuMousePointer2 size={16} />
      </ToolButton>
      <ToolButton label="Draw (D)" active={tool === 'draw'} onClick={setDraw}>
        <LuPencil size={16} />
      </ToolButton>
      <ToolButton label="Line" active={tool === 'line'} onClick={setLine}>
        <LuMinus size={16} />
      </ToolButton>
      <ToolButton label="Arrow" active={tool === 'arrow'} onClick={setArrow}>
        <LuArrowRight size={16} />
      </ToolButton>
      <ToolButton
        label="Rectangle"
        active={tool === 'rectangle'}
        onClick={setRectangle}
      >
        <LuSquare size={16} />
      </ToolButton>
      <ToolButton
        label="Ellipse"
        active={tool === 'ellipse'}
        onClick={setEllipse}
      >
        <LuCircle size={16} />
      </ToolButton>
      <ToolButton label="Text (T)" active={tool === 'text'} onClick={setText}>
        <LuType size={16} />
      </ToolButton>
      <ToolButton
        label="Object (O)"
        active={tool === 'object'}
        onClick={setObject}
      >
        <LuBox size={16} />
      </ToolButton>

      <CanvasColorPicker color={color} onColorChange={handleColorChange} />

      {onAddObject && (
        <ToolButton label="Add Existing Object" onClick={onAddObject}>
          <LuSquarePlus size={16} />
        </ToolButton>
      )}
      {onAddImage && (
        <ToolButton label="Add Image" onClick={onAddImage}>
          <LuImagePlus size={16} />
        </ToolButton>
      )}

      <div sx={stylexStyles.rule4} />

      <ToolButton label="Zoom In (+)" onClick={actions['zoom-in']}>
        <LuZoomIn size={16} />
      </ToolButton>
      <ToolButton label="Zoom Out (-)" onClick={actions['zoom-out']}>
        <LuZoomOut size={16} />
      </ToolButton>
      <ToolButton label="Fit View" onClick={actions['fit-view']}>
        <LuMaximize2 size={16} />
      </ToolButton>
      <ToolButton label="Organize Nodes" onClick={actions['organize-nodes']}>
        <LuLayoutGrid size={16} />
      </ToolButton>
    </div>
  )
}
