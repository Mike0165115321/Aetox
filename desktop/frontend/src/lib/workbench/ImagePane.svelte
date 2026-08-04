<script lang="ts">
  // A picture on the desk, shown as a picture. Unlike the spreadsheet, an image
  // needs no rendering engine to be honest about — the webview already is one —
  // so the open-externally card would be pure ceremony here.
  //
  // The button stays anyway, in the header rather than in place of the picture:
  // opening a file on the desk is the default now (a produced-file card in chat
  // lands here), so every pane owes the user the way out to the real app.
  //
  // Clicking the image toggles fit-to-pane and 1:1. The pane is a 384px-wide
  // column by default, so a screenshot arrives readable but small; 1:1 is how
  // you read the text in it without leaving the window.
  import { OpenFileExternally } from '../../../wailsjs/go/main/App'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'

  let { src, name, path }: { src: string; name: string; path: string } = $props()

  let actual = $state(false)
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

<div class="img-pane">
  <div class="img-head">
    <span class="img-name" title={name}>{name}</span>
    <button type="button" class="ctrl" onclick={openExternally}>
      <Icon name="folderOpen" size={13} /> {t('workbench.openExternally')}
    </button>
  </div>
  {#if failure}
    <div class="img-note">{failure}</div>
  {/if}
  <div class="img-scroll" class:actual>
    <button
      type="button" class="img-wrap" onclick={() => (actual = !actual)}
      title={actual ? t('imagePane.fitToPane') : t('imagePane.actualSize')}
    >
      <img {src} alt={name} />
    </button>
  </div>
</div>

<style>
  .img-pane { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .img-head { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-bottom: 1px solid var(--border-default); flex: none; }
  .img-name { font-size: var(--fs-sm); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  /* `.ctrl` carries no chrome of its own — every rule for it in style.css is
     scoped to a parent this pane is not. Same shape as .sheet-head .ctrl. */
  .img-head .ctrl {
    margin-left: auto; flex: none; appearance: none;
    background: var(--surface-sunken); border: 1px solid var(--border-strong);
    border-radius: var(--r-sm); color: var(--text-secondary);
    font: inherit; font-size: var(--fs-sm); padding: 4px 10px; cursor: pointer;
  }
  .img-head .ctrl:hover { border-color: var(--interactive); color: var(--text-primary); }
  .img-note { flex: none; padding: 6px 10px; font-size: var(--fs-xs); color: var(--status-danger); }

  .img-scroll {
    flex: 1; min-height: 0; overflow: auto;
    display: flex; align-items: center; justify-content: center; padding: 12px;
    /* The checkerboard is the only way a transparent PNG reads as transparent
       rather than as a picture with a dark background painted into it. */
    background-image:
      linear-gradient(45deg, var(--surface-raised) 25%, transparent 25%),
      linear-gradient(-45deg, var(--surface-raised) 25%, transparent 25%),
      linear-gradient(45deg, transparent 75%, var(--surface-raised) 75%),
      linear-gradient(-45deg, transparent 75%, var(--surface-raised) 75%);
    background-size: 16px 16px;
    background-position: 0 0, 0 8px, 8px -8px, -8px 0;
  }
  .img-wrap { display: block; padding: 0; background: none; border: 0; cursor: zoom-in; max-width: 100%; }
  .img-scroll.actual { align-items: flex-start; justify-content: flex-start; }
  .img-scroll.actual .img-wrap { cursor: zoom-out; max-width: none; }
  img { display: block; max-width: 100%; height: auto; border-radius: var(--r-sm); }
  .img-scroll.actual img { max-width: none; }
</style>
