const electron = require('electron');
const app = electron.app;
const ipcMain = electron.ipcMain;
const BrowserWindow = electron.BrowserWindow;

const child_process = require('child_process');
const path = require('path');

let mainWindow;

function loadRuntime() {
  console.log('Starting up the native component...');
  var nativeComponent = child_process.execFile('./resources/app.asar/bin/runtime.exe', [], {
    cwd: process.cwd(),
    encoding: 'buffer',
  }, (err) => {
    console.log('runtime exited with error');
    console.log(err);
  })

  // Process the stdout stream as a header, message sequence.
  // Bind to on('data'), write data to a buffer, then process buffer.
  nativeComponent.stdout.on('data', (data) => {
    mainWindow.webContents.send('runtime-data', data);
  });
  ipcMain.on('runtime-data', (event, data) => {
    if (!nativeComponent || !nativeComponent.stdin) {
      return;
    }
    
    nativeComponent.stdin.write(data);
  })
  // nativeComponent.stdout.pipe(process.stdout);
  nativeComponent.stderr.pipe(process.stderr);
}

function createWindow() {
  mainWindow = new BrowserWindow({
    frame: false,
    height: 680,
    width: 900,
    webPreferences: {
      preload: path.join(app.getAppPath(), 'preload.js'),
    },
    /*
    webPreferences: {
      nodeIntegration: true
    }
    */
    // nodeIntegrationInWorker: true,
  });

  // mainWindow.webContents.openDevTools();
  mainWindow.loadFile("index.html");
  mainWindow.on('closed', () => mainWindow = null);
  loadRuntime()
}

app.on('ready', createWindow);

app.on('window-all-closed', () => {
  app.quit();
});

app.on('activate', () => {
  if (mainWindow === null) {
    createWindow();
  }
});

