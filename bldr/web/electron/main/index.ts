import electron from 'electron'
import path from 'path'

import {
  ElectronInit,
  type ElectronInit as ElectronInitType,
} from '../../plugin/electron/electron.pb.js'
import { BldrElectronApp } from './app.js'

// BLDR_DEBUG is set if this is a debug build.
declare const BLDR_DEBUG: boolean | undefined

const app = electron.app

// immediately configure the data directory to cwd
const userDataPath =
  process.env['BLDR_PLUGIN_STATE_PATH'] || path.join(process.cwd(), 'userData')
app.setPath('userData', userDataPath)

// add some electron flags
if (typeof BLDR_DEBUG === 'boolean' && BLDR_DEBUG) {
  // enables pasting in the devtools without "allow pasting"
  // https://github.com/electron/electron/issues/40995
  app.commandLine.appendSwitch('--unsafely-disable-devtools-self-xss-warnings')
}

// Decode ElectronInit from base64-encoded env var
let electronInit: ElectronInitType = {}
const initB64 = process.env['BLDR_ELECTRON_INIT']
if (initB64) {
  electronInit = ElectronInit.fromBinary(Buffer.from(initB64, 'base64'))
}

const webRuntimeId: string = process.env['BLDR_RUNTIME_ID'] || 'default'
new BldrElectronApp(electron.app, webRuntimeId, electronInit).init()
