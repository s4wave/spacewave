import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'

// LLMS_TXT_URL is the agent operating guide the prompt points at.
export const LLMS_TXT_URL = 'https://spacewave.app/llms.txt'

// AgentContext is the place in the app the person was looking at.
export interface AgentContext {
  // spaceId is the Space shared object ID, empty outside a Space.
  spaceId: string
  // spaceName is the Space display name, empty when unknown.
  spaceName?: string
  // objectKey is the open object key, empty at the Space root.
  objectKey: string
  // path is the path inside the object, empty at its root.
  path: string
}

// AgentAccess is how the agent's CLI reaches the account: a pairing code
// approved in the app, or the desktop app's CLI socket.
export type AgentAccess =
  | { kind: 'code'; code: string; ttlMinutes: number }
  | { kind: 'socket'; socketPath: string }

// parseAgentContext reads the Space, object key and path from an app route
// such as /u/1/so/<space>/-/<objectKey>/-/<path>. The session index is left
// out because it belongs to this app's daemon, not the agent's.
export function parseAgentContext(routePath: string): AgentContext {
  const match = /\/so\/([^/]+)(\/.*)?$/.exec(routePath)
  if (!match) {
    return { spaceId: '', objectKey: '', path: '' }
  }
  const { objectKey, path } = parseObjectUri(match[2] ?? '')
  return { spaceId: match[1], objectKey, path }
}

// buildAgentPrompt renders the text the person pastes into their agent. It
// carries what the agent cannot discover and nothing that grants access alone.
export function buildAgentPrompt(
  access: AgentAccess,
  context: AgentContext,
): string {
  const lines = [
    `Read ${LLMS_TXT_URL} and follow its instructions to connect to my Spacewave account.`,
    '',
  ]

  if (access.kind === 'code') {
    lines.push(
      `Pairing code: ${access.code} (expires within ${access.ttlMinutes} minutes)`,
      `Pair with: spacewave login p2p --code ${access.code} --label "<agent> on <host>"`,
      'Show me the six emoji it prints; I will compare them and approve in Spacewave.',
    )
  } else {
    lines.push(
      `The Spacewave desktop app is running with its CLI socket at ${access.socketPath}`,
      `Connect with: SPACEWAVE_SOCKET_PATH=${quoteShell(access.socketPath)} spacewave -o json session list`,
      'No pairing is needed; the socket already holds my account.',
    )
  }

  if (context.spaceId) {
    lines.push('', 'Work in:')
    const name = context.spaceName ? `"${context.spaceName}" ` : ''
    lines.push(`  Space: ${name}(id ${context.spaceId})`)
    if (context.objectKey) {
      lines.push(`  Object: ${context.objectKey}`)
    }
    if (context.path) {
      lines.push(`  Path: ${context.path}`)
    }
  }

  return lines.join('\n') + '\n'
}

// quoteShell quotes a value for a POSIX shell when it needs quoting.
function quoteShell(value: string): string {
  if (/^[\w@%+=:,./-]+$/.test(value)) {
    return value
  }
  return `'${value.replaceAll("'", `'\\''`)}'`
}
