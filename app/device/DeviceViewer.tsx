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

const stylexStyles3 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule4: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: 1,
    overflow: 'auto',
    minHeight: 0,
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 5)`,
    marginInline: 'auto',
    display: 'flex',
    flexDirection: 'column',
    maxWidth: 'var(--container-4xl)',
    width: '100%',
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'repeat(2, minmax(0, 1fr))',
      '@media (min-width: 64rem)': 'repeat(4, minmax(0, 1fr))',
    },
  },
  rule7: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 64rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule8: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: themeColors['--color-background-secondary'],
  },
  rule9: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
  },
  rule10: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    wordBreak: 'break-all',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule11: {
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule12: {
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule13: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule14: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule15: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule16: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule17: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: themeColors['--color-background-secondary'],
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'auto minmax(0, 1fr)',
    },
  },
  rule18: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule19: {
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule20: {
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule21: {
    gridColumn: {
      default: null,
      '@media (min-width: 40rem)': 'span 2 / span 2',
    },
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule22: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
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
                <div sx={stylexStyles3.rule1}>
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
