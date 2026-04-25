// SelfEnrollmentSkipState records the self-enrollment generation skipped by this session.
export interface SelfEnrollmentSkipState {
  skippedKey: string
  skippedAt: number
}

export const selfEnrollmentSkipAtomKey = 'selfEnrollmentSkip'

export const defaultSelfEnrollmentSkip: SelfEnrollmentSkipState | null = null
