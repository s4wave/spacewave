import { describe, expect, it } from 'vitest'

import {
  buildSpacewaveCommand,
  DEFAULT_SESSION_INDEX,
  SHARED_DEFAULT_SOCKET_PATH,
} from './command-line-commands.js'

describe('buildSpacewaveCommand', () => {
  it('omits flags on the shared default session and socket', () => {
    expect(
      buildSpacewaveCommand('status', {
        sessionIndex: DEFAULT_SESSION_INDEX,
        socketPath: '',
      }),
    ).toBe('spacewave status')
  })

  it('omits --socket-path when the live path matches the shared default', () => {
    expect(
      buildSpacewaveCommand('status', {
        sessionIndex: DEFAULT_SESSION_INDEX,
        socketPath: SHARED_DEFAULT_SOCKET_PATH,
      }),
    ).toBe('spacewave status')
  })

  it('omits --socket-path when the listener reports the expanded shared default', () => {
    expect(
      buildSpacewaveCommand('status', {
        sessionIndex: DEFAULT_SESSION_INDEX,
        socketPath: '/Users/example/.spacewave/spacewave.sock',
      }),
    ).toBe('spacewave status')
  })

  it('emits --socket-path when the listener path differs from the default', () => {
    expect(
      buildSpacewaveCommand('status', {
        sessionIndex: DEFAULT_SESSION_INDEX,
        socketPath: '/run/custom.sock',
      }),
    ).toBe("spacewave --socket-path '/run/custom.sock' status")
  })

  it('quotes socket paths with spaces and single quotes', () => {
    expect(
      buildSpacewaveCommand('status', {
        sessionIndex: DEFAULT_SESSION_INDEX,
        socketPath: "/Users/example/Space Wave's/socket.sock",
      }),
    ).toBe(
      `spacewave --socket-path '/Users/example/Space Wave'"'"'s/socket.sock' status`,
    )
  })

  it('emits --session-index when the active session is not the default', () => {
    expect(
      buildSpacewaveCommand('whoami', {
        sessionIndex: 2,
        socketPath: '',
      }),
    ).toBe('spacewave --session-index 2 whoami')
  })

  it('emits both flags together, socket-path before session-index', () => {
    expect(
      buildSpacewaveCommand('space list', {
        sessionIndex: 3,
        socketPath: '/run/alt.sock',
      }),
    ).toBe(
      "spacewave --socket-path '/run/alt.sock' --session-index 3 space list",
    )
  })

  it('passes subcommands with spaces through untouched', () => {
    expect(
      buildSpacewaveCommand('space list', {
        sessionIndex: DEFAULT_SESSION_INDEX,
        socketPath: '',
      }),
    ).toBe('spacewave space list')
  })
})
