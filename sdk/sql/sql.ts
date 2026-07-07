import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'
import type { Message } from '@aptre/protobuf-es-lite'
import { pushable, type Pushable } from 'it-pushable'
import { Client as SRPCClient, openRpcStream } from 'starpc'

import type { ColumnSchema, Row, SqlValue } from '../../db/sql/sql.pb.js'
import { SqlClient, SqlOpsClient } from '../../db/sql/rpc/sql_srpc.pb.js'
import type {
  SqlExecResponse,
  SqlQueryRequest,
  SqlQueryResponse,
  SqlTransactionRequest,
  SqlTransactionResponse,
} from '../../db/sql/rpc/sql.pb.js'

// SqlDbTypeID is the world ObjectType id for SQL databases.
export const SqlDbTypeID = 'sql/db'

// SqlQueryResult is a fully buffered SQL query result.
export interface SqlQueryResult {
  columns: ColumnSchema[]
  rows: Row[]
}

// ISqlDatabase contains the SQL database handle interface.
export interface ISqlDatabase {
  exec(
    query: string,
    args?: SqlValue[],
    abortSignal?: AbortSignal,
  ): Promise<SqlExecResponse>
  query(
    query: string,
    args?: SqlValue[],
    abortSignal?: AbortSignal,
  ): Promise<SqlQueryResult>
  listSchemas(abortSignal?: AbortSignal): Promise<string[]>
  listTables(schema?: string, abortSignal?: AbortSignal): Promise<string[]>
  withTransaction<T>(
    write: boolean,
    dsn: string,
    fn: (tx: SqlTransaction) => Promise<T>,
    abortSignal?: AbortSignal,
  ): Promise<T>
  release(): void
  [Symbol.dispose](): void
}

// SqlDatabase is the typed SDK handle for a sql/db object.
export class SqlDatabase extends Resource implements ISqlDatabase {
  private service: SqlClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SqlClient(resourceRef.client)
  }

  // exec runs a write statement in its own transaction.
  public async exec(
    query: string,
    args: SqlValue[] = [],
    abortSignal?: AbortSignal,
  ): Promise<SqlExecResponse> {
    return this.withTransaction(
      true,
      '',
      async (tx) => tx.exec(query, args, abortSignal),
      abortSignal,
    )
  }

  // query runs a read query in its own transaction and buffers the result.
  public async query(
    query: string,
    args: SqlValue[] = [],
    abortSignal?: AbortSignal,
  ): Promise<SqlQueryResult> {
    return this.withTransaction(
      false,
      '',
      async (tx) => tx.query(query, args, abortSignal),
      abortSignal,
    )
  }

  // listSchemas returns schema names visible to the database.
  public async listSchemas(abortSignal?: AbortSignal): Promise<string[]> {
    const result = await this.query('SHOW DATABASES', [], abortSignal)
    return result.rows.flatMap((row) => sqlValueToString(row.values?.[0]))
  }

  // listTables returns table names visible in a schema.
  public async listTables(
    schema = '',
    abortSignal?: AbortSignal,
  ): Promise<string[]> {
    const target = schema ? ` FROM ${quoteIdentifier(schema)}` : ''
    const result = await this.query(`SHOW TABLES${target}`, [], abortSignal)
    return result.rows.flatMap((row) => sqlValueToString(row.values?.[0]))
  }

  // withTransaction opens a SQL transaction and commits or discards it.
  public async withTransaction<T>(
    write: boolean,
    dsn: string,
    fn: (tx: SqlTransaction) => Promise<T>,
    abortSignal?: AbortSignal,
  ): Promise<T> {
    const tx = await this.openTransaction(write, dsn, abortSignal)
    try {
      const result = await fn(tx)
      if (write) {
        await tx.commit(abortSignal)
        return result
      }
      await tx.discard()
      return result
    } catch (err) {
      await tx.discard()
      throw err
    }
  }

  // openTransaction opens a SQL transaction.
  public async openTransaction(
    write: boolean,
    dsn = '',
    abortSignal?: AbortSignal,
  ): Promise<SqlTransaction> {
    const requests = pushable<Message<SqlTransactionRequest>>({
      objectMode: true,
    })
    const responses = this.service.SqlTransaction(requests, abortSignal)
    const iterator = responses[Symbol.asyncIterator]()

    requests.push({
      body: {
        case: 'init',
        value: { write, dsn },
      },
    })
    const ack = await nextTransactionResponse(iterator)
    if (ack.body?.case !== 'ack') {
      requests.end()
      throw new Error('sql/db: transaction did not return ack')
    }
    if (ack.body.value.error) {
      requests.end()
      throw new Error(ack.body.value.error)
    }
    const transactionId = ack.body.value.transactionId ?? ''
    if (!transactionId) {
      requests.end()
      throw new Error('sql/db: transaction ack missing transaction id')
    }

    const opsRpc = new SRPCClient(() =>
      openRpcStream(
        transactionId,
        (stream) => this.service.SqlTransactionRpc(stream, abortSignal),
        false,
      ),
    )
    return new SqlTransaction(requests, iterator, new SqlOpsClient(opsRpc))
  }
}

