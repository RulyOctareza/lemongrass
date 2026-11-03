// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Smoke run for a packaged build: launches it, waits for the header, opens a Shell tab and saves a screenshot.
// Usage: node smoke/packaged.mjs <Lemongrass.app | linux-unpacked dir | executable> [out dir]
//
// The app runs with HOME set to a fresh temp directory seeded with one trusted project, so the self-install of
// lgrass, lgrassd and lgrassconf lands there. That isolation is incomplete on both systems, so run it only where the
// machine is disposable or the leaks are harmless:
// - macOS: `lgrassconf install` boots out and replaces the LaunchAgent of the same label, so on a Mac in use it would
//   replace the real keeper. CI runners only.
// - Linux: the systemd user manager ignores HOME, so XDG_RUNTIME_DIR and DBUS_SESSION_BUS_ADDRESS are removed below to
//   make `systemctl --user` fail instead of enabling a unit for the real user.

// Plain JavaScript run by node directly, so there are no types to declare.
/* eslint-disable @typescript-eslint/explicit-function-return-type */

import { _electron as electron } from 'playwright-core'
import { execFileSync } from 'node:child_process'
import { randomUUID } from 'node:crypto'
import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'

const STEP_TIMEOUT = 60_000

function executableFor(target) {
  const resolved = path.resolve(target)
  if (resolved.endsWith('.app')) {
    const name = path.basename(resolved, '.app')
    return path.join(resolved, 'Contents', 'MacOS', name)
  }
  if (fs.statSync(resolved).isDirectory()) return path.join(resolved, 'lemongrass')
  return resolved
}

function seedHome() {
  // /tmp rather than os.tmpdir(): the per-user temp path on macOS is long, and the unix sockets lgrassd opens in
  // the temp home must fit the 104-byte socket path limit there.
  const base = process.platform === 'win32' ? os.tmpdir() : '/tmp'
  const home = fs.mkdtempSync(path.join(base, 'lg-smoke-'))
  const project = path.join(home, 'smoke-project')
  fs.mkdirSync(project)
  fs.mkdirSync(path.join(home, '.lemongrass'))
  const projects = [
    { id: randomUUID(), name: 'smoke-project', path: fs.realpathSync(project), trusted: true }
  ]
  fs.writeFileSync(
    path.join(home, '.lemongrass', 'projects.json'),
    JSON.stringify(projects, null, 2)
  )
  return home
}

// The environment of the shell that started the smoke run must not leak in: LGRASS_* would make the app's tabs look
// like tabs of the lemongrass this runs under, and XDG_* would point config and data back at the real home.
function appEnv(home) {
  const env = {}
  for (const [key, value] of Object.entries(process.env)) {
    if (key.startsWith('LGRASS_') || key.startsWith('XDG_')) continue
    if (key === 'DBUS_SESSION_BUS_ADDRESS') continue
    env[key] = value
  }
  env.HOME = home
  return env
}

function processTable() {
  try {
    return execFileSync('ps', ['-axo', 'pid=,ppid=,command='], { encoding: 'utf8' })
      .split('\n')
      .map((line) => /^\s*(\d+)\s+(\d+)\s+(.*)$/.exec(line))
      .filter(Boolean)
      .map((m) => ({ pid: Number(m[1]), ppid: Number(m[2]), command: m[3] }))
  } catch {
    return []
  }
}

// The terminal is drawn on a WebGL canvas, so the page cannot show whether the shell started. A zsh process under the
// app does: node-pty starts it through spawn-helper, which fails when the helper is not executable.
async function waitForShellUnder(rootPid) {
  const deadline = Date.now() + 15_000
  while (Date.now() < deadline) {
    const table = processTable()
    const tree = new Set([rootPid])
    let grew = true
    while (grew) {
      grew = false
      for (const p of table) {
        if (!tree.has(p.pid) && tree.has(p.ppid)) {
          tree.add(p.pid)
          grew = true
        }
      }
    }
    const shell = table.find((p) => tree.has(p.pid) && /^-?(\S*\/)?zsh(\s|$)/.test(p.command))
    if (shell) return shell
    await new Promise((resolve) => setTimeout(resolve, 500))
  }
  return null
}

