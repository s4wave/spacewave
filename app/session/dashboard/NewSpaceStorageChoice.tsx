import { useId } from 'react'
import { LuDatabase } from '@s4wave/web/images/icons.js'

import { accountStorageChoice } from '@s4wave/app/session/storage/storage-backend.js'
import { useStorageBackends } from '@s4wave/app/session/storage/useStorageBackends.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: { WebkitUserSelect: 'none', userSelect: 'none' },
  rule3: {
    '--tw-outline-style': 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: 'none',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

interface NewSpaceStorageChoiceProps {
  // value is a storage backend id, accountStorageChoice, or empty for the
  // account's default.
  value: string
  onChange: (value: string) => void
}

// NewSpaceStorageChoice picks where the next new Space stores its data. It
// shows only when the account has at least one storage backend.
export function NewSpaceStorageChoice({
  value,
  onChange,
}: NewSpaceStorageChoiceProps) {
  const id = useId()
  const resource = useStorageBackends()
  const backends = resource.value?.storageBackends ?? []
  if (resource.error || backends.length === 0) {
    return null
  }

  const defaultId = resource.value?.defaultStorageBackendId ?? ''
  const defaultName =
    backends.find((b) => b.backend?.id === defaultId)?.backend?.displayName ||
    "the account's own storage"

  return (
    <div sx={stylexStyles2.rule1}>
      <LuDatabase sx={stylexStyles.rule1} aria-hidden="true" />
      <label htmlFor={id} sx={stylexStyles2.rule2}>
        New Spaces store data in
      </label>
      <select
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        sx={stylexStyles2.rule3}
        data-testid="new-space-storage"
      >
        <option value="">{defaultName} (default)</option>
        {defaultId && (
          <option value={accountStorageChoice}>
            the account's own storage
          </option>
        )}
        {backends
          .filter((b) => b.backend?.id !== defaultId)
          .map((b) => (
            <option key={b.backend?.id} value={b.backend?.id}>
              {b.backend?.displayName}
            </option>
          ))}
      </select>
    </div>
  )
}
