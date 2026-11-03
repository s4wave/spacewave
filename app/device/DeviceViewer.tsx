import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useCallback } from 'react'
import { LuTerminal } from '@s4wave/web/images/icons.js'

import {
  DeviceCapabilityState,
  DeviceAvailabilityState,
  DeviceSetupState,
  DeviceUpdateState,
} from '@s4wave/sdk/device/device.pb.js'
import { DeviceHandle, DeviceTypeID } from '@s4wave/sdk/device/device.js'
import { CREATE_TERMINAL_OP_ID } from '@s4wave/sdk/terminal/create-terminal.js'

import {
  buildCreateTerminalOpData,
  findOpenableTerminalCapability,
} from './terminal-action.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    minHeight: 0,
    flex: 1,
    overflow: 'auto',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    marginInline: 'auto',
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule6: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'repeat(2, minmax(0, 1fr))',
      '@media (min-width: 64rem)': 'repeat(4, minmax(0, 1fr))',
    },
  },
  rule7: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 64rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule8: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: themeColors['--color-background-secondary'],
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule9: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-muted-foreground'],
    textTransform: 'uppercase',
  },
  rule10: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    wordBreak: 'break-all',
    color: themeColors['--color-foreground'],
  },
  rule11: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  rule12: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  rule13: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule14: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule15: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-muted-foreground'],
    textTransform: 'uppercase',
  },
  rule16: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  rule17: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: themeColors['--color-background-secondary'],
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'auto minmax(0, 1fr)',
    },
  },
  rule18: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule19: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  rule20: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  rule21: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-muted-foreground'],
    gridColumn: {
      default: null,
      '@media (min-width: 40rem)': 'span 2 / span 2',
    },
  },
  rule22: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export { DeviceTypeID }

