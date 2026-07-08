import { castToError, Client as SRPCClient } from 'starpc'
import type { BackendAPI } from '@aptre/bldr-sdk'
import { Client as ResourcesClient, ResourceServiceClient } from '@s4wave/sdk'
import { Root } from '@s4wave/sdk/root'
import { TestbedResourceServiceClient } from '@s4wave/sdk/testbed/testbed_srpc.pb.js'

import { testProvider } from './provider.js'
import { testHashFunctions } from './hash.js'
import { testTypedObject } from './typed-object.js'

type CoreE2ETest = (
  rootResource: Root,
  abortSignal: AbortSignal,
) => Promise<void>

const coreE2ETests: Record<string, CoreE2ETest> = {
  provider: testProvider,
  hash: testHashFunctions,
  typedObject: testTypedObject,
}

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

  const testbedService = new TestbedResourceServiceClient(backendAPI.client)

  try {
    const corePluginClient = new SRPCClient(
      backendAPI.buildPluginOpenStream('spacewave-core'),
    )
    const resourcesService = new ResourceServiceClient(corePluginClient)
    const resourcesClient = new ResourcesClient(resourcesService, abortSignal)

    using rootResourceRef = await resourcesClient.accessRootResource()
    using rootResource = new Root(rootResourceRef)
    console.log('created root resource handle')

    await runQueuedCoreE2ETests(testbedService, rootResource, abortSignal)

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

async function runQueuedCoreE2ETests(
  testbedService: TestbedResourceServiceClient,
  rootResource: Root,
  abortSignal: AbortSignal,
) {
  const running = new Set<Promise<void>>()

  while (true) {
    const claim = await testbedService.ClaimTest({}, abortSignal)
    if (claim.closed) {
      break
    }
    if (!claim.testName) {
      throw new Error('received empty core e2e test name')
    }

    const run = runCoreE2ETest(
      claim.testName,
      testbedService,
      rootResource,
      abortSignal,
    ).finally(() => {
      running.delete(run)
    })
    running.add(run)
  }

  const results = await Promise.allSettled(Array.from(running))
  const failure = results.find((result) => result.status === 'rejected')
  if (failure?.status === 'rejected') {
    throw failure.reason
  }
}

async function runCoreE2ETest(
  testName: string,
  testbedService: TestbedResourceServiceClient,
  rootResource: Root,
  abortSignal: AbortSignal,
) {
  try {
    const test = coreE2ETests[testName]
    if (!test) {
      throw new Error(`unknown core e2e test: ${testName}`)
    }

    await test(rootResource, abortSignal)
    await testbedService.MarkTestResult(
      { success: true, errorMessage: '', testName },
      abortSignal,
    )
  } catch (error) {
    const errorMessage = castToError(error).message
    await testbedService.MarkTestResult(
      { success: false, errorMessage, testName },
      abortSignal,
    )
    throw error
  }
}
