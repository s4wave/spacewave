import { describe, expect, it } from 'vitest'

import { getDefaultStateNamespace } from './useObjectViewer.js'

describe('getDefaultStateNamespace', () => {
  it('uses an explicit namespace over the object-derived default', () => {
    expect(
      getDefaultStateNamespace(
        {
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'drive',
            },
          },
        },
        'drive',
        ['space', 'primary', 'drive'],
      ),
    ).toEqual(['space', 'primary', 'drive'])
  })

  it('keys world object viewer state by object key by default', () => {
    expect(
      getDefaultStateNamespace(
        {
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'drive',
            },
          },
        },
        'drive',
        undefined,
      ),
    ).toEqual(['objectViewer', 'drive'])
  })

  it('keys UnixFS viewer state by UnixFS id and scoped path', () => {
    expect(
      getDefaultStateNamespace(
        {
          info: {
            case: 'unixfsObjectInfo',
            value: {
              unixfsId: 'files',
              path: '/photos/2026',
            },
          },
        },
        undefined,
        undefined,
      ),
    ).toEqual(['objectViewer', 'unixfs', 'files', '/photos/2026'])
  })

  it('changes the UnixFS viewer namespace when the scoped path changes', () => {
    const rootNs = getDefaultStateNamespace(
      {
        info: {
          case: 'unixfsObjectInfo',
          value: {
            unixfsId: 'files',
            path: '/',
          },
        },
      },
      undefined,
      undefined,
    )
    const nestedNs = getDefaultStateNamespace(
      {
        info: {
          case: 'unixfsObjectInfo',
          value: {
            unixfsId: 'files',
            path: '/nested',
          },
        },
      },
      undefined,
      undefined,
    )

    expect(rootNs).not.toEqual(nestedNs)
  })
})
