import { useCallback, useMemo, useRef, useState, type ReactNode } from 'react'
import { isDesktop } from '@aptre/bldr'
import { useWatchStateRpc } from '@aptre/bldr-react'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { LuTerminal, LuTriangleAlert } from '@s4wave/web/images/icons.js'

import type { Root } from '@s4wave/sdk/root'
import {
  RootResourceServiceClient,
  type RootResourceService,
} from '@s4wave/sdk/root/root_srpc.pb.js'
import {
  ListenerYieldPrompt,
  RuntimeHandoffState,
  WatchListenerYieldPromptsRequest,
  WatchListenerYieldPromptsResponse,
  WatchRuntimeHandoffRequest,
  WatchRuntimeHandoffResponse,
} from '@s4wave/sdk/root/root.pb.js'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import { toast } from '@s4wave/web/ui/toaster.js'

import { RuntimeHandoffProvider } from '@s4wave/app/listener/RuntimeHandoffContext.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({ rule1: { minWidth: 0, flex: 1 } })

const stylexStyles2 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 60%, transparent)`,
  },
  rule4: {
    fontSize: '.65rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule5: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule6: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule7: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule8: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule9: {
    display: 'flex',
    width: '100%',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule10: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule11: { minWidth: 0 },
  rule12: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 80%, transparent)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule13: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule14: {
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '10px',
    lineHeight: 1.5,
  },
  rule15: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    flexShrink: 0,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-warning']} 50%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-warning']} 15%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    flexShrink: 0,
    color: themeColors['--color-warning'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-warning'],
  },
})

// ListenerYieldNotifier watches for daemon-control takeover prompts
// emitted by the native resource listener's yield broker. When a
// prompt arrives it surfaces a modal so the user can allow or deny
// the remote runtime takeover. It wraps descendants with the
// RuntimeHandoffProvider so runtime-dependent components can disable
// actions while the runtime is handed off to a remote peer.
export function ListenerYieldNotifier({
  rootResource,
  children,
}: {
  rootResource: Resource<Root>
  children?: ReactNode
}) {
  if (!isDesktop) {
    return <>{children}</>
  }
  return (
    <ListenerYieldNotifierInner rootResource={rootResource}>
      {children}
    </ListenerYieldNotifierInner>
  )
}

function ListenerYieldNotifierInner({
  rootResource,
  children,
}: {
  rootResource: Resource<Root>
  children?: ReactNode
}) {
  const root = rootResource.value
  const service: RootResourceService | null = useMemo(() => {
    if (!root) return null
    return new RootResourceServiceClient(root.client)
  }, [root])

  const watchPrompts = useCallback(
    (_: WatchListenerYieldPromptsRequest, signal: AbortSignal) => {
      if (!service) return null
      return service.WatchListenerYieldPrompts({}, signal)
    },
    [service],
  )

  const promptsResp: WatchListenerYieldPromptsResponse | null =
    useWatchStateRpc(
      watchPrompts,
      {},
      WatchListenerYieldPromptsRequest.equals,
      WatchListenerYieldPromptsResponse.equals,
    )

  const prompts = promptsResp?.prompts ?? []
  const active: ListenerYieldPrompt | null =
    prompts.length > 0 ? prompts[0] : null

  const [pendingDecision, setPendingDecision] = useState<
    null | 'allow' | 'deny'
  >(null)
  const respondedIdRef = useRef<string | null>(null)

  const respond = useCallback(
    (promptId: string, allow: boolean) => {
      if (!service) return
      respondedIdRef.current = promptId
      setPendingDecision(allow ? 'allow' : 'deny')
      service
        .RespondToListenerYieldPrompt({ promptId, allow })
        .catch((err) => {
          toast.error('Takeover response failed', { description: String(err) })
        })
        .finally(() => setPendingDecision(null))
    },
    [service],
  )

  const onOpenChange = useCallback(
    (open: boolean) => {
      if (open) return
      if (!active) return
      const id = active.promptId ?? ''
      if (respondedIdRef.current === id) return
      // Closing the dialog is treated as deny.
      respond(id, false)
    },
    [active, respond],
  )

  const handoff = useHandoffState(service)

  const requesterName = active?.requesterName?.trim() || 'unknown process'
  const socketPath = active?.socketPath || ''

  return (
    <RuntimeHandoffProvider state={handoff}>
      {handoff?.active && (
        <RuntimeHandoffBanner handoff={handoff} service={service} />
      )}

      <Dialog open={active != null} onOpenChange={onOpenChange}>
        <DialogContent showCloseButton={false}>
          <DialogHeader>
            <DialogTitle variant="withIcon">
              <LuTriangleAlert sx={stylexStyles.rule1} />
              Allow command-line takeover?
            </DialogTitle>
            <DialogDescription>
              A local process is asking to take over Spacewave's shared runtime
              socket. If you allow it, that process will act with your runtime
              authority until you reclaim it from this window. Only continue if
              you started this process yourself.
            </DialogDescription>
          </DialogHeader>

          <div sx={stylexStyles2.rule1}>
            <div sx={stylexStyles2.rule2}>
              <div sx={stylexStyles2.rule3}>
                <LuTerminal sx={stylexStyles.rule2} />
              </div>
              <div sx={stylexStyles3.rule1}>
                <div sx={[nativeChildSpacingNext.s2, false]}>
                  <p sx={[nativeChildSpacing.s0_5, stylexStyles2.rule4]}>
                    Requesting runtime
                  </p>
                  <p sx={[nativeChildSpacing.s0_5, stylexStyles2.rule5]}>
                    {requesterName}
                  </p>
                </div>
                <div sx={[nativeChildSpacingNext.s2, false]}>
                  <p sx={[nativeChildSpacing.s0_5, stylexStyles2.rule4]}>
                    Socket path
                  </p>
                  <p sx={[nativeChildSpacing.s0_5, stylexStyles2.rule6]}>
                    {socketPath}
                  </p>
                </div>
              </div>
            </div>
          </div>

          <DialogFooter>
            <button
              type="button"
              onClick={() => active && respond(active.promptId ?? '', false)}
              disabled={pendingDecision != null}
              sx={stylexStyles2.rule7}
            >
              {pendingDecision === 'deny' ? 'Denying…' : 'Deny'}
            </button>
            <button
              type="button"
              onClick={() => active && respond(active.promptId ?? '', true)}
              disabled={pendingDecision != null}
              sx={[stylexStyles2.rule8]}
            >
              {pendingDecision === 'allow' ? 'Allowing…' : 'Allow takeover'}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {children}
    </RuntimeHandoffProvider>
  )
}

// useHandoffState watches the current handoff state via the Root RPC.
function useHandoffState(
  service: RootResourceService | null,
): RuntimeHandoffState | null {
  const watchFn = useCallback(
    (_: WatchRuntimeHandoffRequest, signal: AbortSignal) => {
      if (!service) return null
      return service.WatchRuntimeHandoff({}, signal)
    },
    [service],
  )
  const resp: WatchRuntimeHandoffResponse | null = useWatchStateRpc(
    watchFn,
    {},
    WatchRuntimeHandoffRequest.equals,
    WatchRuntimeHandoffResponse.equals,
  )
  return resp?.state ?? null
}

// RuntimeHandoffBanner renders the "Runtime handed off" banner with a
// Reclaim action that re-binds the listener socket.
function RuntimeHandoffBanner({
  handoff,
  service,
}: {
  handoff: RuntimeHandoffState
  service: RootResourceService | null
}) {
  const [reclaiming, setReclaiming] = useState(false)
  const onReclaim = useCallback(() => {
    if (!service) return
    setReclaiming(true)
    service
      .ReclaimRuntime({})
      .catch((err) => {
        toast.error('Reclaim failed', { description: String(err) })
      })
      .finally(() => setReclaiming(false))
  }, [service])

  const requesterName = handoff.requesterName?.trim() || 'a local process'
  const socketPath = handoff.socketPath || ''

  return (
    <div data-slot="runtime-handoff-banner" sx={stylexStyles2.rule9}>
      <div sx={stylexStyles2.rule10}>
        <LuTriangleAlert sx={stylexStyles.rule3} />
        <div sx={stylexStyles2.rule11}>
          <p sx={stylexStyles2.rule12}>Runtime handed off</p>
          <p sx={stylexStyles2.rule13}>
            {requesterName} is running against{' '}
            <code sx={stylexStyles2.rule14}>{socketPath}</code>. Runtime actions
            are disabled until you reclaim.
          </p>
        </div>
      </div>
      <button
        type="button"
        onClick={onReclaim}
        disabled={reclaiming}
        sx={[stylexStyles2.rule15]}
      >
        {reclaiming ? 'Reclaiming…' : 'Reclaim runtime'}
      </button>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s0_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
