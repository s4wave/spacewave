// ConversionState is the conversion's control flow and installed copy.
type ConversionState = { pc: string; source: number; temp: number; pathNew: boolean }

// title names the conversion lifecycle.
export const title = 'Convert an old database while keeping its original'

// state distinguishes the installed copy from a newer source commit.
export const state = (v: ConversionState) =>
  v.pathNew ? (v.temp === v.source ? 'Installed current copy' : 'Installed stale copy') : v.pc

// step names the source operation on every visible transition.
export const step = (action: string) => action
