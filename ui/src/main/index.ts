import { app, shell, BrowserWindow } from 'electron'
import { join } from 'path'
import { electronApp, optimizer, is } from '@electron-toolkit/utils'
import icon from '../../resources/icon.png?asset'
import {
  beginShutdown,
  hasCodexShells,
  killAllShells,
  registerPtyHandlers,
  stopCodexShells
} from './pty'
import { registerTabSessionHandlers } from './tabSessions'
import { registerAccountHandlers } from './accounts'
import { registerProjectHandlers } from './projects'
import { setTrustBinary } from './projectTrust'
import { registerWorkspaceLayoutHandlers } from './workspaceLayouts'
import { registerBiblioHandlers, registerBiblioImageScheme, closeBiblioWatchers } from './biblio'
import { registerGitHandlers } from './git'
import { registerVaultHandlers } from './vault'
import { registerNodeHandlers } from './node'
import { registerWindowControlHandlers, wireMaximizeEvents } from './windowControls'
import { installLgrass, installLgrassd, installLgrassconf } from './lgrassInstall'
import { ensureDaemonsRunning, killDaemons } from './daemons'
import { startWorkgroupBridge, stopWorkgroupBridge } from './workgroupBridge'
import { registerWorkgroupHandlers } from './workgroups'
import { destroyAllInstances } from './web'

let mainWindow: BrowserWindow | undefined

function createWindow(): void {
  let closing = false
  const win = new BrowserWindow({
    width: 900,
    height: 670,
    show: false,
    frame: false,
    autoHideMenuBar: true,
    ...(process.platform === 'linux' ? { icon } : {}),
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      sandbox: false
    }
  })
  mainWindow = win
  wireMaximizeEvents(win)

  win.on('ready-to-show', () => {
    win.show()
  })

  win.on('close', (event) => {
    if (closing) return
    event.preventDefault()
    closing = true
    beginShutdown()
    if (hasCodexShells()) win.webContents.send('window:closing')
    void stopCodexShells().finally(() => win.destroy())
  })

  // Otherwise mainWindow keeps pointing at a destroyed BrowserWindow after
  // close. Accessing .webContents on it throws "Object has been
  // destroyed" for any handler still holding this getter.
  win.on('closed', () => {
    mainWindow = undefined
    destroyAllInstances()
  })

  win.webContents.setWindowOpenHandler((details) => {
    shell.openExternal(details.url)
    return { action: 'deny' }
  })

  if (is.dev && process.env['ELECTRON_RENDERER_URL']) {
    win.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    win.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

registerBiblioImageScheme()

app.whenReady().then(() => {
  electronApp.setAppUserModelId('com.lemongrass.app')

  // Toggles DevTools with F12 in dev, blocks CommandOrControl+R in prod.
  app.on('browser-window-created', (_, window) => {
    optimizer.watchWindowShortcuts(window, { zoom: true })
  })

  const lgrassPath = installLgrass()
  const lgrassdPath = installLgrassd()
  installLgrassconf()
  if (lgrassdPath) {
    void ensureDaemonsRunning(lgrassdPath)
  }

  registerPtyHandlers(() => mainWindow?.webContents)
  registerAccountHandlers(lgrassdPath)
  setTrustBinary(lgrassdPath)
  registerProjectHandlers(() => mainWindow)
  registerWorkspaceLayoutHandlers()
  registerTabSessionHandlers(lgrassdPath)
  registerBiblioHandlers(() => mainWindow?.webContents)
  registerGitHandlers()
  registerVaultHandlers()
  registerNodeHandlers()
  registerWindowControlHandlers(() => mainWindow)
  startWorkgroupBridge(() => mainWindow, lgrassdPath)
  registerWorkgroupHandlers(lgrassPath)

  createWindow()

  app.on('activate', function () {
    // On macOS it's common to re-create a window in the app when the
    // dock icon is clicked and there are no other windows open.
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
  })
})

app.on('before-quit', () => stopWorkgroupBridge())

// macOS apps conventionally stay alive with no windows open until Cmd+Q.
app.on('window-all-closed', () => {
  killAllShells()
  killDaemons()
  closeBiblioWatchers()
  if (process.platform !== 'darwin') {
    app.quit()
  }
})
