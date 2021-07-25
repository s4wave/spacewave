const electron = require('electron');
const net = require('net');
const path = require('path');

const app = electron.app;
const BrowserWindow = electron.BrowserWindow;
const ipcMain = electron.ipcMain;

var mainWindow;

/*
const {
    default: installExtension,
    REACT_DEVELOPER_TOOLS
} = require('electron-devtools-installer');
*/

function createWindow() {
    mainWindow = new BrowserWindow({
        frame: false,
        height: 680,
        width: 900,
        webPreferences: {
            preload: path.join(app.getAppPath(), 'preload.js'),
        },
    });

    mainWindow.webContents.openDevTools();
    // installExtension(REACT_DEVELOPER_TOOLS)
    mainWindow.loadURL('http://localhost:5100');
    mainWindow.on('closed', () => mainWindow = null);

    process.stdin.on('data', (data) => {
        mainWindow.webContents.send('runtime-data', data)
    });

    let ipcPath = path.join(process.cwd(), '..', '.pipe')
    if (process.platform === 'win32') {
        ipcPath = path.join('\\\\.\\pipe', process.cwd(), '..', 'pipe')
    }
    let socket = net.connect(ipcPath, function () {
        socket.on('data', (data) => {
            mainWindow.webContents.send('runtime-data', data);
        })
        ipcMain.on('runtime-data', (event, data) => {
            socket.write(data);
        })
    }).on('error', function (err) {
        console.error(err)
        process.exit(1);
    })
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