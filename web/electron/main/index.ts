import electron from 'electron'
import path from 'path'
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

const webRuntimeId: string = process.env['BLDR_RUNTIME_ID'] || 'default'
new BldrElectronApp(electron.app, webRuntimeId).init()
