const electron = require('electron');
const chokidar = require('chokidar');

const app = electron.app;
const BrowserWindow = electron.BrowserWindow;
const ipcMain = electron.ipcMain;

const child_process = require('child_process');

var mainWindow;
var runtimeProc;

// const { default: installExtension, REACT_DEVELOPER_TOOLS } = require('electron-devtools-installer');

function createWindow() {
  mainWindow = new BrowserWindow({
    frame: false,
    height: 680,
    width: 900,
  });

  mainWindow.webContents.openDevTools();
  // installExtension(REACT_DEVELOPER_TOOLS)
  mainWindow.loadURL('http://localhost:5100');
  mainWindow.on('closed', () => mainWindow = null);

  function loadRuntime() {
    if (runtimeProc) {
      console.log('Runtime change detected, restarting.');
      runtimeProc.kill();
      runtimeProc = null;
    }

    console.log('Starting runtime...');
    runtimeProc = child_process.execFile(exe, [], {
      encoding: 'buffer',
      cwd: process.cwd(),
    }, (err) => {
      console.log('runtime exited');
    });

    runtimeProc.stdout.on('data', (data) => {
      mainWindow.webContents.send('runtime-data', data)
    });
    runtimeProc.stderr.pipe(process.stderr);
  }

  ipcMain.on('runtime-data', (event, data) => {
    if (!runtimeProc || !runtimeProc.stdin) {
      return
    }
    
    runtimeProc.stdin.write(data);
  })

  const exe = '../desktop/bin/runtime.exe';
  const runtimeWatcher = chokidar.watch(exe);
  runtimeWatcher
    .on('add', loadRuntime)
    .on('change', loadRuntime)
    .on('unlink', function () {
      if (!runtimeProc) {
        return;
      }

      runtimeProc.kill();
      runtimeProc = null;
    });
}

app.on('ready', createWindow);
app.on('window-all-closed', () => {
  app.quit();
  if (runtimeProc) {
    runtimeProc.kill();
  }
});

app.on('activate', () => {
  if (mainWindow === null) {
    createWindow();
  }
});
