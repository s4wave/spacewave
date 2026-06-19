import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import { SqlQueryResultResourceServiceClient } from './query-result_srpc.pb.js'
import type { GetResultGridResponse } from './query-result.pb.js'

// SqlQueryResultTypeID is the world ObjectType id for SQL query results.
export const SqlQueryResultTypeID = 'sql/query-result'

// SqlQueryResult is the typed SDK handle for a sql/query-result object.
export class SqlQueryResult extends Resource {
  private service: SqlQueryResultResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SqlQueryResultResourceServiceClient(resourceRef.client)
  }

  // getResultGrid returns the persisted result grid.
  public async getResultGrid(
    abortSignal?: AbortSignal,
  ): Promise<GetResultGridResponse> {
    return this.service.GetResultGrid({}, abortSignal)
  }
}
