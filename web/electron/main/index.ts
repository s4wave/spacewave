import electron from 'electron'
import path from 'path'
import { BldrElectronApp } from './app.js'

const app = electron.app

// immediately configure the data directory to cwd
const userDataPath =
  process.env['BLDR_PLUGIN_STATE_PATH'] || path.join(process.cwd(), 'userData')
app.setPath('userData', userDataPath)

const webRuntimeId: string = process.env['BLDR_RUNTIME_ID'] || 'default'
new BldrElectronApp(electron.app, webRuntimeId).init()
