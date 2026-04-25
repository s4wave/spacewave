import { describe, expect, it } from 'vitest'

import { AccountEscalationIntentKind } from '@s4wave/sdk/account/account.pb.js'
import { AccountAuthMethodKind } from '@s4wave/core/provider/spacewave/api/api.pb.js'

import { buildAccountEscalationState } from './useAccountEscalationState.js'

describe('buildAccountEscalationState', () => {
  it('derives methods and single-sig requirement from watched state', () => {
    const got = buildAccountEscalationState(
      {
        kind: AccountEscalationIntentKind.AccountEscalationIntentKind_ACCOUNT_ESCALATION_INTENT_KIND_REVOKE_SESSION,
        title: 'Sign Out Session',
        description: 'Sign out Safari from Spacewave Cloud.',
        targetLabel: 'Safari',
        targetPeerId: 'peer-2',
      },
      0,
      [
        {
          peerId: 'peer-password',
          kind: AccountAuthMethodKind.PASSWORD,
          label: 'Password',
          secondaryLabel: 'Primary sign-in',
        },
        {
          peerId: 'peer-backup',
          kind: AccountAuthMethodKind.BACKUP_KEY,
          label: 'Backup key',
          secondaryLabel: 'Stored offline',
        },
      ],
      [
        {
          keypair: { peerId: 'peer-password', authMethod: 'password' },
          unlocked: true,
        },
      ],
      1,
    )

    expect(got.intent?.title).toBe('Sign Out Session')
    expect(got.requirement?.authThreshold).toBe(0)
    expect(got.requirement?.requiredSigners).toBe(1)
    expect(got.requirement?.unlockedSigners).toBe(1)
    expect(got.requirement?.totalMethods).toBe(2)
    expect(got.methods).toEqual([
      {
        peerId: 'peer-password',
        kind: AccountAuthMethodKind.PASSWORD,
        label: 'Password',
        secondaryLabel: 'Primary sign-in',
        provider: undefined,
        unlocked: true,
      },
      {
        peerId: 'peer-backup',
        kind: AccountAuthMethodKind.BACKUP_KEY,
        label: 'Backup key',
        secondaryLabel: 'Stored offline',
        provider: undefined,
        unlocked: false,
      },
    ])
  })

  it('derives multi-sig signer counts from the account threshold', () => {
    const got = buildAccountEscalationState(
      {
        kind: AccountEscalationIntentKind.AccountEscalationIntentKind_ACCOUNT_ESCALATION_INTENT_KIND_LINK_SSO,
        title: 'Link Google',
        description: 'Confirm your identity to link Google.',
        provider: 'google',
      },
      2,
      [
        {
          peerId: 'peer-1',
          kind: AccountAuthMethodKind.PASSWORD,
          label: 'Password',
        },
        {
          peerId: 'peer-2',
          kind: AccountAuthMethodKind.BACKUP_KEY,
          label: 'Backup key',
        },
        {
          peerId: 'peer-3',
          kind: AccountAuthMethodKind.GOOGLE_SSO,
          label: 'Google',
          provider: 'google',
        },
      ],
      [
        {
          keypair: { peerId: 'peer-1', authMethod: 'password' },
          unlocked: true,
        },
        {
          keypair: { peerId: 'peer-2', authMethod: 'pem' },
          unlocked: true,
        },
      ],
      2,
    )

    expect(got.requirement?.authThreshold).toBe(2)
    expect(got.requirement?.requiredSigners).toBe(3)
    expect(got.requirement?.unlockedSigners).toBe(2)
    expect(got.requirement?.totalMethods).toBe(3)
    expect(got.methods?.[2]).toEqual({
      peerId: 'peer-3',
      kind: AccountAuthMethodKind.GOOGLE_SSO,
      label: 'Google',
      secondaryLabel: undefined,
      provider: 'google',
      unlocked: false,
    })
  })
})
