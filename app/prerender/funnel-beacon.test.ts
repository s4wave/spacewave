import { describe, expect, it } from 'vitest'

import { FUNNEL_BEACON_SOURCE } from './funnel-beacon.js'

// beacons runs the funnel beacon at the URL and returns the events it sent.
function beacons(url: string, session = false): string[] {
  const u = new URL(url)
  const sent: string[] = []
  const run = new Function(
    'location',
    'localStorage',
    'navigator',
    FUNNEL_BEACON_SOURCE,
  )
  run(
    u,
    { getItem: () => (session ? '1' : null) },
    {
      sendBeacon: (path: string, body: string) => {
        sent.push(`${path} ${body}`)
        return true
      },
    },
  )
  return sent
}

describe('FUNNEL_BEACON_SOURCE', () => {
  it('reports landing and intent pages on the production website', () => {
    expect(beacons('https://spacewave.app/')).toEqual([
      '/api/funnel/event LANDING',
    ])
    expect(beacons('https://spacewave.app/pricing')).toEqual([
      '/api/funnel/event LANDING',
    ])
    expect(beacons('https://spacewave.app/quickstart/drive')).toEqual([
      '/api/funnel/event INTENT',
    ])
  })

  it('reports nothing elsewhere or when the app boots', () => {
    expect(beacons('https://spacewave.app/blog')).toEqual([])
    expect(beacons('https://staging.spacewave.app/')).toEqual([])
    expect(beacons('http://localhost:8080/')).toEqual([])
    expect(beacons('https://spacewave.app/#/u/1')).toEqual([])
    expect(beacons('https://spacewave.app/', true)).toEqual([])
  })
})
