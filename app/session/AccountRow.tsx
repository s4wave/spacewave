import { LuChevronRight, LuLock, LuPencil, LuUser } from 'react-icons/lu'

import {
  SessionLockMode,
  type SessionMetadata,
} from '@s4wave/core/session/session.pb.js'
import { Button } from '@s4wave/web/ui/button.js'
import { cn } from '@s4wave/web/style/utils.js'

import { accountName } from './account-presentation.js'
import {
  DisplayNameForm,
  type DisplayNameEditor,
  type DisplayNameEditState,
} from './DisplayNameEditor.js'

// AccountRowState is the one value an account row renders from, as
// AccountRow.tla models it. A row past viewing carries the name draft.
export type AccountRowState =
  | { kind: 'loading' }
  | { kind: 'named'; renamable: boolean }
  | { kind: 'unnamed' }
  | { kind: 'locked' }
  | Exclude<DisplayNameEditState, { kind: 'viewing' }>

// accountRowState derives the row state from the watched metadata and the
// name edit. Only a local account that opens without a PIN can be named here.
export function accountRowState(
  meta: SessionMetadata | null,
  edit: DisplayNameEditState,
): AccountRowState {
  if (!meta) return { kind: 'loading' }
  if (edit.kind !== 'viewing') return edit
  const local = meta.providerId === 'local'
  const pin = meta.lockMode === SessionLockMode.PIN_ENCRYPTED
  if (accountName(meta)) return { kind: 'named', renamable: local && !pin }
  return local && pin ? { kind: 'locked' } : { kind: 'unnamed' }
}

// AccountRow renders one account in the session picker: a button that picks
// the account and the naming action its state allows, or the name field while
// a name is edited. editor drives the field and must match row.
export function AccountRow(props: {
  row: AccountRowState
  sessionIndex: number
  title: string
  subtitle: string
  cloud: boolean
  linked: boolean
  inactive: boolean
  editor: DisplayNameEditor
  onSelect: () => void
}) {
  const { row } = props

  if (
    row.kind === 'editing' ||
    row.kind === 'saving' ||
    row.kind === 'failed'
  ) {
    return (
      <div
        data-testid="session-card"
        data-session-index={props.sessionIndex || ''}
        className="border-foreground/10 flex items-center gap-3 rounded-lg border px-4 py-3"
      >
        <AccountIcon />
        <DisplayNameForm
          editor={props.editor}
          label={`Name for ${props.title}`}
        />
      </div>
    )
  }

  return (
    <div
      data-testid="session-card"
      data-session-index={props.sessionIndex || ''}
      className={cn(
        'border-foreground/10 flex items-center rounded-lg border pr-2',
        props.linked && 'opacity-50',
      )}
    >
      <button
        type="button"
        onClick={props.onSelect}
        className="hover:bg-foreground/5 flex min-w-0 flex-1 items-center gap-3 rounded-lg px-4 py-3 text-left transition-colors"
      >
        <AccountIcon />
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <span className="text-foreground truncate text-sm font-medium">
              {props.title}
            </span>
            {props.cloud && (
              <span className="bg-brand/15 text-brand micro-nine rounded-full px-1.5 py-0.5 font-semibold tracking-wider uppercase">
                Cloud
              </span>
            )}
            {props.linked && (
              <span className="text-foreground-alt/80 micro-ten rounded-full px-1.5 py-0.5 font-medium">
                (linked)
              </span>
            )}
            {props.inactive && (
              <span className="bg-foreground/6 text-foreground-alt/75 micro-ten rounded-full px-1.5 py-0.5 font-medium">
                (Inactive)
              </span>
            )}
          </div>
          <div className="text-foreground-alt/60 truncate text-xs">
            {props.subtitle}
          </div>
        </div>
        <LuChevronRight
          aria-hidden
          className="text-foreground-alt/40 size-4 shrink-0"
        />
      </button>
      <div className="flex w-20 shrink-0 justify-end">
        <AccountRowAction
          row={row}
          title={props.title}
          onName={props.editor.start}
        />
      </div>
    </div>
  )
}

// AccountIcon is the leading icon of an account row.
function AccountIcon() {
  return (
    <div className="bg-foreground/5 flex size-9 shrink-0 items-center justify-center rounded-lg">
      <LuUser aria-hidden className="size-4" />
    </div>
  )
}

// AccountRowAction renders the naming action a viewing row offers: Name for
// an unnamed local account, a pencil to rename a named one, and a lock that
// says when a PIN account can be named.
function AccountRowAction(props: {
  row: AccountRowState
  title: string
  onName: () => void
}) {
  const { row } = props
  if (row.kind === 'unnamed') {
    return (
      <Button
        variant="outline"
        size="sm"
        onClick={props.onName}
        aria-label={`Name ${props.title}`}
      >
        Name
      </Button>
    )
  }
  if (row.kind === 'named' && row.renamable) {
    return (
      <Button
        variant="muted"
        size="iconSm"
        onClick={props.onName}
        aria-label={`Rename ${props.title}`}
      >
        <LuPencil aria-hidden className="size-4" />
      </Button>
    )
  }
  if (row.kind === 'locked') {
    const label = 'Locked with a PIN. Unlock the account to name it.'
    return (
      <span
        role="img"
        aria-label={label}
        title={label}
        className="text-foreground-alt/60 flex size-8 items-center justify-center"
      >
        <LuLock aria-hidden className="size-4" />
      </span>
    )
  }
  return null
}
