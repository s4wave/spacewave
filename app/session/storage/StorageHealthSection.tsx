import { useCallback } from 'react'

import { LuDatabase } from 'react-icons/lu'

import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'

import { StorageHealth } from './StorageHealth.js'

interface StorageHealthSectionProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onNavigateToPath: (path: string) => void
}

// StorageHealthSection places the shared storage-health surface in settings home.
export function StorageHealthSection({
  open,
  onOpenChange,
  onNavigateToPath,
}: StorageHealthSectionProps) {
  const handleOpenRecovery = useCallback(() => {
    onNavigateToPath('settings/storage/recovery')
  }, [onNavigateToPath])
  const handleLinkDevice = useCallback(() => {
    onNavigateToPath('setup/link-device')
  }, [onNavigateToPath])
  const handleUseCloud = useCallback(() => {
    onNavigateToPath('plan')
  }, [onNavigateToPath])

  return (
    <CollapsibleSection
      title="Storage"
      icon={<LuDatabase className="size-3.5" />}
      open={open}
      onOpenChange={onOpenChange}
    >
      <StorageHealth
        mode="settings"
        onOpenRecovery={handleOpenRecovery}
        onLinkDevice={handleLinkDevice}
        onUseCloud={handleUseCloud}
      />
    </CollapsibleSection>
  )
}
