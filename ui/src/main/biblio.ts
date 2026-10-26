// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { ipcMain, net, protocol, WebContents } from 'electron'
import { pathToFileURL } from 'url'
import {
  closeSync,
  existsSync,
  mkdirSync,
  openSync,
  readdirSync,
  readFileSync,
  readSync,
  realpathSync,
  renameSync,
  statSync,
  watch,
  writeFileSync,
  type FSWatcher
} from 'fs'
import { dirname, extname, join, posix, relative, resolve, sep } from 'path'

const IMAGES_DIR = 'images'
const IMAGE_SCHEME = 'biblio-image'
const MAX_IMAGE_BYTES = 10 * 1024 * 1024
const MAX_TEXT_BYTES = 2 * 1024 * 1024
const BINARY_SNIFF_BYTES = 8 * 1024
const IMAGE_EXTENSIONS: Record<string, string> = {
  'image/png': '.png',
  'image/jpeg': '.jpg',
  'image/gif': '.gif',
  'image/webp': '.webp'
}

export interface BiblioNode {
  name: string
  path: string
  type: 'dir' | 'file'
  // Set only under books/, for the toc viewer's written and updated dates.
  createdMs?: number
  modifiedMs?: number
  children?: BiblioNode[]
}

export type BiblioFileResult =
  | { kind: 'text'; text: string }
  | { kind: 'image'; modifiedMs: number }
  | { kind: 'binary' }
  | { kind: 'tooLarge' }

export interface BiblioTree {
  // books/toc.md, pulled out of the normal books/ listing so it can be
  // shown in its own dedicated spot rather than buried inside a category.
  toc: BiblioNode | null
  children: BiblioNode[]
}

// Guarantees a "scratchpad" entry always exists in the top-level listing,
// even before it's ever been created on disk -- otherwise the "add
// scratchpad" affordance would have no row to attach to for a biblio/ that
// hasn't grown one yet.
function ensureScratchpad(children: BiblioNode[]): BiblioNode[] {
  if (children.some((n) => n.type === 'dir' && n.name === 'scratchpad')) return children
  const scratchpad: BiblioNode = {
    name: 'scratchpad',
    path: 'scratchpad',
    type: 'dir',
    children: []
  }
  return [...children, scratchpad].sort((a, b) => a.name.localeCompare(b.name))
}

function extractToc(children: BiblioNode[]): BiblioNode | null {
  const books = children.find((n) => n.type === 'dir' && n.name === 'books')
  if (!books?.children) return null
  const index = books.children.findIndex((n) => n.type === 'file' && n.name === 'toc.md')
  if (index === -1) return null
  const [toc] = books.children.splice(index, 1)
  return toc
}

const ARCHIVE_DIR = 'archive'

function createdMs(path: string): number {
  const stats = statSync(path)
  return stats.birthtimeMs > 0 ? stats.birthtimeMs : stats.mtimeMs
}

function times(path: string): Pick<BiblioNode, 'createdMs' | 'modifiedMs'> {
  const stats = statSync(path)
  return {
    createdMs: stats.birthtimeMs > 0 ? stats.birthtimeMs : stats.mtimeMs,
    modifiedMs: stats.mtimeMs
  }
}

function walk(dir: string, relativeTo: string): BiblioNode[] {
  const parent = relative(relativeTo, dir)
  const inBooks = parent.split(sep)[0] === 'books'
  const newestFirst = parent === 'scratchpad' || parent === join('scratchpad', ARCHIVE_DIR)
  // A task folder holds whatever files the work needs; the other tiers are markdown-only by convention.
  const anyFile = parent.split(sep)[0] === 'scratchpad'
  const entries = readdirSync(dir, { withFileTypes: true })
    .filter((e) => (e.isDirectory() ? e.name !== IMAGES_DIR : anyFile || e.name.endsWith('.md')))
    .sort((a, b) => {
      if (a.isDirectory() !== b.isDirectory()) return a.isDirectory() ? -1 : 1
      if (a.isDirectory() && newestFirst) {
        if (a.name === ARCHIVE_DIR) return -1
        if (b.name === ARCHIVE_DIR) return 1
        return createdMs(join(dir, b.name)) - createdMs(join(dir, a.name))
      }
      return a.name.localeCompare(b.name)
    })

  return entries.map((entry) => {
    const fullPath = join(dir, entry.name)
    const relPath = relative(relativeTo, fullPath)
    const dated = inBooks ? times(fullPath) : {}
    if (entry.isDirectory()) {
      return {
        name: entry.name,
        path: relPath,
        type: 'dir',
        ...dated,
        children: walk(fullPath, relativeTo)
      }
    }
    return { name: entry.name, path: relPath, type: 'file', ...dated }
  })
}

