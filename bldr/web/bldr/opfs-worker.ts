type OpfsRequestID = string | number | undefined

type OpfsResponseError = {
  name: string
  message: string
}

type OpfsResponse = {
  id: OpfsRequestID
  ok: boolean
  result?: unknown
  error?: OpfsResponseError
}

type FileEntry = {
  handle: FileSystemFileHandle
}

let rootDirectoryID: number | undefined
let rootDirectory: FileSystemDirectoryHandle | undefined
let nextHandleID = 1

const directories = new Map<number, FileSystemDirectoryHandle>()
const files = new Map<number, FileEntry>()
const writeStreams = new Map<number, FileSystemWritableFileStream>()

function nextID(): number {
  return nextHandleID++
}

function storeDirectory(handle: FileSystemDirectoryHandle): number {
  const id = nextID()
  directories.set(id, handle)
  return id
}

function storeFile(handle: FileSystemFileHandle): number {
  const id = nextID()
  files.set(id, { handle })
  return id
}

function storeWriteStream(stream: FileSystemWritableFileStream): number {
  const id = nextID()
  writeStreams.set(id, stream)
  return id
}

// The Go RemoteDriver is the only client. It sends each op as { id, op, args }
// with args a flat object whose keys match these field readers exactly, so the
// readers take one canonical key and reject anything else.
function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function requestID(value: unknown): OpfsRequestID {
  if (typeof value === 'string' || typeof value === 'number') {
    return value
  }
  return undefined
}

function field(args: unknown, name: string): unknown {
  return isObject(args) ? args[name] : undefined
}

function stringField(args: unknown, name: string): string {
  const value = field(args, name)
  if (typeof value !== 'string') {
    throw new TypeError(`expected string field ${name}`)
  }
  return value
}

function booleanField(args: unknown, name: string): boolean {
  return field(args, name) === true
}

function numberField(args: unknown, name: string): number {
  const value = field(args, name)
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new TypeError(`expected number field ${name}`)
  }
  return value
}

function handleField(args: unknown, name: string): number {
  const value = field(args, name)
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new TypeError(`expected handle field ${name}`)
  }
  return value
}

function optionalHandleField(args: unknown, name: string): number | undefined {
  const value = field(args, name)
  if (value === undefined || value === null) {
    return undefined
  }
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new TypeError(`expected handle field ${name}`)
  }
  return value
}

function bytesField(args: unknown, name: string): Uint8Array<ArrayBuffer> {
  const value = field(args, name)
  if (value instanceof ArrayBuffer) {
    return new Uint8Array(value)
  }
  if (ArrayBuffer.isView(value)) {
    const view = new Uint8Array(value.buffer, value.byteOffset, value.byteLength)
    const copy = new Uint8Array(view.byteLength)
    copy.set(view)
    return copy
  }
  throw new TypeError(`expected ArrayBuffer field ${name}`)
}

function getDirectoryHandle(id: number): FileSystemDirectoryHandle {
  const handle = directories.get(id)
  if (!handle) {
    throw new DOMException(`unknown directory handle: ${id}`, 'NotFoundError')
  }
  return handle
}

function getFileEntry(id: number): FileEntry {
  const entry = files.get(id)
  if (!entry) {
    throw new DOMException(`unknown file handle: ${id}`, 'NotFoundError')
  }
  return entry
}

function getWriteStream(id: number): FileSystemWritableFileStream {
  const stream = writeStreams.get(id)
  if (!stream) {
    throw new DOMException(`unknown write stream handle: ${id}`, 'NotFoundError')
  }
  return stream
}

async function getRootDirectory(): Promise<FileSystemDirectoryHandle> {
  if (!rootDirectory) {
    rootDirectory = await navigator.storage.getDirectory()
  }
  return rootDirectory
}

async function rootDirectoryHandleID(): Promise<number> {
  if (rootDirectoryID === undefined) {
    rootDirectoryID = storeDirectory(await getRootDirectory())
  }
  return rootDirectoryID
}

// directoryFromArgs resolves the dir handle, defaulting to the OPFS root when
// the dir field is absent.
async function directoryFromArgs(
  args: unknown,
): Promise<FileSystemDirectoryHandle> {
  const id = optionalHandleField(args, 'dir')
  return id === undefined ? getRootDirectory() : getDirectoryHandle(id)
}

