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

const stylexStyles3 = stylex.create({ rule1: { flex: 1, minWidth: 0 } })

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    display: 'flex',
  },
  rule3: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 60%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    WebkitUserSelect: 'none',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: '.65rem',
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
    userSelect: 'none',
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule6: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'hidden',
    color: themeColors['--color-foreground-alt'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
  },
  rule7: {
    borderRadius: themeRadii['--radius-md'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule8: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule9: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 5%, transparent)`,
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    width: '100%',
  },
  rule10: {
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'flex-start',
    display: 'flex',
    minWidth: 0,
  },
  rule11: { minWidth: 0 },
  rule12: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 80%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule13: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule14: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: themeLayout['--spacing'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: '10px',
    lineHeight: 1.5,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule15: {
    '--tw-duration': '.15s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-warning']} 50%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-warning']} 15%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    cursor: { default: null, ':disabled': 'not-allowed' },
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration: '.15s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-warning'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    color: themeColors['--color-warning'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
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
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})