// Resolves a biblio-relative path and checks it stays inside the biblio/
// root, so a crafted "../.." path from the renderer can't read outside it.
function resolveInBiblio(biblioRoot: string, relativePath: string): string | null {
  const resolved = resolve(biblioRoot, relativePath)
  if (resolved !== biblioRoot && !resolved.startsWith(biblioRoot + sep)) return null
  return resolved
}

// Must run before the app is ready.
export function registerBiblioImageScheme(): void {
  protocol.registerSchemesAsPrivileged([
    { scheme: IMAGE_SCHEME, privileges: { standard: true, secure: true, supportFetchAPI: true } }
  ])
}

function isImageFile(target: string): boolean {
  const extension = extname(target).toLowerCase()
  return extension === '.jpeg' || Object.values(IMAGE_EXTENSIONS).includes(extension)
}

function isScratchpadImage(root: string, target: string): boolean {
  return relative(root, target).split(sep)[0] === 'scratchpad' && isImageFile(target)
}

function handleImageRequest(request: Request): Response | Promise<Response> {
  try {
    const params = new URL(request.url).searchParams
    const projectPath = params.get('p')
    const relativePath = params.get('f')
    if (!projectPath || !relativePath) return new Response(null, { status: 400 })
    const root = realpathSync(join(projectPath, 'biblio'))
    const target = resolveInBiblio(root, relativePath)
    if (!target || !isScratchpadImage(root, target)) return new Response(null, { status: 403 })
    return net.fetch(pathToFileURL(realpathSync(target)).toString())
  } catch {
    return new Response(null, { status: 404 })
  }
}

function slugify(title: string): string {
  const slug = title
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return slug || 'untitled'
}

// A tool call on the destroyed sender's WebContents (window closed mid-debounce) throws.
function send(sender: WebContents | undefined, channel: string, payload: unknown): void {
  if (sender && !sender.isDestroyed()) sender.send(channel, payload)
}

const watchDebounce = 400
const watchers = new Map<string, FSWatcher>()
const rootWatchers = new Map<string, FSWatcher>()
const debounceTimers = new Map<string, ReturnType<typeof setTimeout>>()

// True for scratchpad/ and everything under it except scratchpad/archive, which only a model may change.
function isLiveScratchpadPath(relativePath: string): boolean {
  const parts = relativePath.split(sep)
  return parts[0] === 'scratchpad' && parts[1] !== 'archive'
}

// Starts a recursive watch on a project's biblio/ root the first time its
// tree is fetched, so a file added by anything other than this app's own
// IPC handlers (another lgrass pane, a direct filesystem write) still
// reaches the renderer instead of waiting for the next project switch.
function watchBiblio(
  biblioRoot: string,
  projectPath: string,
  getSender: () => WebContents | undefined
): void {
  if (watchers.has(projectPath)) return
  try {
    const watcher = watch(biblioRoot, { recursive: true }, () => {
      clearTimeout(debounceTimers.get(projectPath))
      debounceTimers.set(
        projectPath,
        setTimeout(() => send(getSender(), 'biblio:changed', projectPath), watchDebounce)
      )
    })
    watcher.on('error', () => {
      watcher.close()
      watchers.delete(projectPath)
    })
    watchers.set(projectPath, watcher)
  } catch {
    // Recursive fs.watch isn't available on every platform; the tree stays
    // on-demand-only for this project instead of failing the app.
  }
}

function isBiblioDir(biblioRoot: string): boolean {
  try {
    return statSync(biblioRoot).isDirectory()
  } catch {
    return false
  }
}

function unwatch(map: Map<string, FSWatcher>, projectPath: string): void {
  map.get(projectPath)?.close()
  map.delete(projectPath)
}

// Watches the project root for biblio/ to appear, because the recursive watch needs an existing directory.
function watchForBiblio(projectPath: string, getSender: () => WebContents | undefined): void {
  if (rootWatchers.has(projectPath)) return
  const biblioRoot = join(projectPath, 'biblio')
  const found = (): void => {
    unwatch(rootWatchers, projectPath)
    send(getSender(), 'biblio:changed', projectPath)
  }
  try {
    const watcher = watch(projectPath, (_event, filename) => {
      if (filename === 'biblio' && isBiblioDir(biblioRoot)) found()
    })
    watcher.on('error', () => unwatch(rootWatchers, projectPath))
    rootWatchers.set(projectPath, watcher)
  } catch {
    return
  }
  if (isBiblioDir(biblioRoot)) found()
}

