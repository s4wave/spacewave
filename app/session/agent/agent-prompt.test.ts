import { describe, expect, it } from 'vitest'

import { buildAgentPrompt, parseAgentContext } from './agent-prompt.js'

describe('parseAgentContext', () => {
  it('reads the Space, object key and path from a session route', () => {
    expect(
      parseAgentContext('/u/2/so/space-1/-/notes/index/-/docs/a.md'),
    ).toEqual({
      spaceId: 'space-1',
      objectKey: 'notes/index',
      path: 'docs/a.md',
    })
  })

  it('reads a Space root', () => {
    expect(parseAgentContext('/u/1/so/space-1')).toEqual({
      spaceId: 'space-1',
      objectKey: '',
      path: '',
    })
  })

  it('returns an empty context outside a Space', () => {
    expect(parseAgentContext('/u/1/settings')).toEqual({
      spaceId: '',
      objectKey: '',
      path: '',
    })
  })
})

describe('buildAgentPrompt', () => {
  it('carries the pairing code and the open object', () => {
    const prompt = buildAgentPrompt(
      { kind: 'code', code: 'ABCD2345', ttlMinutes: 10 },
      {
        spaceId: 'space-1',
        spaceName: 'Notes',
        objectKey: 'notes/index',
        path: 'docs/a.md',
      },
    )
    expect(prompt).toContain('https://spacewave.app/llms.txt')
    expect(prompt).toContain('spacewave login p2p --code ABCD2345')
    expect(prompt).toContain('Space: "Notes" (id space-1)')
    expect(prompt).toContain('Object: notes/index')
    expect(prompt).toContain('Path: docs/a.md')
  })

  it('uses the desktop socket instead of a code', () => {
    const prompt = buildAgentPrompt(
      { kind: 'socket', socketPath: '/Users/me/Library/App Support/sw.sock' },
      { spaceId: '', objectKey: '', path: '' },
    )
    expect(prompt).toContain(
      "SPACEWAVE_SOCKET_PATH='/Users/me/Library/App Support/sw.sock'",
    )
    expect(prompt).not.toContain('login p2p')
    expect(prompt).not.toContain('Work in:')
  })
})
