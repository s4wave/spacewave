import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import { SqlTableViewResourceServiceClient } from './table-view_srpc.pb.js'
import type {
  FetchRowsResponse,
  GetTableViewResponse,
} from './table-view.pb.js'

// SqlTableViewTypeID is the world ObjectType id for SQL table views.
export const SqlTableViewTypeID = 'sql/table-view'

// SqlTableView is the typed SDK handle for a sql/table-view object.
export class SqlTableView extends Resource {
  private service: SqlTableViewResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SqlTableViewResourceServiceClient(resourceRef.client)
  }

  // getTableView returns table view metadata.
  public async getTableView(
    abortSignal?: AbortSignal,
  ): Promise<GetTableViewResponse> {
    return this.service.GetTableView({}, abortSignal)
  }

  // fetchRows executes the saved table view SELECT.
  public async fetchRows(
    abortSignal?: AbortSignal,
  ): Promise<FetchRowsResponse> {
    return this.service.FetchRows({}, abortSignal)
  }
}
