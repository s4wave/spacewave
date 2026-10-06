const { app, BrowserWindow } = require('electron')

// Match the desktop entrypoint's profile selection before requesting the lock.
app.setPath('userData', process.env.BLDR_PLUGIN_STATE_PATH)

if (!app.requestSingleInstanceLock()) {
  process.stdout.write('denied\n')
  app.quit()
} else {
  let window

  app.on('second-instance', () => {
    window.focus()
    process.stdout.write('focused\n')
  })

  process.stdin.on('data', (command) => {
    if (command.toString().trim() === 'quit') app.quit()
  })

  app.whenReady().then(() => {
    window = new BrowserWindow({ show: false })
    process.stdout.write('ready\n')
  })
}
