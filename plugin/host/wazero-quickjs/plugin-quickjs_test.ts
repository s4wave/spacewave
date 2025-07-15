import type { BackendAPI } from '../../../sdk/plugin.js'

export default async function main(backendAPI: BackendAPI) {
  console.log('waiting for plugin info...')
  const pluginInfo = await backendAPI.pluginHost.GetPluginInfo({})
  console.log(
    'loaded plugin info',
    backendAPI.protos.GetPluginInfoResponse.toJsonString(pluginInfo),
  )
}
