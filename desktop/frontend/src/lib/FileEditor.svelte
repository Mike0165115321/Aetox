<script lang="ts">
  import { onMount, onDestroy, untrack } from 'svelte'
  import type * as Monaco from 'monaco-editor'
  import { WriteFile } from '../../wailsjs/go/main/App'
  import { t } from './i18n.svelte'
  import { editorFont } from './editorFont.svelte'
  import { editorTheme, ensureEditorThemesRegistered } from './editorTheme.svelte'
  import { theme } from './theme.svelte'
  import { detectLanguage } from './monacoSetup'
  import { renderMarkdown } from './markdown'
  import { openUrlInWorkbench } from './stores/workbench.svelte'
  import Icon from './Icon.svelte'

  // 'auto' follows the app's named UI theme; any other choice is a manual override.
  const monacoTheme = $derived(editorTheme.choice === 'auto' ? theme.name : editorTheme.choice)

  let { path, content }: { path: string; content: string } = $props()

  // Editor owns its draft from the initial content; tabs are keyed per file so
  // a new file mounts a fresh editor (see App.svelte's keyed each on f.path).
  // svelte-ignore state_referenced_locally
  let draft = $state(content)
  // svelte-ignore state_referenced_locally
  let base = $state(content) // last saved text
  let saving = $state(false)
  let errorMsg = $state('')
  // Bytes that arrived from outside while there was an unsaved draft in here.
  // Null the rest of the time, which with autosave on is nearly always.
  let incoming = $state<string | null>(null)

  const dirty = $derived(draft !== base)

  // How long after the last keystroke the file is written.
  //
  // Owner, 24 ส.ค.: *"เวลาเอเจนเปิดไฟล์หรือแก้ไขไฟล์ผมอยากให้มัน ออโต้เซฟ"*.
  // Short enough that the agent reading the file a moment later reads what is
  // on screen — which is the whole point when a turn is running beside you —
  // and long enough that a sentence being typed is one write, not thirty.
  const AUTOSAVE_MS = 700
  let saveTimer: ReturnType<typeof setTimeout> | undefined
  // True only while an outside change is being written into the model. The
  // change handler fires during that, and without this the file would schedule
  // a save of bytes it had just been handed — writing the agent's own work back
  // at it as if the user had typed it.
  let applyingExternal = false

  function scheduleSave() {
    clearTimeout(saveTimer)
    // A conflict is the one state autosave must not resolve on its own: the
    // draft here and the file on disk have both moved, and quietly writing this
    // one over that one is a decision with no way back. The bar asks instead.
    if (incoming !== null) return
    saveTimer = setTimeout(() => { void save() }, AUTOSAVE_MS)
  }

  /** Put bytes from outside into the editor without moving the user.
   *
   * pushEditOperations rather than setValue: it keeps the undo stack, so an
   * edit the agent made while the user was reading is still Ctrl+Z-able, and
   * the caret and scroll are put back where they were. A pane that jumps to
   * line 1 every time a turn saves is one nobody can work beside. */
  function applyExternal(next: string) {
    applyingExternal = true
    try {
      if (editor && model) {
        const pos = editor.getPosition()
        const top = editor.getScrollTop()
        model.pushEditOperations([], [{ range: model.getFullModelRange(), text: next }], () => null)
        if (pos) editor.setPosition(pos)
        editor.setScrollTop(top)
      }
      draft = next
      base = next
      incoming = null
    } finally {
      applyingExternal = false
    }
  }

  // The file changed on disk — the agent wrote it, and the store re-read it
  // (workbench.svelte.ts filesChangedOnDisk). `content` is a prop, so this is
  // the whole subscription.
  //
  // untrack around the rest: `base` and `draft` are read to decide what to do
  // and must not themselves re-trigger the decision, or applying a change would
  // immediately ask to apply it again.
  $effect(() => {
    const next = content
    untrack(() => {
      if (next === undefined || next === base) return
      if (draft !== base) {
        // Someone is typing in here. Hold the new bytes and stop the clock:
        // whichever side wins, it is the user who says so.
        clearTimeout(saveTimer)
        incoming = next
        return
      }
      applyExternal(next)
    })
  })

  // Markdown files open in a rendered view (same renderer as chat); one click
  // flips to the editor. The Monaco mount stays alive underneath (CSS-hidden)
  // so toggling never re-runs the editor lifecycle.
  const isMarkdown = /\.(md|markdown)$/i.test(path)
  // svelte-ignore state_referenced_locally
  let preview = $state(isMarkdown)

  // Links in the rendered view must not navigate the app's webview away —
  // open them in a workbench browser tab instead (same rule as chat).
  function onPreviewClick(e: MouseEvent) {
    const a = (e.target as HTMLElement).closest('a')
    const href = a?.getAttribute('href')
    if (!href || !/^https?:\/\//i.test(href)) return
    e.preventDefault()
    openUrlInWorkbench(href)
  }

  let container = $state<HTMLDivElement>()
  let editor: Monaco.editor.IStandaloneCodeEditor | undefined
  let model: Monaco.editor.ITextModel | undefined

  async function save() {
    clearTimeout(saveTimer)
    if (!dirty || saving) return
    saving = true
    errorMsg = ''
    try {
      await WriteFile(path, draft)
      base = draft
      // Saving IS the answer to a conflict: the user chose this text over the
      // one on disk, and the file now says so.
      incoming = null
    } catch (err) {
      errorMsg = String(err)
    } finally {
      saving = false
    }
  }

  onMount(() => {
    let disposed = false
    // Monaco is large (~5MB) — load it lazily so opening the app (or a tab
    // that never touches the editor) doesn't pay for it upfront.
    import('monaco-editor').then(async (monaco) => {
      await import('./monacoSetup') // registers MonacoEnvironment.getWorker before create()
      await ensureEditorThemesRegistered()
      if (disposed || !container) return

      model = monaco.editor.createModel(content, detectLanguage(path))
      editor = monaco.editor.create(container, {
        model,
        theme: monacoTheme,
        fontSize: editorFont.size,
        minimap: { enabled: true },
        automaticLayout: true,
        scrollBeyondLastLine: false,
      })
      editor.onDidChangeModelContent(() => {
        draft = model!.getValue()
        if (!applyingExternal) scheduleSave()
      })
      // eslint-disable-next-line no-bitwise
      editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, save)
    })

    return () => { disposed = true }
  })

  onDestroy(() => {
    // The last write, and the one autosave exists for: a tab closed (or a
    // session switched) inside the debounce window would otherwise drop
    // whatever was typed in the final second. Not awaited — nothing may block a
    // teardown — and skipped on a conflict, where the file on disk is not this
    // pane's to overwrite unasked.
    clearTimeout(saveTimer)
    if (draft !== base && incoming === null) void WriteFile(path, draft)
    editor?.dispose()
    model?.dispose()
  })

  $effect(() => {
    editor?.updateOptions({ fontSize: editorFont.size })
  })

  $effect(() => {
    import('monaco-editor').then((monaco) => monaco.editor.setTheme(monacoTheme))
  })