async function fileHandleFromArgs(
  args: unknown,
  create: boolean,
): Promise<FileSystemFileHandle> {
  const dir = await directoryFromArgs(args)
  const name = stringField(args, 'name')
  return dir.getFileHandle(name, { create })
}

function toError(reason: unknown): OpfsResponseError {
  if (reason instanceof DOMException || reason instanceof Error) {
    return {
      name: reason.name || reason.constructor.name || 'Error',
      message: reason.message,
    }
  }
  return {
    name: 'Error',
    message: String(reason),
  }
}

function postResponse(port: MessagePort, response: OpfsResponse): void {
  if (response.ok && response.result instanceof ArrayBuffer) {
    port.postMessage(response, [response.result])
    return
  }
  port.postMessage(response)
}

async function opGetRoot(): Promise<{ id: number }> {
  return { id: await rootDirectoryHandleID() }
}

async function opGetDirectory(args: unknown): Promise<{ id: number }> {
  const parent = await directoryFromArgs(args)
  const name = stringField(args, 'name')
  const create = booleanField(args, 'create')
  const handle = await parent.getDirectoryHandle(name, { create })
  return { id: storeDirectory(handle) }
}

async function opDeleteEntry(args: unknown): Promise<null> {
  const dir = await directoryFromArgs(args)
  const name = stringField(args, 'name')
  const recursive = booleanField(args, 'recursive')
  await dir.removeEntry(name, { recursive })
  return null
}

async function opListDirectory(args: unknown): Promise<string[]> {
  const dir = await directoryFromArgs(args)
  const names: string[] = []
  for await (const [name] of dir.entries()) {
    names.push(name)
  }
  return names
}

async function opFileExists(args: unknown): Promise<boolean> {
  try {
    const dir = await directoryFromArgs(args)
    const name = stringField(args, 'name')
    await dir.getFileHandle(name)
    return true
  } catch (err) {
    if (err instanceof DOMException && err.name === 'NotFoundError') {
      return false
    }
    throw err
  }
}

async function opDirExists(args: unknown): Promise<boolean> {
  try {
    const dir = await directoryFromArgs(args)
    const name = stringField(args, 'name')
    await dir.getDirectoryHandle(name)
    return true
  } catch (err) {
    if (err instanceof DOMException && err.name === 'NotFoundError') {
      return false
    }
    throw err
  }
}

async function opReadFile(args: unknown): Promise<ArrayBuffer> {
  const handle = await fileHandleFromArgs(args, false)
  return handle.getFile().then((file) => file.arrayBuffer())
}

async function opWriteFile(args: unknown): Promise<number> {
  const bytes = bytesField(args, 'data')
  const handle = await fileHandleFromArgs(args, true)
  const writable = await handle.createWritable()
  try {
    if (bytes.byteLength !== 0) {
      await writable.write(bytes)
    }
    await writable.close()
    return bytes.byteLength
  } catch (err) {
    await writable.abort().catch(() => undefined)
    throw err
  }
}

// Async file opens keep only FileSystemFileHandle tokens. Sync access handles pin
// exclusive OPFS locks, so holding one across bbolt read/write calls can block
// later writable sessions against the same OPFS file.
async function opOpenFile(
  args: unknown,
  create: boolean,
): Promise<{ id: number }> {
  const handle = await fileHandleFromArgs(args, create)
  return { id: storeFile(handle) }
}

async function opReadAt(args: unknown): Promise<ArrayBuffer> {
  const fileID = handleField(args, 'file')
  const offset = numberField(args, 'offset')
  const length = numberField(args, 'length')
  if (length <= 0) {
    return new ArrayBuffer(0)
  }
  const entry = getFileEntry(fileID)
  const file = await entry.handle.getFile()
  if (offset >= file.size) {
    return new ArrayBuffer(0)
  }
  const end = Math.min(offset + length, file.size)
  return file.slice(offset, end).arrayBuffer()
}

async function opWriteAt(args: unknown): Promise<number> {
  const fileID = handleField(args, 'file')
  const offset = numberField(args, 'offset')
  const bytes = bytesField(args, 'data')
  const entry = getFileEntry(fileID)
  const writable = await entry.handle.createWritable({ keepExistingData: true })
  try {
    if (offset !== 0) {
      await writable.seek(offset)
    }
    if (bytes.byteLength !== 0) {
      await writable.write(bytes)
    }
    await writable.close()
    return bytes.byteLength
  } catch (err) {
    await writable.abort().catch(() => undefined)
    throw err
  }
}

