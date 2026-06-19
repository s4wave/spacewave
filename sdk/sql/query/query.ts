import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import type { SqlValue } from '../../../db/sql/sql.pb.js'
import { SqlQueryResourceServiceClient } from './query_srpc.pb.js'
import type { GetQueryTextResponse, RunQueryResponse } from './query.pb.js'

// SqlQueryTypeID is the world ObjectType id for SQL queries.
export const SqlQueryTypeID = 'sql/query'

// SqlQueryBlockTypeID is the block type id for SQL query roots.
export const SqlQueryBlockTypeID =
  'github.com/s4wave/spacewave/sdk/sql/query.Query'

// SqlQuery is the typed SDK handle for a sql/query object.
export class SqlQuery extends Resource {
  private service: SqlQueryResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SqlQueryResourceServiceClient(resourceRef.client)
  }

  // getQueryText returns query text and target metadata.
  public async getQueryText(
    abortSignal?: AbortSignal,
  ): Promise<GetQueryTextResponse> {
    return this.service.GetQueryText({}, abortSignal)
  }

  // setQueryText updates query text and target metadata.
  public async setQueryText(
    sqlText: string,
    dialectHint: string,
    targetDbObjectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.SetQueryText(
      { sqlText, dialectHint, targetDbObjectKey },
      abortSignal,
    )
  }

  // setParameters updates positional bind arguments.
  public async setParameters(
    parameters: SqlValue[],
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.SetParameters({ parameters }, abortSignal)
  }

  // run executes the query and creates a query result object.
  public async run(
    maxRows = 0,
    abortSignal?: AbortSignal,
  ): Promise<RunQueryResponse> {
    return this.service.Run({ maxRows }, abortSignal)
  }
}
