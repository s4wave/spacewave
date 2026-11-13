import {
  AccountRow,
  type AccountRowState,
} from '@s4wave/app/session/AccountRow.js'
import type {
  DisplayNameEditor,
  DisplayNameEditState,
} from '@s4wave/app/session/DisplayNameEditor.js'

// AccountRowFixture is one state of the session picker's account row.
interface AccountRowFixture {
  label: string
  row: AccountRowState
  title: string
  subtitle: string
  cloud?: boolean
  changed?: boolean
}

const unnamedTitle = 'Local account, created Nov 4, 2026, 3:12 PM'
const unnamedSubtitle = 'Local · Session 2 · 9xQe4T'

// fixtures lists every row state AccountRow.tla reaches.
const fixtures: AccountRowFixture[] = [
  {
    label: 'Loading',
    row: { kind: 'loading' },
    title: 'Session 1',
    subtitle: 'Local',
  },
  {
    label: 'Cloud account',
    row: { kind: 'named', renamable: false },
    title: 'alice',
    subtitle: 'Cloud',
    cloud: true,
  },
  {
    label: 'Named',
    row: { kind: 'named', renamable: true },
    title: 'Work laptop',
    subtitle: 'Local',
  },
  {
    label: 'Unnamed local account',
    row: { kind: 'unnamed' },
    title: unnamedTitle,
    subtitle: unnamedSubtitle,
  },
  {
    label: 'Locked: named after unlocking',
    row: { kind: 'locked' },
    title: 'Local account, created Oct 30, 2026, 9:41 AM',
    subtitle: 'Local · Session 4 · Lk2v7P',
  },
  {
    label: 'Editing the name',
    row: { kind: 'editing', draft: 'Home desktop' },
    title: unnamedTitle,
    subtitle: unnamedSubtitle,
    changed: true,
  },
  {
    label: 'Saving',
    row: { kind: 'saving', draft: 'Home desktop' },
    title: unnamedTitle,
    subtitle: unnamedSubtitle,
    changed: true,
  },
  {
    label: 'Save failed',
    row: {
      kind: 'failed',
      draft: 'Home desktop',
      error: 'The account is not ready',
    },
    title: unnamedTitle,
    subtitle: unnamedSubtitle,
    changed: true,
  },
]

// fixtureEditor is a display name editor frozen in one state.
function fixtureEditor(row: AccountRowState, changed: boolean) {
  const state: DisplayNameEditState =
    row.kind === 'editing' || row.kind === 'saving' || row.kind === 'failed'
      ? row
      : { kind: 'viewing' }
  const noop = () => {}
  const editor: DisplayNameEditor = {
    state,
    changed,
    start: noop,
    edit: noop,
    cancel: noop,
    save: noop,
  }
  return editor
}

// AccountRowsDebug renders the session picker's account row in each state.
export function AccountRowsDebug() {
  return (
    <main className="bg-background-landing flex h-full w-full justify-center overflow-auto px-4 py-12">
      <div className="w-full max-w-md space-y-4">
        <h1 className="text-foreground text-lg font-semibold">Account rows</h1>
        {fixtures.map((fixture) => (
          <section key={fixture.label} className="space-y-2">
            <h2 className="text-foreground-alt/70 text-xs font-medium">
              {fixture.label}
            </h2>
            <AccountRow
              row={fixture.row}
              sessionIndex={0}
              title={fixture.title}
              subtitle={fixture.subtitle}
              cloud={!!fixture.cloud}
              linked={false}
              inactive={false}
              editor={fixtureEditor(fixture.row, !!fixture.changed)}
              onSelect={() => {}}
            />
          </section>
        ))}
      </div>
    </main>
  )
}
