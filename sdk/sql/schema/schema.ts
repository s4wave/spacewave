import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import { SqlSchemaResourceServiceClient } from './schema_srpc.pb.js'
import type { GetSchemaResponse, ListTablesResponse } from './schema.pb.js'

// SqlSchemaTypeID is the world ObjectType id for SQL schemas.
export const SqlSchemaTypeID = 'sql/schema'

// SqlSchema is the typed SDK handle for a sql/schema object.
export class SqlSchema extends Resource {
  private service: SqlSchemaResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SqlSchemaResourceServiceClient(resourceRef.client)
  }

  // getSchema returns schema metadata.
  public async getSchema(
    abortSignal?: AbortSignal,
  ): Promise<GetSchemaResponse> {
    return this.service.GetSchema({}, abortSignal)
  }

  // listTables lists tables in the target sql/db schema.
  public async listTables(
    abortSignal?: AbortSignal,
  ): Promise<ListTablesResponse> {
    return this.service.ListTables({}, abortSignal)
  }
}
