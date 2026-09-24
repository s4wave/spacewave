import {
  useCallback,
  useState,
  useSyncExternalStore,
  type FormEvent,
} from 'react'
import { createRoot } from 'react-dom/client'
import { connect, SyncError, type ConnectionState } from 'spacewave'
import { createSyncContext } from 'spacewave/react'

import { schema } from './schema.ts'

const { SyncProvider, useDatabase, useCollection } = createSyncContext(schema)

/** UncertainWrite retains the original input and request ID for an explicit retry. */
interface UncertainWrite {
  requestId: string
  message: string
  retry: () => void
}

const connectionLabels: Record<ConnectionState['status'], string> = {
  ready: 'Connected',
  reconnecting: 'Reconnecting…',
  closed: 'Disconnected',
}

/** TaskBoard renders accepted tasks and keeps pending form input separate. */
function TaskBoard() {
  // Read task and connection state from the database's existing subscriptions.
  const db = useDatabase()
  const todos = useCollection('todos')
  const connection = useSyncExternalStore(
    db.connection.subscribe,
    () => db.connection.current,
    () => db.connection.current,
  )

  // Keep editable input and unresolved writes outside the accepted task snapshot.
  const [title, setTitle] = useState('')
  const [pending, setPending] = useState(false)
  const [uncertain, setUncertain] = useState<UncertainWrite | null>(null)
  const [writeError, setWriteError] = useState<string | null>(null)

  /** runWrite exposes an uncertain write's original operation for a later retry. */
  const runWrite = useCallback(async function runWrite(
    run: () => Promise<unknown>,
    requestId: string,
  ): Promise<void> {
    // Suspend new edits and clear the previous write's feedback.
    setPending(true)
    setUncertain(null)
    setWriteError(null)

    // Keep failed writes recoverable without inserting unaccepted task rows.
    try {
      await run()
    } catch (error) {
      if (error instanceof SyncError && error.code === 'UNCERTAIN') {
        setUncertain({
          requestId,
          message: error.message,
          retry: () => {
            void runWrite(run, requestId)
          },
        })
      } else {
        setWriteError(error instanceof Error ? error.message : String(error))
      }
    } finally {
      setPending(false)
    }
  }, [])

  /** addTask preserves the title and generated IDs until the write is accepted. */
  const addTask = (event: FormEvent<HTMLFormElement>) => {
    // Accept a nonempty title only while connected and ready for another write.
    event.preventDefault()
    const trimmed = title.trim()
    if (!trimmed || pending || connection.status !== 'ready') return

    // Capture the write once so Retry submits the same input with the same ID.
    const id = crypto.randomUUID()
    const requestId = crypto.randomUUID()
    void runWrite(async () => {
      await db
        .collection('todos')
        .put(id, { title: trimmed, done: false }, { requestId })
      setTitle('')
    }, requestId)
  }

  /** completeTask invokes the server mutation with a recoverable request ID. */
  const completeTask = (id: string) => {
    const requestId = crypto.randomUUID()
    void runWrite(
      () => db.mutate('completeTodo', { id }, { requestId }),
      requestId,
    )
  }

  // Describe the snapshot's freshness without treating stale data as current.
  const todosStatus =
    todos.status === 'loading'
      ? 'Loading tasks…'
      : todos.status === 'current'
        ? `Live · ${todos.data.length} ${
            todos.data.length === 1 ? 'task' : 'tasks'
          }`
        : 'Showing the last saved view while reconnecting…'

  // Render connection feedback, write controls, and the accepted task list.
  return (
    <>
      <h1>Task board</h1>
      <p>
        Two tabs share one server: the <a href="/">Vanilla</a> example and this{' '}
        <a href="/react">React</a> example.
      </p>

      <p className="status">
        {`Connection: ${connectionLabels[connection.status]}`}
        {connection.error ? `: ${connection.error.message}` : ''}
      </p>
      {todos.status === 'error' && todos.error ? (
        <p className="notice" role="alert">
          {todos.error.message}
        </p>
      ) : (
        <p className="status">{todosStatus}</p>
      )}

      <form onSubmit={addTask}>
        <label htmlFor="new-task">
          Add a task
          <input
            id="new-task"
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            disabled={pending}
            required
            maxLength={120}
          />
        </label>
        <button
          className="button"
          type="submit"
          disabled={pending || connection.status !== 'ready' || !title.trim()}
        >
          Add task
        </button>
      </form>

      {writeError ? <p className="notice">{writeError}</p> : null}
      {uncertain ? (
        <p className="notice">
          {uncertain.message} The write may not have been accepted (request{' '}
          {uncertain.requestId}).{' '}
          <button
            className="button"
            type="button"
            disabled={pending || connection.status !== 'ready'}
            onClick={uncertain.retry}
          >
            Retry
          </button>
        </p>
      ) : null}

      <ul className="tasks">
        {todos.data.map((entry) => (
          <li className="task" key={entry.key} data-done={entry.value.done}>
            <span>{entry.value.title}</span>{' '}
            <button
              className="button"
              type="button"
              disabled={
                pending || connection.status !== 'ready' || entry.value.done
              }
              onClick={() => completeTask(entry.key)}
            >
              {entry.value.done ? 'Completed' : 'Complete'}
            </button>
          </li>
        ))}
      </ul>
    </>
  )
}

/** start connects the board and releases its React tree and database on pagehide. */
async function start(): Promise<void> {
  // Require the page's mount point before opening a connection.
  const container = document.getElementById('app')
  if (!container) throw new Error('Missing #app mount point')

  // Connect to the sync endpoint served alongside this page.
  const url = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${
    location.host
  }/sync`
  const db = await connect({
    url,
    schema,
    getAccessToken: () => 'local-demo',
  })

  // Supply the admitted database to every task-board component.
  const root = createRoot(container)
  root.render(
    <SyncProvider db={db}>
      <TaskBoard />
    </SyncProvider>,
  )

  // Unmount subscriptions before closing the database they use.
  window.addEventListener(
    'pagehide',
    () => {
      root.unmount()
      void db.close()
    },
    { once: true },
  )
}

// Replace the loading view with a readable failure if startup cannot finish.
start().catch((error: unknown) => {
  const container = document.getElementById('app') ?? document.body
  const notice = document.createElement('p')
  notice.className = 'notice'
  notice.textContent = `Task board failed to start: ${
    error instanceof Error ? error.message : String(error)
  }`
  container.replaceChildren(notice)
})
