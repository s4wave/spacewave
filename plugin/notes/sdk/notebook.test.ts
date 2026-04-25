import { describe, it, expect } from 'vitest'
import {
  Notebook,
  NotebookSource,
} from '../proto/notebook.pb.js'
import { NotebookHandle, NotebookTypeID } from './notebook.js'

describe('Notebook proto', () => {
  it('creates Notebook message with name and sources', () => {
    const nb = Notebook.create({
      name: 'My Notebook',
      sources: [
        NotebookSource.create({ name: 'Docs', ref: 'obj-key/-/docs' }),
        NotebookSource.create({ name: 'Notes', ref: 'obj-key/-/notes' }),
      ],
    })
    expect(nb.name).toBe('My Notebook')
    expect(nb.sources).toHaveLength(2)
    expect(nb.sources![0].name).toBe('Docs')
    expect(nb.sources![1].ref).toBe('obj-key/-/notes')
  })

  it('serializes and deserializes Notebook via toBinary/fromBinary', () => {
    const original = Notebook.create({
      name: 'Round-trip Test',
      sources: [
        NotebookSource.create({ name: 'Source A', ref: 'key-a/-/path' }),
      ],
    })
    const bytes = Notebook.toBinary(original)
    const decoded = Notebook.fromBinary(bytes)
    expect(decoded.name).toBe('Round-trip Test')
    expect(decoded.sources).toHaveLength(1)
    expect(decoded.sources![0].name).toBe('Source A')
    expect(decoded.sources![0].ref).toBe('key-a/-/path')
  })

  it('handles empty sources list', () => {
    const nb = Notebook.create({ name: 'Empty', sources: [] })
    const bytes = Notebook.toBinary(nb)
    const decoded = Notebook.fromBinary(bytes)
    expect(decoded.name).toBe('Empty')
    expect(decoded.sources ?? []).toHaveLength(0)
  })

  it('handles Notebook with no name', () => {
    const nb = Notebook.create({
      sources: [NotebookSource.create({ name: 'Only Source', ref: 'k/-/p' })],
    })
    const bytes = Notebook.toBinary(nb)
    const decoded = Notebook.fromBinary(bytes)
    expect(decoded.name ?? '').toBe('')
    expect(decoded.sources).toHaveLength(1)
  })
})

describe('NotebookSource proto', () => {
  it('has name and ref fields', () => {
    const src = NotebookSource.create({ name: 'My Source', ref: 'obj/-/sub' })
    expect(src.name).toBe('My Source')
    expect(src.ref).toBe('obj/-/sub')
  })

  it('serializes and deserializes via toBinary/fromBinary', () => {
    const original = NotebookSource.create({
      name: 'Test',
      ref: 'key/-/path',
    })
    const bytes = NotebookSource.toBinary(original)
    const decoded = NotebookSource.fromBinary(bytes)
    expect(decoded.name).toBe('Test')
    expect(decoded.ref).toBe('key/-/path')
  })

  it('handles empty fields', () => {
    const src = NotebookSource.create({})
    expect(src.name ?? '').toBe('')
    expect(src.ref ?? '').toBe('')
  })
})

describe('NotebookHandle', () => {
  it('has correct typeId', () => {
    expect(NotebookTypeID).toBe('spacewave-notes/notebook')
  })

  it('constructs with a mock ClientResourceRef', () => {
    const mockRef = {
      resourceId: 42,
      released: false,
      client: {
        serverStreamingRequest: () => (async function* () {})(),
        request: () => Promise.resolve(new Uint8Array()),
      },
      createRef: () => mockRef,
      createResource: () => null,
      release: () => {},
      [Symbol.dispose]: () => {},
    }
    const handle = new NotebookHandle(mockRef as never)
    expect(handle.id).toBe(42)
    expect(handle.released).toBe(false)
  })

  it('release delegates to resourceRef.release', () => {
    let released = false
    const mockRef = {
      resourceId: 1,
      get released() {
        return released
      },
      client: {
        serverStreamingRequest: () => (async function* () {})(),
        request: () => Promise.resolve(new Uint8Array()),
      },
      createRef: () => mockRef,
      createResource: () => null,
      release: () => {
        released = true
      },
      [Symbol.dispose]: () => {},
    }
    const handle = new NotebookHandle(mockRef as never)
    expect(handle.released).toBe(false)
    handle.release()
    expect(released).toBe(true)
  })
})
