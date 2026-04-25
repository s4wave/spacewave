import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import { RootContext } from '@s4wave/web/contexts/contexts.js'

// useMountAccount mounts a provider account resource from the root context.
// Returns a Resource<Account> that resolves when both providerId and accountId
// are non-empty. Pass an extra guard (e.g. isCloud) to conditionally skip.
export function useMountAccount(
  providerId: string,
  accountId: string,
  guard?: boolean,
): Resource<Account> {
  const rootResource = RootContext.useContext()
  return useResource(
    rootResource,
    async (root, signal, cleanup) => {
      if (guard === false) return null
      if (!providerId || !accountId) return null
      const provider = cleanup(await root.lookupProvider(providerId, signal))
      return cleanup(await provider.mountAccount(accountId, signal))
    },
    [providerId, accountId, guard],
  )
}
