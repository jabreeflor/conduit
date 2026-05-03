#!/usr/bin/env node
// Broken-link checker for the docs site (issue #137 acceptance criteria).
//
// Pure Node, zero deps so CI doesn't need an `npm install` just to lint.
// Walks website/docs/ for *.md, extracts every relative markdown link, and
// resolves it against the on-disk file tree using VitePress conventions:
//   * `foo.md`  -> ./foo.md
//   * `foo/`    -> ./foo/index.md
//   * `foo`     -> ./foo.md OR ./foo/index.md
//   * `#anchor` -> same file (anchor not validated; cheap & safe)
//
// External links (http/https/mailto), absolute repo links, and image refs
// are skipped — VitePress' build will already catch malformed externals if
// you opt in. We catch the in-repo class of bug, which is what reviewers miss.

import { readFile, readdir, stat } from 'node:fs/promises'
import { join, dirname, resolve, relative, extname } from 'node:path'
import { existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const docsRoot = resolve(__dirname, '..', 'docs')

async function walk(dir) {
  const entries = await readdir(dir, { withFileTypes: true })
  const files = []
  for (const entry of entries) {
    if (entry.name.startsWith('.')) continue
    const full = join(dir, entry.name)
    if (entry.isDirectory()) files.push(...(await walk(full)))
    else if (entry.name.endsWith('.md')) files.push(full)
  }
  return files
}

const linkRegex = /\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g

async function checkFile(file) {
  const src = await readFile(file, 'utf8')
  const errors = []
  for (const match of src.matchAll(linkRegex)) {
    const raw = match[1]
    if (!raw) continue
    if (/^(https?:|mailto:|#)/.test(raw)) continue
    // strip query / hash
    const clean = raw.split('#')[0].split('?')[0]
    if (!clean) continue

    const baseDir = dirname(file)
    const candidates = []
    const target = resolve(baseDir, clean)

    if (extname(target)) {
      candidates.push(target)
    } else if (clean.endsWith('/')) {
      candidates.push(join(target, 'index.md'))
    } else {
      candidates.push(`${target}.md`)
      candidates.push(join(target, 'index.md'))
    }

    if (!candidates.some((c) => existsSync(c))) {
      errors.push(`  ${raw}  (resolved: ${candidates.map((c) => relative(docsRoot, c)).join(' | ')})`)
    }
  }
  return errors
}

const files = await walk(docsRoot)
let total = 0
let broken = 0
for (const file of files) {
  const errors = await checkFile(file)
  total += 1
  if (errors.length) {
    broken += errors.length
    console.error(`\n${relative(docsRoot, file)}:`)
    for (const e of errors) console.error(e)
  }
}

if (broken > 0) {
  console.error(`\n${broken} broken link(s) across ${files.length} file(s).`)
  process.exit(1)
} else {
  console.log(`OK — ${total} markdown file(s), no broken in-repo links.`)
}
