import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { EntityKeypairState } from '@s4wave/sdk/account/account.pb.js'

// UseEntityKeypairsResult is the return type of useEntityKeypairs.
export interface UseEntityKeypairsResult {
  keypairs: EntityKeypairState[]
  unlockedCount: number
  loading: boolean
}

// useEntityKeypairs subscribes to the entity keypairs stream and returns their lock state.
export function useEntityKeypairs(
  account: Resource<Account>,
): UseEntityKeypairsResult {
  const resource = useStreamingResource(
    account,
    (acc, signal) => acc.watchEntityKeypairs({}, signal),
    [],
  )

  return buildEntityKeypairsResult(
    resource.value?.keypairs ?? [],
    resource.value?.unlockedCount ?? 0,
    resource.loading,
  )
}

function buildEntityKeypairsResult(
  keypairs: EntityKeypairState[],
  unlockedCount: number,
  loading: boolean,
): UseEntityKeypairsResult {
  return {
    keypairs,
    unlockedCount,
    loading,
  }
}
