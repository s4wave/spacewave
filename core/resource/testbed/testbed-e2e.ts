import type { BackendAPI } from '@aptre/bldr-sdk'
import type { TestbedRoot } from '../../../sdk/testbed/testbed.js'
import { EngineWorldState } from '../../../sdk/world/engine-state.js'

export default async function main(
  backendAPI: BackendAPI,
  abortSignal: AbortSignal,
  testbedRoot: TestbedRoot,
) {
  console.log('waiting for plugin info...')
  const pluginInfo = await backendAPI.pluginHost.GetPluginInfo({}, abortSignal)
  console.log(
    'loaded plugin info',
    backendAPI.protos.GetPluginInfoResponse.toJsonString(pluginInfo),
  )

  // create a world engine
  console.log('creating world engine...')
  using engine = await testbedRoot.createWorld('test-engine-ts')
  console.log('created world engine')

  // wrap engine in EngineWorldState to get WorldState operations
  const worldState = new EngineWorldState(engine, true)

  // create an object in the world to signal completion
  console.log('creating completion marker object...')
  using _objState = await worldState.createObject('e2e-test-completed', {})
  console.log('created completion marker object')

  // get the object back to verify
  using objState2 = await worldState.getObject('e2e-test-completed')
  if (!objState2) {
    throw new Error('failed to retrieve completion marker object')
  }
  console.log('successfully retrieved completion marker object')

  // done
  console.log('testbed e2e test completed successfully')
}
