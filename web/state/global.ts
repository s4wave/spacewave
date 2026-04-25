import {
  atom,
  atomWithLocalStorage,
  type Atom,
  type StateType,
} from './persist.js'

// localStateAtom is the root atom for tab-local state (not synced across windows).
// This is the default for most UI state.
export const localStateAtom: Atom<StateType> = atom<StateType>({})

// persistentStateAtom is the root atom for persistent state (synced across windows via localStorage).
// Use this for state that should persist across sessions and sync between windows.
export const persistentStateAtom: Atom<StateType> =
  atomWithLocalStorage<StateType>('app-persistent', {})
