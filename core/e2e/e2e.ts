import { castToError, Client as SRPCClient } from 'starpc'
import type { BackendAPI } from '@aptre/bldr-sdk'
import { Client as ResourcesClient, ResourceServiceClient } from '@s4wave/sdk'
import { Root } from '@s4wave/sdk/root'
import { TestbedResourceServiceClient } from '@s4wave/sdk/testbed/testbed_srpc.pb.js'

import { testProvider } from './provider.js'
import { testHashFunctions } from './hash.js'
import { testTypedObject } from './typed-object.js'

export default async function main(
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
) {
  console.log('waiting for plugin info...')
  const pluginInfo = await backendAPI.pluginHost.GetPluginInfo({}, abortSignal)
  console.log(
    'loaded plugin info',
    backendAPI.protos.GetPluginInfoResponse.toJsonString(pluginInfo),
  )

  // testbed service is registered directly on testbed mux, reachable via bus
  // fallback (hostMux -> LookupRpcService -> testbed RpcServiceController)
  const testbedService = new TestbedResourceServiceClient(backendAPI.client)

  try {
    // get the rpc client for the spacewave-core plugin
    const corePluginClient = new SRPCClient(
      backendAPI.buildPluginOpenStream('spacewave-core'),
    )

    // construct the resources client for spacewave-core
    const resourcesService = new ResourceServiceClient(corePluginClient)
    const resourcesClient = new ResourcesClient(resourcesService, abortSignal)

    // access the root resource, starting the rpc request
    // the "using" triggers a Dispose when this function returns
    using rootResourceRef = await resourcesClient.accessRootResource()
    using rootResource = new Root(rootResourceRef)
    console.log('created root resource handle')

    // run test cases
    await testProvider(rootResource, abortSignal)
    await testHashFunctions(rootResource, abortSignal)
    console.log('about to run testTypedObject...')
    await testTypedObject(rootResource, abortSignal)
    console.log('testTypedObject completed!')

    console.log('test completed successfully')
    await testbedService.MarkTestResult(
      { success: true, errorMessage: '' },
      abortSignal,
    )
  } catch (error) {
    const errorMessage = castToError(error).message
    await testbedService.MarkTestResult(
      { success: false, errorMessage },
      abortSignal,
    )
    console.error('test failed:', errorMessage)
    throw error
  }
}
