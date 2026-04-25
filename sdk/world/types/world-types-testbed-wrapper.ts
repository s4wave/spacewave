import type { BackendAPI } from '@aptre/bldr-sdk'
import { Client as ResourcesClient } from '@aptre/bldr-sdk/resource/client.js'
import { ResourceServiceClient } from '@go/github.com/s4wave/spacewave/bldr/resource/resource_srpc.pb.js'
import { TestbedRoot } from '../../../sdk/testbed/testbed.js'
import testMain from './types-testbed.js'

export default async function main(
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
) {
  let testbedRoot: TestbedRoot | undefined
  let success = false
  let errorMsg = ''

  try {
    console.log('testbed wrapper: setting up resources client...')

    // construct the resources client
    const resourcesService = new ResourceServiceClient(backendAPI.client)
    const resourcesClient = new ResourcesClient(resourcesService, abortSignal)

    // access the root resource (testbed)
    using rootResourceRef = await resourcesClient.accessRootResource()
    console.log('testbed wrapper: created root resource handle')

    // create testbed root resource
    testbedRoot = new TestbedRoot(rootResourceRef)
    console.log('testbed wrapper: testbed root ready')

    // run the test
    console.log('testbed wrapper: running test...')
    await testMain(backendAPI, abortSignal, testbedRoot)

    success = true
    console.log('testbed wrapper: test completed successfully')
  } catch (err) {
    success = false
    errorMsg = String(err)
    console.error('testbed wrapper: test failed:', err)
  }

  // report result to testbed root
  try {
    if (testbedRoot) {
      console.log('testbed wrapper: marking test result...')
      await testbedRoot.markTestResult(success, errorMsg)
      console.log('testbed wrapper: test result marked')
    }
  } catch (err) {
    console.error('testbed wrapper: failed to mark test result:', err)
  }

  if (!success) {
    throw new Error('test failed: ' + errorMsg)
  }
}
