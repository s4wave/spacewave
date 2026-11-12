import { LuCode } from 'react-icons/lu'

import { ReactDevControls } from '@s4wave/web/devtools/ReactDevControls.js'
import { useReactDev } from '@s4wave/web/devtools/react-dev.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'

export interface DeveloperSectionProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

// DeveloperSection holds this device's developer settings. It is absent where
// none applies, such as a shell without a service worker.
export function DeveloperSection({
  open,
  onOpenChange,
}: DeveloperSectionProps) {
  const control = useReactDev()
  if (!control) return null
  return (
    <CollapsibleSection
      title="Developer"
      icon={<LuCode className="size-3.5" />}
      open={open}
      onOpenChange={onOpenChange}
    >
      <ReactDevControls control={control} />
    </CollapsibleSection>
  )
}
