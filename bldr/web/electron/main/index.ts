import electron from 'electron'
import path from 'path'

import {
  ElectronInit,
  type ElectronInit as ElectronInitType,
} from '../../plugin/electron/electron.pb.js'
import { BldrElectronApp } from './app.js'
import { ignoreClosedProcessStreamErrors } from './process-stream.js'

// BLDR_DEBUG is set if this is a debug build.
declare const BLDR_DEBUG: boolean | undefined

const app = electron.app

ignoreClosedProcessStreamErrors()

// Each plugin instance owns its Electron profile for its complete lifetime.
const userDataPath =
  process.env['BLDR_PLUGIN_STATE_PATH'] || path.join(process.cwd(), 'userData')
app.setPath('userData', userDataPath)

// Development builds permit pasting into their interactive developer tools.
if (typeof BLDR_DEBUG === 'boolean' && BLDR_DEBUG) {
  // enables pasting in the devtools without "allow pasting"
  // https://github.com/electron/electron/issues/40995
  app.commandLine.appendSwitch('--unsafely-disable-devtools-self-xss-warnings')
}

// Operators can inspect an installed release without replacing it with a
// development build. Inspection remains opt-in and bound to loopback.
const remoteDebuggingPort = process.env['BLDR_ELECTRON_REMOTE_DEBUGGING_PORT']
if (remoteDebuggingPort) {
  const port = Number(remoteDebuggingPort)
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(
      'BLDR_ELECTRON_REMOTE_DEBUGGING_PORT must be a valid TCP port',
    )
  }
  app.commandLine.appendSwitch('remote-debugging-address', '127.0.0.1')
  app.commandLine.appendSwitch('remote-debugging-port', String(port))
}

// The parent process supplies the generated startup message through the
// environment before this Electron process creates any windows.
let electronInit: ElectronInitType = {}
const initB64 = process.env['BLDR_ELECTRON_INIT']
if (initB64) {
  electronInit = ElectronInit.fromBinary(Buffer.from(initB64, 'base64'))
}

const webRuntimeId = process.env['BLDR_RUNTIME_ID'] || 'default'
new BldrElectronApp(electron.app, webRuntimeId, electronInit).init()
