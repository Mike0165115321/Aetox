// The AI's cross-project identity files (config.IdentityDir at DataRoot) —
// every *.md file here is folded into every system prompt regardless of
// which project is open (internal/prompt's "Personal instructions" layer).
// Multiple files (context.md, skills.md, ...), not one blob — independent of
// any single project's state.

import { ListIdentityFiles, ReadIdentityFile, SaveIdentityFile, DeleteIdentityFile } from '../../wailsjs/go/main/App'
import { t } from './i18n.svelte'

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

export async function createIdentityFile(name: string, content = ''): Promise<void> {
  const trimmed = name.trim()
  if (!trimmed) return
  const finalName = trimmed.toLowerCase().endsWith('.md') ? trimmed : trimmed + '.md'
  await SaveIdentityFile(finalName, content)
  await loadIdentityFiles()
  await openIdentityFile(finalName)
}

// Suggested starting files (ARCHITECTURE.md §11, 2026-07-24) — convention
// only, the engine treats every *.md in the identity dir identically.
// thinking.md is deliberately "discipline, not steps": step-by-step
// instructions can interfere with native-reasoning models, values don't.
//
// A function and not a const, because the bodies live in the locale files now
// and `t` has to be read at call time. They were Thai literals here, which made
// this the one place a user who picked another language still got Thai — and it
// is the least forgivable place for it: this is the file where they write down
// who they are, and it opened in a language they did not choose. The filenames
// stay untranslated on purpose; they are addresses the engine reads.
export function identityTemplates(): { name: string; content: string }[] {
  return [
    { name: 'identity.md', content: t('identity.tplIdentity') },
    { name: 'thinking.md', content: t('identity.tplThinking') },
    { name: 'context.md', content: t('identity.tplContext') },
    { name: 'skills.md', content: t('identity.tplSkills') },
  ]
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
