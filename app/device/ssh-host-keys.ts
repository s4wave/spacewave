import { SOParticipantRole } from '@s4wave/core/sobject/sobject.pb.js'
import type { SshHostEndpoint } from '@s4wave/sdk/sshhost/sshhost.pb.js'
import type { Space } from '@s4wave/sdk/space/space.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { slugObjectKeySegment } from '../space/create-op-builders.js'

// SshHostKeys are the object keys one Add Device wizard writes for one SSH
// Host the person entered: the host and its credential Secrets; its
// terminal's key is the host's with a -terminal suffix. Every tab and every
// press of that wizard derives the same keys for the same label and
// endpoint, so a second create of any of them is refused as existing
// instead of making a copy.
export interface SshHostKeys {
  host: string
  password: string
  privateKey: string
  passphrase: string
}

// deriveSshHostKeys returns the keys for the label and endpoint the person
// entered in the wizard at wizardKey. A wizard key is reused once its wizard
// is gone, so a key may already hold a host an earlier wizard made; round
// picks the next set of keys past it.
export async function deriveSshHostKeys(
  wizardKey: string,
  label: string,
  endpoint: SshHostEndpoint,
  round = 0,
): Promise<SshHostKeys> {
  const id = JSON.stringify([
    wizardKey,
    label,
    endpoint.host ?? '',
    endpoint.port ?? 0,
    endpoint.username ?? '',
    round,
  ])
  const digest = await crypto.subtle.digest(
    'SHA-256',
    new TextEncoder().encode(id),
  )
  const hash = Array.from(new Uint8Array(digest).subarray(0, 4), (b) =>
    b.toString(16).padStart(2, '0'),
  ).join('')
  const base = slugObjectKeySegment(label) || 'ssh-host'
  return {
    host: `${base}-${hash}`,
    password: `${base}-ssh-password-${hash}`,
    privateKey: `${base}-ssh-private-key-${hash}`,
    passphrase: `${base}-ssh-passphrase-${hash}`,
  }
}

// KeyedSecret is one credential Secret the wizard stores under a derived key.
export interface KeyedSecret {
  objectKey: string
  displayName: string
  kind: string
  contentType: string
  value: Uint8Array
  readerPublicKeyPem: Uint8Array
}

// putKeyedSecret stores the value as the Secret at its key. A Secret that
// exists there was made by an earlier press whose host did not land, so this
// press's value replaces its payload: the person may have corrected it. When
// the create is refused, because another tab or a lost reply made the
// Secret first, the value replaces that one's payload.
export async function putKeyedSecret(
  space: Pick<Space, 'createSecret' | 'writeSecretPayload'>,
  world: IWorldState,
  secret: KeyedSecret,
): Promise<void> {
  const replace = async () => {
    await space.writeSecretPayload({
      objectKey: secret.objectKey,
      expectedKind: secret.kind,
      contentType: secret.contentType,
      value: secret.value,
    })
  }
  if (await objectExists(world, secret.objectKey)) return replace()
  try {
    // The session writes the payload again on a corrected press.
    await space.createSecret({
      ...secret,
      participantRole: SOParticipantRole.SOParticipantRole_WRITER,
    })
  } catch (err) {
    if (!(await objectExists(world, secret.objectKey))) throw err
    await replace()
  }
}

// objectExists reports whether key names an object in the World.
export async function objectExists(
  world: IWorldState,
  key: string,
): Promise<boolean> {
  using obj = await world.getObject(key)
  return !!obj
}

// sshHostSecretKeys lists the Secret keys of one set of SSH Host keys.
export function sshHostSecretKeys(keys: SshHostKeys): string[] {
  return [keys.password, keys.privateKey, keys.passphrase]
}
