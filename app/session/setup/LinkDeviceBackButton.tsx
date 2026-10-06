import { LuArrowLeft } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

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
      <LuArrowLeft className="text-foreground-alt size-4" />
    </button>
  )
}