export function DeviceViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  // Open the Device through the current Space's typed resource grant.
  const deviceObjectKey = getObjectKey(objectInfo)
  const { navigateToObjects, spaceWorld } = SpaceContainerContext.useContext()
  const handle = useAccessTypedHandle(
    worldState,
    deviceObjectKey,
    DeviceHandle,
    DeviceTypeID,
  )

  // Observe transport reachability and the durable record through one projection.
  const streamFactory = useCallback(
    (h: DeviceHandle, signal: AbortSignal) => h.watchDeviceAvailability(signal),
    [],
  )
  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state = stateResource.value?.device
  const capabilities = state?.capabilities ?? []
  const terminalCapability = findOpenableTerminalCapability(state)

  // Open a Terminal from the Device's declared capability.
  const handleOpenTerminal = async () => {
    if (!state || !terminalCapability) return
    try {
      const terminalOp = await buildCreateTerminalOpData({
        device: state,
        deviceObjectKey,
        world: spaceWorld,
      })
      if (!terminalOp) return
      await spaceWorld.applyWorldOp(
        CREATE_TERMINAL_OP_ID,
        terminalOp.opData,
        '',
      )
      navigateToObjects([terminalOp.objectKey])
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : 'Failed to open terminal',
      )
    }
  }

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <span sx={stylexStyles2.rule3}>Device</span>
        {terminalCapability && (
          <DashboardButton
            icon={<LuTerminal sx={stylexStyles.rule1} />}
            onClick={() => void handleOpenTerminal()}
          >
            Open Terminal
          </DashboardButton>
        )}
      </div>
      <div sx={stylexStyles2.rule4}>
        {stateResource.loading && !state && (
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading device',
              detail: 'Reading the device state stream.',
            }}
          />
        )}
        {state && (
          <div sx={stylexStyles2.rule5}>
            <section sx={stylexStyles2.rule6}>
              <DeviceField label="Name" value={state.label || 'Device'} />
              <DeviceField
                label="Liveness"
                value={formatLiveness(stateResource.value?.state)}
              />
              <DeviceField
                label="Setup"
                value={formatSetupState(state.setupState)}
              />
              <DeviceField
                label="Update"
                value={formatUpdateState(state.updateState)}
              />
            </section>

            <section sx={stylexStyles2.rule7}>
              <div sx={stylexStyles2.rule8}>
                <div sx={stylexStyles2.rule9}>Identity</div>
                <div sx={stylexStyles2.rule10}>{state.peerId || 'unknown'}</div>
                <div sx={stylexStyles2.rule11}>
                  {[state.platform?.os, state.platform?.arch]
                    .filter(Boolean)
                    .join(' / ') || 'unknown platform'}
                </div>
                {state.daemonVersion && (
                  <div sx={stylexStyles2.rule12}>{state.daemonVersion}</div>
                )}
              </div>

              <div sx={stylexStyles2.rule8}>
                <div sx={stylexStyles2.rule9}>Last Status</div>
                <div sx={stylexStyles2.rule13}>
                  {state.lastStatus?.message || 'No status reported'}
                </div>
                {state.lastStatus?.error && (
                  <div sx={stylexStyles2.rule14}>{state.lastStatus.error}</div>
                )}
              </div>
            </section>

            <section>
              <div sx={stylexStyles2.rule15}>Capabilities</div>
              {capabilities.length === 0 ? (
                <div sx={stylexStyles2.rule16}>No capabilities declared</div>
              ) : (
                <div
                  className={
                    /* TODO(stylex): a descendant, group, peer or compound selector: */ 'border-foreground/10 overflow-hidden rounded-md border'
                  }
                >
                  {capabilities.map((capability) => (
                    <div
                      key={capability.id}
                      sx={[stylexStyles2.rule17, capabilityRowStyles.color]}
                    >
                      <div>
                        <div sx={stylexStyles2.rule18}>
                          {capability.label || capability.id}
                        </div>
                        <div sx={stylexStyles2.rule19}>
                          {capability.kind || capability.id}
                        </div>
                      </div>
                      <div sx={stylexStyles2.rule20}>
                        {formatCapabilityState(capability.state)}
                      </div>
                      {capability.detail && (
                        <div sx={stylexStyles2.rule21}>{capability.detail}</div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </section>
          </div>
        )}
      </div>
    </div>
  )
}

function DeviceField({ label, value }: { label: string; value: string }) {
  return (
    <div sx={stylexStyles2.rule8}>
      <div sx={stylexStyles2.rule9}>{label}</div>
      <div sx={stylexStyles2.rule22}>{value}</div>
    </div>
  )
}

function formatSetupState(state?: DeviceSetupState): string {
  switch (state) {
    case DeviceSetupState.WAITING_FOR_COMPLETION:
      return 'Waiting'
    case DeviceSetupState.COMPLETION_IMPORTED:
      return 'Imported'
    case DeviceSetupState.DEVICE_SESSION_READY:
      return 'Ready'
    case DeviceSetupState.FAILED:
      return 'Failed'
    default:
      return 'Unknown'
  }
}

function formatUpdateState(state?: DeviceUpdateState): string {
  switch (state) {
    case DeviceUpdateState.IDLE:
      return 'Idle'
    case DeviceUpdateState.READY:
      return 'Ready'
    case DeviceUpdateState.STAGING:
      return 'Staging'
    case DeviceUpdateState.APPLYING:
      return 'Applying'
    case DeviceUpdateState.UPDATED:
      return 'Updated'
    case DeviceUpdateState.FAILED:
      return 'Failed'
    default:
      return 'Unknown'
  }
}

function formatLiveness(state?: DeviceAvailabilityState): string {
  switch (state) {
    case DeviceAvailabilityState.ONLINE:
      return 'Online'
    case DeviceAvailabilityState.OFFLINE:
      return 'Offline'
    default:
      return 'Unknown'
  }
}

function formatCapabilityState(state?: DeviceCapabilityState): string {
  switch (state) {
    case DeviceCapabilityState.DECLARED:
      return 'Declared'
    case DeviceCapabilityState.DISABLED:
      return 'Disabled'
    case DeviceCapabilityState.GRANT_BLOCKED:
      return 'Grant blocked'
    case DeviceCapabilityState.AVAILABLE:
      return 'Available'
    case DeviceCapabilityState.ACTIVE:
      return 'Active'
    case DeviceCapabilityState.REJECTED:
      return 'Rejected'
    default:
      return 'Unknown'
  }
}

// capabilityRowStyles retains the list's border tone on each nonfinal row.
const capabilityRowStyles = stylex.create({
  color: {
    borderColor: {
      default: null,
      ':not(:last-child)':
        'color-mix(in oklab, var(--color-foreground) 8%, transparent)',
    },
  },
})