function hasZsh() {
  try {
    execFileSync('sh', ['-c', 'command -v zsh'], { stdio: 'ignore' })
    return true
  } catch {
    return false
  }
}

// lgrassd and the keeper are started detached, so they outlive the app. Only processes whose command line holds the
// temp HOME are touched.
function killLeftovers(home) {
  for (const p of processTable()) {
    if (p.pid === process.pid || !p.command.includes(home)) continue
    try {
      process.kill(p.pid, 'SIGTERM')
      console.log(`smoke: stopped leftover ${p.pid} ${p.command}`)
    } catch {
      // already gone
    }
  }
}

async function main() {
  const [target, outArg] = process.argv.slice(2)
  if (!target) {
    console.error('usage: node smoke/packaged.mjs <app> [out dir]')
    process.exit(2)
  }
  const outDir = path.resolve(outArg ?? path.join(import.meta.dirname, 'out'))
  fs.mkdirSync(outDir, { recursive: true })
  const mainLog = fs.createWriteStream(path.join(outDir, 'main.log'))
  const pageLog = fs.createWriteStream(path.join(outDir, 'renderer.log'))
  const home = seedHome()
  const pageErrors = []
  let app = null
  let page = null
  let failed = false

  try {
    app = await electron.launch({
      executablePath: executableFor(target),
      // A linux-unpacked chrome-sandbox is not setuid root, so Electron refuses to start with the sandbox on.
      args: process.platform === 'linux' ? ['--no-sandbox'] : [],
      env: appEnv(home),
      timeout: STEP_TIMEOUT
    })
    app.process().stdout?.pipe(mainLog)
    app.process().stderr?.pipe(mainLog)

    page = await app.firstWindow({ timeout: STEP_TIMEOUT })
    page.on('console', (msg) => pageLog.write(`[${msg.type()}] ${msg.text()}\n`))
    page.on('pageerror', (err) => {
      pageErrors.push(err.message)
      pageLog.write(`[pageerror] ${err.stack ?? err.message}\n`)
    })

    await page.locator('.header-bar').waitFor({ timeout: STEP_TIMEOUT })
    console.log('smoke: header shown')

    await page.getByRole('button', { name: '+ New shell' }).click({ timeout: STEP_TIMEOUT })
    await page
      .locator('.agent-option')
      .filter({ has: page.locator('.agent-label', { hasText: /^Shell$/ }) })
      .click({ timeout: STEP_TIMEOUT })
    await page.getByRole('button', { name: 'Open', exact: true }).click({ timeout: STEP_TIMEOUT })
    await page.locator('.terminal-pane .xterm').waitFor({ timeout: STEP_TIMEOUT })
    // Gives the shell time to print its prompt before the screenshot, and a failed spawn time to show.
    await page.waitForTimeout(3_000)
    if ((await page.locator('.restore-failure').count()) > 0)
      throw new Error('the Shell tab shows a start failure')
    console.log('smoke: terminal tab open')
    // The Shell tab runs zsh, which macOS always has. Elsewhere a missing zsh only skips this check.
    if (hasZsh()) {
      const shell = await waitForShellUnder(app.process().pid)
      if (!shell) throw new Error('no zsh process started under the app')
      console.log(`smoke: shell running, pid ${shell.pid}`)
    } else {
      console.log('smoke: zsh not installed here, shell process check skipped')
    }

    if (pageErrors.length > 0) throw new Error(`renderer errors: ${pageErrors.join(' | ')}`)
  } catch (err) {
    failed = true
    console.error(`smoke: FAILED: ${err.message}`)
  } finally {
    if (page) {
      const shot = path.join(outDir, failed ? 'smoke-failed.png' : 'smoke.png')
      await page.screenshot({ path: shot }).catch(() => {})
      console.log(`smoke: screenshot ${shot}`)
    }
    if (app) await app.close().catch(() => {})
    killLeftovers(home)
    // The temp home is left in place for a look by hand, and its daemon logs go out with the screenshot.
    const logs = path.join(home, '.lemongrass', 'logs')
    if (fs.existsSync(logs)) fs.cpSync(logs, path.join(outDir, 'lgrassd-logs'), { recursive: true })
    console.log(`smoke: temp home ${home}`)
    mainLog.end()
    pageLog.end()
  }
  process.exit(failed ? 1 : 0)
}

await main()
