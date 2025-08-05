import type { BackendAPI } from '../../../sdk/plugin.js'
import * as mock from 'starpc/mock'

export default async function main(backendAPI: BackendAPI) {
  console.log('waiting for plugin info...')
  const pluginInfo = await backendAPI.pluginHost.GetPluginInfo({})
  console.log(
    'loaded plugin info',
    backendAPI.protos.GetPluginInfoResponse.toJsonString(pluginInfo),
  )

  // build a client for the mock service
  const mockClient = new mock.MockClient(backendAPI.client)
  const resp = await mockClient.MockRequest({body: 'hello from the quickjs test plugin'})
  console.log('received response from MockRequest', mock.MockMsg.toJsonString(resp))
}
