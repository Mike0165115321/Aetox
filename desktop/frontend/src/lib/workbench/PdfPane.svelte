<script lang="ts">
  // A PDF on the desk, read on the desk.
  //
  // Nothing here renders the document — the webview already ships a PDF reader
  // (the same one Edge shows), and pointing an iframe at the file host is the
  // whole of using it. Scrolling, zoom, find-in-document and print come with
  // it. Bundling pdf.js to redo that would add a megabyte to the app to arrive
  // somewhere worse.
  //
  // The escape hatch is not a fallback here, it is the header: whether the
  // reader appears is the one thing this pane cannot check for itself. An
  // <iframe> reports `load` either way — a viewer that declined to render looks
  // exactly like one that did. So the way out to the real reader is on screen
  // from the first frame, and a blank document area is never a dead end.
  import { OpenFileExternally } from '../../../wailsjs/go/main/App'
  import { fileURL } from '../fileUrl'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'

  let { path, name }: { path: string; name: string } = $props()

  const src = $derived(fileURL(path))

  let failure = $state('')

  async function openExternally() {
    failure = ''
    try {
      await OpenFileExternally(path)
    } catch (err) {
      failure = t('workbench.openFileError', { err: String(err) })
    }
  }
</script>

<div class="pdf-pane">
  <div class="pdf-head">
    <span class="pdf-name" title={name}>{name}</span>
    <button type="button" class="ctrl" onclick={openExternally}>
      <Icon name="folderOpen" size={13} /> {t('workbench.openExternally')}
    </button>
  </div>
  {#if failure}
    <div class="pdf-note err">{failure}</div>
  {/if}
  <div class="pdf-body">
    <iframe {src} title={name}></iframe>
  </div>
</div>

<style>
  .pdf-pane { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .pdf-head { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-bottom: 1px solid var(--border-default); flex: none; }
  .pdf-name { font-size: var(--fs-sm); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .pdf-head .ctrl {
    margin-left: auto; flex: none; appearance: none;
    background: var(--surface-sunken); border: 1px solid var(--border-strong);
    border-radius: var(--r-sm); color: var(--text-secondary);
    font: inherit; font-size: var(--fs-sm); padding: 4px 10px; cursor: pointer;
  }
  .pdf-head .ctrl:hover { border-color: var(--interactive); color: var(--text-primary); }
  .pdf-note { flex: none; padding: 6px 10px; font-size: var(--fs-xs); }
  .pdf-note.err { color: var(--status-danger); }

  .pdf-body { flex: 1; min-height: 0; background: var(--surface-sunken); }
  iframe { width: 100%; height: 100%; border: 0; display: block; }
</style>
