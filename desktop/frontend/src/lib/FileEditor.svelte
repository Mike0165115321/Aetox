<script lang="ts">
  import { WriteFile } from '../../wailsjs/go/main/App'

  let { path, content }: { path: string; content: string } = $props()

  // Editor owns its draft from the initial content; tabs are keyed per file so
  // a new file mounts a fresh editor.
  // svelte-ignore state_referenced_locally
  let draft = $state(content)
  // svelte-ignore state_referenced_locally
  let base = $state(content) // last saved text
  let saving = $state(false)
  let errorMsg = $state('')

  const dirty = $derived(draft !== base)

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

  function onKeydown(e: KeyboardEvent) {
    if (e.ctrlKey && e.key.toLowerCase() === 's') {
      e.preventDefault()
      save()
    }
  }
</script>

<div class="file-editor">
  <div class="fe-head">
    <span class="fe-path">{path}</span>
    {#if dirty}<span class="fe-dirty">●</span>{/if}
    <span class="spacer"></span>
    {#if errorMsg}<span class="fe-error">{errorMsg}</span>{/if}
    <button class="ctrl" disabled={!dirty || saving} onclick={save}>
      {saving ? 'กำลังบันทึก…' : dirty ? 'บันทึก (Ctrl+S)' : 'บันทึกแล้ว'}
    </button>
  </div>
  <textarea class="editor-ta" bind:value={draft} onkeydown={onKeydown} spellcheck="false"></textarea>
</div>