</script>

<div class="file-editor">
  <div class="fe-head">
    <span class="fe-path">{path}</span>
    {#if dirty}<span class="fe-dirty"><Icon name="dot" size={14} /></span>{/if}
    <span class="spacer"></span>
    {#if errorMsg}<span class="fe-error">{errorMsg}</span>{/if}
    {#if isMarkdown}
      <button class="ctrl" onclick={() => (preview = !preview)}>
        {preview ? t('fileEditor.source') : t('fileEditor.preview')}
      </button>
    {/if}
    <button class="ctrl" disabled={!dirty || saving} onclick={save}>
      {saving ? t('fileEditor.saving') : dirty ? t('fileEditor.save') : t('fileEditor.saved')}
    </button>
  </div>
  {#if incoming !== null}
    <!-- Both sides moved at once, which autosave makes rare and cannot make
         impossible. Neither is thrown away without being asked: the file on
         disk still holds the agent's version, this pane still holds what was
         typed, and the two buttons are the only way out. -->
    <div class="fe-conflict" role="status">
      <span class="fe-conflict-text">{t('fileEditor.changedOutside')}</span>
      <button class="ctrl" onclick={() => applyExternal(incoming ?? draft)}>{t('fileEditor.takeDisk')}</button>
      <button class="ctrl" onclick={save}>{t('fileEditor.keepMine')}</button>
    </div>
  {/if}
  <div class="editor-mount" class:fe-hidden={isMarkdown && preview} bind:this={container}></div>
  {#if isMarkdown && preview}
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="fe-preview" onclick={onPreviewClick}>
      <div class="fe-preview-inner markdown-body">{@html renderMarkdown(draft)}</div>
    </div>
  {/if}
</div>
