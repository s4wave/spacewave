import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import { SqlWorkbenchResourceServiceClient } from './workbench_srpc.pb.js'
import type {
  GetWorkbenchResponse,
  WorkbenchLayout,
  WorkbenchTab,
} from './workbench.pb.js'

// SqlWorkbenchTypeID is the world ObjectType id for SQL workbenches.
export const SqlWorkbenchTypeID = 'sql/workbench'

// SqlWorkbenchBlockTypeID is the block type id for SQL workbench roots.
export const SqlWorkbenchBlockTypeID =
  'github.com/s4wave/spacewave/sdk/sql/workbench.Workbench'

// SqlWorkbench is the typed SDK handle for a sql/workbench object.
export class SqlWorkbench extends Resource {
  private service: SqlWorkbenchResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SqlWorkbenchResourceServiceClient(resourceRef.client)
  }

  // getWorkbench returns persisted workbench state.
  public async getWorkbench(
    abortSignal?: AbortSignal,
  ): Promise<GetWorkbenchResponse> {
    return this.service.GetWorkbench({}, abortSignal)
  }

  // addPin pins a sql/query object.
  public async addPin(
    queryObjectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.AddPin({ queryObjectKey }, abortSignal)
  }

  // removePin removes a pinned sql/query object.
  public async removePin(
    queryObjectKey: string,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.RemovePin({ queryObjectKey }, abortSignal)
  }

  // setLayout replaces open tabs and layout preferences.
  public async setLayout(
    openTabs: WorkbenchTab[],
    layout?: WorkbenchLayout,
    abortSignal?: AbortSignal,
  ): Promise<void> {
    await this.service.SetLayout({ openTabs, layout }, abortSignal)
  }
}
