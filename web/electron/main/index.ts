import electron from 'electron'
import net from 'net'
import path from 'path'

var mainWindow: electron.BrowserWindow | null
const app = electron.app
var ipcMain: Electron.IpcMain = electron.ipcMain

/*
const {
    default: installExtension,
    REACT_DEVELOPER_TOOLS
} = require('electron-devtools-installer');
*/

// setup handler for MessagePort updates.
let runtimePort: Electron.MessagePortMain | undefined
function setupRuntimePort() {
  let ipcPath = path.join(process.cwd(), '.pipe')
  if (process.platform === 'win32') {
    ipcPath = path.join('\\\\.\\pipe', process.cwd(), '.pipe')
  }
  let socket = net
    .connect(ipcPath, () => {
      socket.on('data', (data) => {
        if (runtimePort) {
          runtimePort.postMessage(data)
        }
      })
    })
    .on('error', function (err) {
      console.error(err)
      process.exit(1)
    })
  ipcMain.on('BLDR_PORT', (event) => {
    const ports = event.ports
    if (!ports || !ports.length) {
      return
    }
    if (runtimePort) {
      runtimePort.close()
    }
    runtimePort = ports[0]
    runtimePort.on('message', (event) => {
      const data = event?.data as Uint8Array
      if (data && data.length) {
        socket.write(data)
      }
    })
    runtimePort.start()
  })
}

function createWindow() {
  console.log('preload: ' + path.join(app.getAppPath(), 'preload.js'))
  mainWindow = new electron.BrowserWindow({
    frame: false,
    height: 680,
    width: 900,
    webPreferences: {
      sandbox: true,
      nodeIntegration: false,
      contextIsolation: true,
      // enableRemoteModule: false,
      preload: path.join(app.getAppPath(), 'preload.js'),
    },
  })

  // installExtension(REACT_DEVELOPER_TOOLS)
  // mainWindow.loadURL('http://localhost:5100');
  // mainWindow.webContents.openDevTools()

  mainWindow.loadFile('index.html')
  mainWindow.on('closed', () => (mainWindow = null))
}

function startup() {
  setupRuntimePort()
  createWindow()
}

app.on('ready', startup)

app.on('window-all-closed', () => {
  app.quit()
})

app.on('activate', () => {
  if (mainWindow === null) {
    createWindow()
  }
})
