// Dropped files uploading, and the question asked when a name is taken.

type Upload = {
  item: Record<string, string>
  shown: Record<string, string>
}

export const title = 'Upload: asking before a dropped file replaces another'

// question names what the asking dialog shows: a file or a folder, and
// whether an answer can apply to other names too.
const question = (v: Upload) => {
  const asking = Object.keys(v.item).filter((n) => v.item[n] === 'asking')
  if (asking.length === 0) return ''
  const holds = asking.some((n) => v.shown[n] !== 'dir')
    ? 'a file has the name'
    : 'a folder or link has the name'
  return asking.length > 1
    ? `Asking, ${holds}, Do this for the others shown`
    : `Asking, ${holds}`
}

export const state = (v: Upload) => {
  const items = Object.values(v.item)
  if (items.every((s) => s === 'out')) return 'Waiting for a drop'
  const asked = question(v)
  if (asked) return asked
  if (items.includes('failed')) return 'Failed, Retry shown'
  if (items.includes('sending')) return 'Uploading'
  return 'Uploads listed as complete, skipped or cancelled'
}

const steps: Record<string, string> = {
  Drop: 'files dropped',
  OtherWrite: null as unknown as string,
  Land: 'a file is written',
  Refuse: 'the store refuses: another writer took the name',
  Fail: 'a send fails',
  Retry: 'presses Retry',
  CancelAll: 'presses Cancel all',
  CancelFolder: "cancels a folder's row",
  Press: 'answers',
}

export const step = (action: string, from: Upload, to: Upload) => {
  // A step that keeps the drawn state draws no edge.
  if (state(from) === state(to)) return null
  return steps[action.replace(/\(.*$/, '')] ?? null
}
