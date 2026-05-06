import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

function readLocalSource(fileName: string): string {
  return readFileSync(new URL(fileName, import.meta.url), 'utf8')
}

describe('spacewave frontend route contract', () => {
  it('keeps cloud root routing on the Onboarding Status context instead of route-level streams', () => {
    const source = readLocalSource('./SpacewaveRootRouter.tsx')

    expect(source).toContain('SpacewaveOnboardingContext.useContextSafe()')
    expect(source).not.toMatch(/\buseStreamingResource\b/)
    expect(source).not.toMatch(/\buseResource\b/)
    expect(source).not.toMatch(/\.spacewave\.(?:watch|get|list|create|mount)/)
    expect(source).not.toMatch(/\bfetch\s*\(/)
    expect(source).not.toMatch(/\bWebSocket\b/)
  })

  it('keeps account-status helpers as pure Onboarding Status predicates', () => {
    const source = readLocalSource('./account-status.ts')

    expect(source).toContain('WatchOnboardingStatusResponse')
    expect(source).not.toMatch(/\buseStreamingResource\b/)
    expect(source).not.toMatch(/\buseResource\b/)
    expect(source).not.toMatch(/\.spacewave\.(?:watch|get|list|create|mount)/)
    expect(source).not.toMatch(/\bfetch\s*\(/)
    expect(source).not.toMatch(/\bWebSocket\b/)
  })
})
