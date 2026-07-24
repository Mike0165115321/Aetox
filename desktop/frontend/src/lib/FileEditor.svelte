<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import type * as Monaco from 'monaco-editor'
  import { WriteFile } from '../../wailsjs/go/main/App'
  import { t } from './i18n.svelte'
  import { editorFont } from './editorFont.svelte'
  import { editorTheme, ensureEditorThemesRegistered } from './editorTheme.svelte'
  import { theme } from './theme.svelte'
  import { detectLanguage } from './monacoSetup'
  import { renderMarkdown } from './markdown'
  import { openUrlInWorkbench } from './stores/workbench.svelte'

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

  const dirty = $derived(draft !== base)

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
    if (!dirty || saving) return
    saving = true
    errorMsg = ''
    try {
      await WriteFile(path, draft)
      base = draft
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
      })
      // eslint-disable-next-line no-bitwise
      editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, save)
    })

    return () => { disposed = true }
  })

  onDestroy(() => {
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
    {#if dirty}<span class="fe-dirty">●</span>{/if}
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
  <div class="editor-mount" class:fe-hidden={isMarkdown && preview} bind:this={container}></div>
  {#if isMarkdown && preview}
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="fe-preview markdown-body" onclick={onPreviewClick}>{@html renderMarkdown(draft)}</div>
  {/if}
</div>