async function opSize(args: unknown): Promise<number> {
  const entry = getFileEntry(handleField(args, 'file'))
  const file = await entry.handle.getFile()
  return file.size
}

async function opTruncate(args: unknown): Promise<null> {
  const entry = getFileEntry(handleField(args, 'file'))
  const size = numberField(args, 'size')
  const writable = await entry.handle.createWritable({ keepExistingData: true })
  try {
    await writable.truncate(size)
    await writable.close()
    return null
  } catch (err) {
    await writable.abort().catch(() => undefined)
    throw err
  }
}

function opCloseFile(args: unknown): null {
  const fileID = handleField(args, 'file')
  getFileEntry(fileID)
  files.delete(fileID)
  return null
}

async function opCreateWriteStream(args: unknown): Promise<{ id: number }> {
  const handle = await fileHandleFromArgs(args, true)
  const stream = await handle.createWritable()
  return { id: storeWriteStream(stream) }
}

async function opStreamWrite(args: unknown): Promise<number> {
  const stream = getWriteStream(handleField(args, 'stream'))
  const bytes = bytesField(args, 'data')
  if (bytes.byteLength !== 0) {
    await stream.write(bytes)
  }
  return bytes.byteLength
}

async function opStreamClose(args: unknown): Promise<null> {
  const streamID = handleField(args, 'stream')
  const stream = getWriteStream(streamID)
  await stream.close()
  writeStreams.delete(streamID)
  return null
}

async function opStreamAbort(args: unknown): Promise<null> {
  const streamID = handleField(args, 'stream')
  const stream = getWriteStream(streamID)
  await stream.abort()
  writeStreams.delete(streamID)
  return null
}

async function dispatchOp(op: string, args: unknown): Promise<unknown> {
  switch (op) {
    case 'getRoot':
      return opGetRoot()
    case 'getDirectory':
      return opGetDirectory(args)
    case 'deleteEntry':
      return opDeleteEntry(args)
    case 'listDirectory':
      return opListDirectory(args)
    case 'fileExists':
      return opFileExists(args)
    case 'dirExists':
      return opDirExists(args)
    case 'readFile':
      return opReadFile(args)
    case 'writeFile':
      return opWriteFile(args)
    case 'openFile':
      return opOpenFile(args, false)
    case 'createFile':
      return opOpenFile(args, true)
    case 'readAt':
      return opReadAt(args)
    case 'writeAt':
      return opWriteAt(args)
    case 'size':
      return opSize(args)
    case 'truncate':
      return opTruncate(args)
    case 'closeFile':
      return opCloseFile(args)
    case 'createWriteStream':
      return opCreateWriteStream(args)
    case 'streamWrite':
      return opStreamWrite(args)
    case 'streamClose':
      return opStreamClose(args)
    case 'streamAbort':
      return opStreamAbort(args)
    default:
      throw new DOMException(`unsupported OPFS op: ${op}`, 'NotSupportedError')
  }
}

function parseRequest(data: unknown): {
  id: OpfsRequestID
  op: string
  args: unknown
} {
  if (!isObject(data)) {
    throw new TypeError('OPFS request must be an object')
  }
  const op = data['op']
  if (typeof op !== 'string') {
    throw new TypeError('OPFS request op must be a string')
  }
  return {
    id: requestID(data['id']),
    op,
    args: data['args'],
  }
}

function handlePortMessage(
  port: MessagePort,
  event: MessageEvent<unknown>,
): void {
  try {
    const { id, op, args } = parseRequest(event.data)
    void dispatchOp(op, args)
      .then((result) => {
        postResponse(port, { id, ok: true, result })
      })
      .catch((err) => {
        postResponse(port, { id, ok: false, error: toError(err) })
      })
  } catch (err) {
    postResponse(port, { id: undefined, ok: false, error: toError(err) })
  }
}

function bindPort(port: MessagePort): void {
  port.addEventListener('message', (event: MessageEvent<unknown>) => {
    handlePortMessage(port, event)
  })
  port.start()
  port.postMessage({ opfsWorkerReady: true })
}

globalThis.addEventListener('message', (event: MessageEvent<unknown>) => {
  const port = event.ports?.[0]
  if (!port) {
    return
  }
  bindPort(port)
})

export { dispatchOp }
