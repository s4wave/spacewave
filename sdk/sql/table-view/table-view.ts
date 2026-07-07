import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import type { SqlValue } from '../../../db/sql/sql.pb.js'
import { SqlTableViewResourceServiceClient } from './table-view_srpc.pb.js'
import type {
  FetchRowsResponse,
  GetDriverCapabilityResponse,
  GetTableViewResponse,
  UpdateRowResponse,
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

  // getDriverCapability returns SQL driver operations available to this view.
  public async getDriverCapability(
    abortSignal?: AbortSignal,
  ): Promise<GetDriverCapabilityResponse> {
    return this.service.GetDriverCapability({}, abortSignal)
  }

  // fetchRows executes the saved table view SELECT.
  public async fetchRows(
    abortSignal?: AbortSignal,
  ): Promise<FetchRowsResponse> {
    return this.service.FetchRows({}, abortSignal)
  }

  // updateRow applies a typed UPDATE against rows matching the supplied values.
  public async updateRow(
    matchColumns: string[],
    matchValues: SqlValue[],
    setColumns: string[],
    setValues: SqlValue[],
    abortSignal?: AbortSignal,
  ): Promise<UpdateRowResponse> {
    return this.service.UpdateRow(
      { matchColumns, matchValues, setColumns, setValues },
      abortSignal,
    )
  }
}
