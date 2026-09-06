import type { BackendAPI } from '@aptre/bldr-sdk'

import { TestbedRoot } from '../../../sdk/testbed/testbed.js'
import { TestbedResourceServiceClient } from '../../../sdk/testbed/testbed_srpc.pb.js'
import testMain from './testbed-e2e.js'

// main runs the plugin test and reports its result through the testbed service.
export default async function main(
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
) {
  let success = false
  let errorMsg = ''

  try {
    // ResourceClient carries the client session context needed to attach child resources.
    using rootRef = await backendAPI.resourceClient.accessRootResource()
    const testbedRoot = new TestbedRoot(rootRef)
    console.log('testbed wrapper: testbed root ready')

    console.log('testbed wrapper: running test...')
    await testMain(backendAPI, abortSignal, testbedRoot)

    success = true
    console.log('testbed wrapper: test completed successfully')
  } catch (err) {
    success = false
    errorMsg = String(err)
    console.error('testbed wrapper: test failed:', err)
  }

  // Report failures as well as successful completion to the testbed.
  try {
    console.log('testbed wrapper: marking test result...')
    const testbedService = new TestbedResourceServiceClient(backendAPI.client)
    await testbedService.MarkTestResult({
      success,
      errorMessage: errorMsg,
    })
    console.log('testbed wrapper: test result marked')
  } catch (err) {
    console.error('testbed wrapper: failed to mark test result:', err)
  }

  if (!success) {
    throw new Error('test failed: ' + errorMsg)
  }
}