// SqlTransaction wraps a SQL transaction control stream and ops client.
export class SqlTransaction {
  private released = false

  constructor(
    private readonly requests: Pushable<Message<SqlTransactionRequest>>,
    private readonly responses: AsyncIterator<Message<SqlTransactionResponse>>,
    private readonly ops: SqlOpsClient,
  ) {}

  // exec runs a statement in this transaction.
  public async exec(
    query: string,
    args: SqlValue[] = [],
    abortSignal?: AbortSignal,
  ): Promise<SqlExecResponse> {
    const resp = await this.ops.Exec({ query, args }, abortSignal)
    if (resp.error) throw new Error(resp.error)
    return resp
  }

  // query runs a query in this transaction and buffers all rows.
  public async query(
    query: string,
    args: SqlValue[] = [],
    abortSignal?: AbortSignal,
  ): Promise<SqlQueryResult> {
    const requests = pushable<Message<SqlQueryRequest>>({
      objectMode: true,
    })
    const responses = this.ops.Query(requests, abortSignal)
    const iterator = responses[Symbol.asyncIterator]()

    requests.push({
      body: {
        case: 'init',
        value: { query, args },
      },
    })
    const ack = await nextQueryResponse(iterator)
    if (ack.body?.case === 'reqError') {
      requests.end()
      throw new Error(ack.body.value)
    }
    if (ack.body?.case !== 'ack') {
      requests.end()
      throw new Error('sql/db: query did not return ack')
    }

    const columns = ack.body.value.columns ?? []
    const rows: Row[] = []
    try {
      for (;;) {
        requests.push({
          body: {
            case: 'next',
            value: 100,
          },
        })
        const resp = await nextQueryResponse(iterator)
        const body = resp.body
        if (body?.case === 'reqError') {
          throw new Error(body.value)
        }
        if (body?.case === 'closed') {
          return { columns, rows }
        }
        if (body?.case !== 'batch') {
          throw new Error('sql/db: query expected row batch or close')
        }
        rows.push(...(body.value.rows ?? []))
      }
    } finally {
      requests.end()
      abortSignal?.throwIfAborted()
    }
  }

  // commit commits the transaction.
  public async commit(abortSignal?: AbortSignal): Promise<void> {
    if (this.released) throw new Error('sql/db: transaction already released')
    this.released = true
    this.requests.push({
      body: {
        case: 'commit',
        value: true,
      },
    })
    try {
      const resp = await nextTransactionResponse(this.responses)
      const complete = resp.body?.case === 'complete' ? resp.body.value : null
      if (!complete) throw new Error('sql/db: transaction did not complete')
      if (complete.error) throw new Error(complete.error)
      if (!complete.committed) {
        throw new Error('sql/db: transaction discarded')
      }
    } finally {
      this.requests.end()
      abortSignal?.throwIfAborted()
    }
  }

  // discard discards the transaction.
  public async discard(): Promise<void> {
    if (this.released) return
    this.released = true
    this.requests.push({
      body: {
        case: 'discard',
        value: true,
      },
    })
    try {
      await nextTransactionResponse(this.responses)
    } finally {
      this.requests.end()
    }
  }
}

async function nextTransactionResponse(
  iterator: AsyncIterator<Message<SqlTransactionResponse>>,
): Promise<Message<SqlTransactionResponse>> {
  const next = await iterator.next()
  if (next.done) throw new Error('sql/db: transaction stream closed')
  return next.value
}

async function nextQueryResponse(
  iterator: AsyncIterator<Message<SqlQueryResponse>>,
): Promise<Message<SqlQueryResponse>> {
  const next = await iterator.next()
  if (next.done) throw new Error('sql/db: query stream closed')
  return next.value
}

function sqlValueToString(value?: SqlValue): string[] {
  if (!value?.value) return []
  switch (value.value.case) {
    case 'strValue':
      return [value.value.value]
    case 'intValue':
      return [value.value.value.toString()]
    case 'floatValue':
      return [value.value.value.toString()]
    case 'blobValue':
      return [new TextDecoder().decode(value.value.value)]
    default:
      return []
  }
}

function quoteIdentifier(value: string): string {
  return `\`${value.replaceAll('`', '``')}\``
}
