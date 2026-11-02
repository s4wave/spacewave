import { LuArrowLeft } from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
})

export interface LinkDeviceBackButtonProps {
  onClick: () => void
}

// LinkDeviceBackButton renders the icon-only back control that returns a link
// step to the choose step.
export function LinkDeviceBackButton({ onClick }: LinkDeviceBackButtonProps) {
  return (
    <button
      type="button"
      aria-label="Back"
      onClick={onClick}
      className={cn(
        'rounded-md border transition-all duration-300',
        'border-foreground/20 hover:border-foreground/40',
        'flex size-10 shrink-0 items-center justify-center',
      )}
    >
      <LuArrowLeft sx={stylexStyles.rule1} />
    </button>
  )
}
