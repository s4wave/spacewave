import { LuSettings, LuShieldCheck } from '@s4wave/web/images/icons.js'

import type { StorageHealthView } from '@s4wave/app/session/storage/useStorageHealth.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import { Facts } from './Facts.js'
import { InspectorSection } from './InspectorSection.js'
import { StorageMeter } from './StorageMeter.js'
import { formatBytes, formatCount, protectionLabel } from './format.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 48rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule2: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// StorageInspector shows the local block store, the browser quota and
// cleanup protection, and links to the storage settings page.
export function StorageInspector({
  storage,
  onOpenSettings,
}: {
  storage: StorageHealthView
  onOpenSettings: () => void
}) {
  return (
    <div sx={stylexStyles2.rule1}>
      <InspectorSection
        title="This device's store"
        description="Spacewave's local block store. Entries measure storage, not a verified backup."
        action={
          <DashboardButton
            icon={<LuSettings sx={stylexStyles.rule1} />}
            onClick={onOpenSettings}
          >
            Storage settings
          </DashboardButton>
        }
      >
        <Facts
          empty="The local store did not provide a reading."
          facts={
            storage.providerLoading
              ? [{ label: 'Status', value: 'Reading the local store' }]
              : storage.providerSupported
                ? [
                    {
                      label: 'Physical size',
                      value: formatBytes(storage.providerBytes),
                      mono: true,
                    },
                    {
                      label: 'Block entries',
                      value: formatCount(storage.blockCount),
                      mono: true,
                    },
                  ]
                : []
          }
        />
      </InspectorSection>

      <InspectorSection
        title="This browser"
        description="The browser's estimate covers all Spacewave data in this origin."
        action={
          storage.protectionState === 'not-protected' && (
            <DashboardButton
              icon={<LuShieldCheck sx={stylexStyles.rule1} />}
              onClick={() => {
                void storage.requestProtection()
              }}
            >
              Request protection
            </DashboardButton>
          )
        }
      >
        <StorageMeter
          usage={storage.originUsageBytes}
          quota={storage.originQuotaBytes}
        />
        <div sx={stylexStyles2.rule2}>
          <Facts
            facts={[
              {
                label: 'Usage estimate',
                value:
                  storage.originUsageBytes == null
                    ? storage.browserReadFailed
                      ? 'Not provided by the browser'
                      : null
                    : formatBytes(storage.originUsageBytes),
                mono: storage.originUsageBytes != null,
              },
              {
                label: 'Quota',
                value:
                  storage.originQuotaBytes == null
                    ? null
                    : formatBytes(storage.originQuotaBytes),
                mono: true,
              },
              {
                label: 'Cleanup protection',
                value: protectionLabel(storage.protectionState),
              },
              {
                label: 'Safari cleanup',
                value: storage.safariCleanupRisk
                  ? 'Safari may remove local data after seven days without a visit'
                  : null,
                tone: 'warning',
              },
            ]}
          />
        </div>
      </InspectorSection>
    </div>
  )
}
