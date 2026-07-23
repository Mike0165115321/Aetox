// The AI's cross-project identity files (config.IdentityDir at DataRoot) —
// every *.md file here is folded into every system prompt regardless of
// which project is open (internal/prompt's "Personal instructions" layer).
// Multiple files (context.md, skills.md, ...), not one blob — independent of
// any single project's state.

import { ListIdentityFiles, ReadIdentityFile, SaveIdentityFile, DeleteIdentityFile } from '../../wailsjs/go/main/App'

export const identity = $state<{
  files: { name: string }[]
  activeName: string
  draft: string
  saved: string
  loaded: boolean
  saving: boolean
}>({ files: [], activeName: '', draft: '', saved: '', loaded: false, saving: false })

export async function loadIdentityFiles(): Promise<void> {
  identity.files = await ListIdentityFiles()
  identity.loaded = true
  if (!identity.activeName && identity.files.length > 0) {
    await openIdentityFile(identity.files[0].name)
  }
}

export async function openIdentityFile(name: string): Promise<void> {
  identity.activeName = name
  const text = await ReadIdentityFile(name)
  identity.draft = text
  identity.saved = text
}

export async function saveIdentityFile(): Promise<void> {
  if (!identity.activeName) return
  identity.saving = true
  try {
    await SaveIdentityFile(identity.activeName, identity.draft)
    identity.saved = identity.draft
  } finally {
    identity.saving = false
  }
}

export async function createIdentityFile(name: string): Promise<void> {
  const trimmed = name.trim()
  if (!trimmed) return
  const finalName = trimmed.toLowerCase().endsWith('.md') ? trimmed : trimmed + '.md'
  await SaveIdentityFile(finalName, '')
  await loadIdentityFiles()
  await openIdentityFile(finalName)
}

export async function deleteIdentityFile(name: string): Promise<void> {
  await DeleteIdentityFile(name)
  if (identity.activeName === name) {
    identity.activeName = ''
    identity.draft = ''
    identity.saved = ''
  }
  await loadIdentityFiles()
}
