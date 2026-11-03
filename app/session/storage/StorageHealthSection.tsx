import { useCallback } from 'react'

import { LuDatabase } from '@s4wave/web/images/icons.js'

import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'

import { StorageBackendsSection } from './StorageBackendsSection.js'
import { StorageHealth } from './StorageHealth.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { marginTop: `calc(${themeLayout['--spacing']} * 4)` },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

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
      icon={<LuDatabase sx={stylexStyles.rule1} />}
      open={open}
      onOpenChange={onOpenChange}
    >
      <StorageHealth
        mode="settings"
        onOpenRecovery={handleOpenRecovery}
        onLinkDevice={handleLinkDevice}
        onUseCloud={handleUseCloud}
      />
      <div sx={stylexStyles2.rule1}>
        <StorageBackendsSection />
      </div>
    </CollapsibleSection>
  )
}
