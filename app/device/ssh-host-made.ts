import {
  SshHost,
  type SshHostEndpoint,
} from '@s4wave/sdk/sshhost/sshhost.pb.js'
import { Terminal } from '@s4wave/sdk/terminal/terminal.pb.js'
import { iriToKey, keyToIRI } from '@s4wave/sdk/world/graph-utils.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// PRED_WIZARD_SSH_HOST links an Add Device wizard to an SSH Host it created,
// and PRED_WIZARD_SSH_TERMINAL links the wizard to a terminal it created for
// a host. Each link commits in one list with the create it records, so a link
// identifies an object that exists, and deleting the wizard removes its links.
export const PRED_WIZARD_SSH_HOST = '<wizard/ssh-host>'
export const PRED_WIZARD_SSH_TERMINAL = '<wizard/ssh-terminal>'

// maxSshHostsMade bounds the links one read follows. A wizard links one host
// per distinct host the person entered across failed Finishes.
const maxSshHostsMade = 32

// SshHostMade is an SSH Host a wizard created, with the terminal it created
// for that host, or an empty key when that step has not landed.
export interface SshHostMade {
  hostObjectKey: string
  host: SshHost
  terminalObjectKey: string
}

// readSshHostsMade reads the SSH Hosts the wizard created and their
// terminals from one World. Objects deleted since are left out.
export async function readSshHostsMade(
  world: IWorldState,
  wizardKey: string,
  signal?: AbortSignal,
): Promise<SshHostMade[]> {
  const linked = async (predicate: string) => {
    const res = await world.lookupGraphQuads(
      keyToIRI(wizardKey),
      predicate,
      undefined,
      undefined,
      maxSshHostsMade,
      signal,
    )
    return (res.quads ?? []).map((q) => iriToKey(q.obj ?? ''))
  }
  const [hostKeys, terminalKeys] = await Promise.all([
    linked(PRED_WIZARD_SSH_HOST),
    linked(PRED_WIZARD_SSH_TERMINAL),
  ])
  if (hostKeys.length === 0) return []
  const bodies = await world.getObjectBodies(
    [...hostKeys, ...terminalKeys],
    signal,
  )

  // Pair each terminal with the host it opens.
  const terminals = new Map<string, string>()
  for (const body of bodies.slice(hostKeys.length)) {
    if (!body.exists || !body.body || !body.objectKey) continue
    const hostKey = Terminal.fromBinary(body.body).sshHostObjectKey
    if (hostKey && !terminals.has(hostKey)) {
      terminals.set(hostKey, body.objectKey)
    }
  }
  return bodies.slice(0, hostKeys.length).flatMap((body) => {
    if (!body.exists || !body.body || !body.objectKey) return []
    return [
      {
        hostObjectKey: body.objectKey,
        host: SshHost.fromBinary(body.body),
        terminalObjectKey: terminals.get(body.objectKey) ?? '',
      },
    ]
  })
}

// findSshHostMade returns the host the wizard created with this label and
// endpoint. Credentials are not compared: the wizard never stores them, so
// after a reload the person's draft is empty, and the host keeps the
// credentials it was created with.
export function findSshHostMade(
  made: SshHostMade[],
  label: string,
  endpoint: SshHostEndpoint,
): SshHostMade | undefined {
  return made.find(
    ({ host }) =>
      (host.label ?? '') === label &&
      (host.endpoint?.host ?? '') === (endpoint.host ?? '') &&
      (host.endpoint?.port ?? 0) === (endpoint.port ?? 0) &&
      (host.endpoint?.username ?? '') === (endpoint.username ?? ''),
  )
}
