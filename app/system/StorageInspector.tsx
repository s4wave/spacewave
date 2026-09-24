import { LuSettings, LuShieldCheck } from 'react-icons/lu'

import type { StorageHealthView } from '@s4wave/app/session/storage/useStorageHealth.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import { Facts } from './Facts.js'
import { InspectorSection } from './InspectorSection.js'
import { StorageMeter } from './StorageMeter.js'
import { formatBytes, formatCount, protectionLabel } from './format.js'

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
    <div className="grid gap-3 @3xl:grid-cols-2">
      <InspectorSection
        title="This device's store"
        description="Spacewave's local block store. Entries measure storage, not a verified backup."
        action={
          <DashboardButton
            icon={<LuSettings className="size-3.5" />}
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
              icon={<LuShieldCheck className="size-3.5" />}
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
        <div className="mt-3">
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