export function closeBiblioWatchers(): void {
  for (const watcher of watchers.values()) watcher.close()
  watchers.clear()
  for (const watcher of rootWatchers.values()) watcher.close()
  rootWatchers.clear()
  for (const timer of debounceTimers.values()) clearTimeout(timer)
  debounceTimers.clear()
}

export function registerBiblioHandlers(getSender: () => WebContents | undefined): void {
  protocol.handle(IMAGE_SCHEME, handleImageRequest)

  ipcMain.handle('biblio:tree', (_event, projectPath: string): BiblioTree | null => {
    const biblioRoot = join(projectPath, 'biblio')
    if (!isBiblioDir(biblioRoot)) {
      unwatch(watchers, projectPath)
      watchForBiblio(projectPath, getSender)
      return null
    }
    unwatch(rootWatchers, projectPath)
    try {
      const root = realpathSync(biblioRoot)
      watchBiblio(root, projectPath, getSender)
      const children = ensureScratchpad(walk(root, root))
      return { toc: extractToc(children), children }
    } catch {
      return null
    }
  })

  ipcMain.handle(
    'biblio:read',
    (_event, projectPath: string, relativePath: string): string | null => {
      const biblioRoot = join(projectPath, 'biblio')
      try {
        const root = realpathSync(biblioRoot)
        const target = resolveInBiblio(root, relativePath)
        if (!target || !target.endsWith('.md')) return null
        return readFileSync(target, 'utf-8')
      } catch {
        return null
      }
    }
  )

  // Reads any scratchpad file for the read-only viewer. Text is told apart from binary by a NUL byte or invalid UTF-8 in the first 8 KB.
  ipcMain.handle(
    'biblio:readFile',
    (_event, projectPath: string, relativePath: string): BiblioFileResult | null => {
      try {
        const root = realpathSync(join(projectPath, 'biblio'))
        const target = resolveInBiblio(root, relativePath)
        if (!target || relative(root, target).split(sep)[0] !== 'scratchpad') return null
        const stats = statSync(target)
        if (!stats.isFile()) return null
        if (isImageFile(target)) return { kind: 'image', modifiedMs: stats.mtimeMs }
        const head = Buffer.alloc(Math.min(stats.size, BINARY_SNIFF_BYTES))
        const fd = openSync(target, 'r')
        try {
          readSync(fd, head, 0, head.length, 0)
        } finally {
          closeSync(fd)
        }
        if (head.includes(0)) return { kind: 'binary' }
        try {
          new TextDecoder('utf-8', { fatal: true }).decode(head, { stream: true })
        } catch {
          return { kind: 'binary' }
        }
        if (stats.size > MAX_TEXT_BYTES) return { kind: 'tooLarge' }
        return { kind: 'text', text: readFileSync(target, 'utf-8') }
      } catch {
        return null
      }
    }
  )

  // Saves an existing file under the live scratchpad tier only, so a stale editor can never recreate a moved or archived file.
  ipcMain.handle(
    'biblio:write',
    (_event, projectPath: string, relativePath: string, content: string): boolean => {
      if (!isLiveScratchpadPath(relativePath)) return false
      const biblioRoot = join(projectPath, 'biblio')
      try {
        const root = realpathSync(biblioRoot)
        const target = resolveInBiblio(root, relativePath)
        if (!target || !target.endsWith('.md') || !existsSync(target)) return false
        writeFileSync(target, content, 'utf-8')
        return true
      } catch {
        return false
      }
    }
  )

  ipcMain.handle(
    'biblio:createScratchpad',
    (_event, projectPath: string, title: string, content: string): string | null => {
      const biblioRoot = join(projectPath, 'biblio')
      try {
        mkdirSync(biblioRoot, { recursive: true })
        const root = realpathSync(biblioRoot)
        const scratchpadRoot = join(root, 'scratchpad')
        mkdirSync(scratchpadRoot, { recursive: true })

        const base = slugify(title)
        let slug = base
        let attempt = 2
        while (existsSync(join(scratchpadRoot, slug))) {
          slug = `${base}-${attempt}`
          attempt += 1
        }

        const taskDir = join(scratchpadRoot, slug)
        mkdirSync(taskDir, { recursive: true })
        writeFileSync(join(taskDir, 'notes.md'), content, 'utf-8')
        return `scratchpad${sep}${slug}${sep}notes.md`
      } catch {
        return null
      }
    }
  )

  // Moves a top-level scratchpad task folder into scratchpad/archive/, and
  // its matching handover file (if one was ever written for this task) into
  // handover/archive/ alongside it. A task with no handover just moves alone.
  ipcMain.handle(
    'biblio:archiveScratchpad',
    (_event, projectPath: string, relativePath: string): boolean => {
      const parts = relativePath.split(sep)
      if (parts.length !== 2 || parts[0] !== 'scratchpad' || parts[1] === 'archive') return false
      const slug = parts[1]

      const biblioRoot = join(projectPath, 'biblio')
      try {
        const root = realpathSync(biblioRoot)
        const source = resolveInBiblio(root, relativePath)
        if (!source || !statSync(source).isDirectory()) return false

        const scratchpadArchiveRoot = join(root, 'scratchpad', 'archive')
        mkdirSync(scratchpadArchiveRoot, { recursive: true })
        let destSlug = slug
        let attempt = 2
        while (existsSync(join(scratchpadArchiveRoot, destSlug))) {
          destSlug = `${slug}-${attempt}`
          attempt += 1
        }
        renameSync(source, join(scratchpadArchiveRoot, destSlug))

        const handoverFile = join(root, 'handover', `${slug}.md`)
        if (existsSync(handoverFile)) {
          const handoverArchiveRoot = join(root, 'handover', 'archive')
          mkdirSync(handoverArchiveRoot, { recursive: true })
          let handoverDestName = `${slug}.md`
          let handoverAttempt = 2
          while (existsSync(join(handoverArchiveRoot, handoverDestName))) {
            handoverDestName = `${slug}-${handoverAttempt}.md`
            handoverAttempt += 1
          }
          renameSync(handoverFile, join(handoverArchiveRoot, handoverDestName))
        }

        return true
      } catch {
        return false
      }
    }
  )

  // Writes a single file into an existing folder under scratchpad/.
  ipcMain.handle(
    'biblio:createScratchpadFile',
    (
      _event,
      projectPath: string,
      folderRelativePath: string,
      title: string,
      content: string
    ): string | null => {
      if (!isLiveScratchpadPath(folderRelativePath)) return null
      const biblioRoot = join(projectPath, 'biblio')
      try {
        const root = realpathSync(biblioRoot)
        const targetDir = resolveInBiblio(root, folderRelativePath)
        if (!targetDir || !statSync(targetDir).isDirectory()) return null

        const base = slugify(title)
        let filename = `${base}.md`
        let attempt = 2
        while (existsSync(join(targetDir, filename))) {
          filename = `${base}-${attempt}.md`
          attempt += 1
        }

        writeFileSync(join(targetDir, filename), content, 'utf-8')
        return `${folderRelativePath}${sep}${filename}`
      } catch {
        return null
      }
    }
  )

  // Stores a pasted or dropped image under the note's task folder and returns
  // its path relative to the note.
  ipcMain.handle(
    'biblio:saveScratchpadImage',
    (
      _event,
      projectPath: string,
      notePath: string,
      bytes: Uint8Array,
      mime: string
    ): { path: string } | { error: 'too-large' | 'unsupported' | 'failed' } => {
      const extension = IMAGE_EXTENSIONS[mime]
      if (!extension) return { error: 'unsupported' }
      if (bytes.byteLength > MAX_IMAGE_BYTES) return { error: 'too-large' }
      const noteParts = notePath.split(sep)
      if (noteParts[0] !== 'scratchpad' || noteParts.length < 3 || noteParts[1] === 'archive')
        return { error: 'failed' }
      try {
        const root = realpathSync(join(projectPath, 'biblio'))
        const note = resolveInBiblio(root, notePath)
        const imagesDir = resolveInBiblio(root, join('scratchpad', noteParts[1], IMAGES_DIR))
        if (!note || !imagesDir || !statSync(dirname(note)).isDirectory())
          return { error: 'failed' }
        mkdirSync(imagesDir, { recursive: true })
        const name = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}${extension}`
        writeFileSync(join(imagesDir, name), Buffer.from(bytes))
        return { path: relative(dirname(note), join(imagesDir, name)).split(sep).join(posix.sep) }
      } catch {
        return { error: 'failed' }
      }
    }
  )

  // Reads an image file the user pasted as a path.
  ipcMain.handle(
    'biblio:readImageFile',
    (
      _event,
      filePath: string
    ): { bytes: Uint8Array; mime: string } | { error: 'too-large' | 'unsupported' | 'failed' } => {
      const mime = Object.keys(IMAGE_EXTENSIONS).find(
        (m) =>
          IMAGE_EXTENSIONS[m] === extname(filePath).toLowerCase() ||
          (m === 'image/jpeg' && extname(filePath).toLowerCase() === '.jpeg')
      )
      if (!mime) return { error: 'unsupported' }
      try {
        const stats = statSync(filePath)
        if (!stats.isFile()) return { error: 'failed' }
        if (stats.size > MAX_IMAGE_BYTES) return { error: 'too-large' }
        return { bytes: readFileSync(filePath), mime }
      } catch {
        return { error: 'failed' }
      }
    }
  )
}
