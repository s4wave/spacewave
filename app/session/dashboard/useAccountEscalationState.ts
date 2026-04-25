import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type {
  AccountEscalationIntent,
  AccountEscalationMethod,
  AccountEscalationRequirement,
  AccountEscalationState,
  EntityKeypairState,
  WatchAccountInfoResponse,
  WatchAuthMethodsResponse,
  WatchEntityKeypairsResponse,
} from '@s4wave/sdk/account/account.pb.js'
import type { AccountAuthMethod } from '@s4wave/core/provider/spacewave/api/api.pb.js'

// UseAccountEscalationStateResult is the derived escalation-model result.
export interface UseAccountEscalationStateResult {
  state: AccountEscalationState
  loading: boolean
}

// buildAccountEscalationState derives the shared escalation model from
// account watches plus the protected-action intent.
export function buildAccountEscalationState(
  intent: AccountEscalationIntent,
  authThreshold: number,
  authMethods: AccountAuthMethod[],
  keypairs: EntityKeypairState[],
  unlockedCount: number,
): AccountEscalationState {
  const unlockedByPeerId = new Map<string, boolean>()
  for (const keypair of keypairs) {
    const peerId = keypair.keypair?.peerId ?? ''
    if (!peerId) {
      continue
    }
    unlockedByPeerId.set(peerId, keypair.unlocked ?? false)
  }

  const methods: AccountEscalationMethod[] = authMethods.map((method) => {
    const peerId = method.peerId ?? method.keypair?.peerId ?? ''
    return {
      peerId,
      kind: method.kind,
      label: method.label,
      secondaryLabel: method.secondaryLabel,
      provider: method.provider,
      unlocked: peerId ? (unlockedByPeerId.get(peerId) ?? false) : false,
    }
  })

  const requirement: AccountEscalationRequirement = {
    authThreshold,
    requiredSigners: Math.max(1, authThreshold + 1),
    unlockedSigners: unlockedCount,
    totalMethods: methods.length,
  }

  return {
    intent,
    requirement,
    methods,
  }
}

// useAccountEscalationState derives the shared escalation model from account
// watch streams for a given protected-action intent.
export function useAccountEscalationState(
  account: Resource<Account>,
  intent: AccountEscalationIntent,
): UseAccountEscalationStateResult {
  const accountInfoResource = useStreamingResource(
    account,
    (acc, signal) => acc.watchAccountInfo({}, signal),
    [],
  )
  const authMethodsResource = useStreamingResource(
    account,
    (acc, signal) => acc.watchAuthMethods({}, signal),
    [],
  )
  const entityKeypairsResource = useStreamingResource(
    account,
    (acc, signal) => acc.watchEntityKeypairs({}, signal),
    [],
  )
  const accountInfo =
    (accountInfoResource.value as
      | WatchAccountInfoResponse
      | null
      | undefined) ?? null
  const authMethods =
    (authMethodsResource.value as
      | WatchAuthMethodsResponse
      | null
      | undefined) ?? null
  const entityKeypairs =
    (entityKeypairsResource.value as
      | WatchEntityKeypairsResponse
      | null
      | undefined) ?? null

  return buildAccountEscalationStateResult(
    intent,
    accountInfo?.authThreshold ?? 0,
    authMethods?.authMethods ?? [],
    entityKeypairs?.keypairs ?? [],
    entityKeypairs?.unlockedCount ?? 0,
    accountInfoResource.loading ||
      authMethodsResource.loading ||
      entityKeypairsResource.loading,
  )
}

export function buildAccountEscalationStateResult(
  intent: AccountEscalationIntent,
  authThreshold: number,
  authMethods: AccountAuthMethod[],
  keypairs: EntityKeypairState[],
  unlockedCount: number,
  loading: boolean,
): UseAccountEscalationStateResult {
  return {
    state: buildAccountEscalationState(
      intent,
      authThreshold,
      authMethods,
      keypairs,
      unlockedCount,
    ),
    loading,
  }
}
