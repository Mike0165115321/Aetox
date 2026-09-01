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
  //
  // An .svg is the one picture here that is not fetched as one. It is drawn
  // into the app's own document instead — see inlineDrawing, and the fetch
  // below for why a photograph must not take the same route.
  import { OpenFileExternally } from '../../../wailsjs/go/main/App'
  import { inlineDrawing } from '../markdown'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'
  import MediaOriginLine from './MediaOriginLine.svelte'

  let { src, name, path }: { src: string; name: string; path: string } = $props()

  let actual = $state(false)
  let failure = $state('')

  const isDrawing = $derived(/\.svg$/i.test(path))

  // The markup, once the file has been read and made safe. Empty means "not
  // read yet or not a drawing"; `drawError` is the difference between the two,
  // because a pane that draws nothing and says nothing is the bug this whole
  // change is about.
  let drawing = $state('')
  let drawError = $state('')
  let host = $state<HTMLDivElement | null>(null)

  // Read as text over the file host, not through a binding: it is the same URL
  // the <img> would have used, so the sandbox rule that answers it is the same
  // one — this adds no reach, only a second way of reading what was already
  // being served (desktop/filehost.go).
  //
  // A photograph never comes this way. Its bytes are not markup, and streaming
  // them into a string is exactly the cost the file host exists to avoid.
  $effect(() => {
    const url = src
    const key = path
    if (!isDrawing) {
      drawing = ''
      drawError = ''
      return
    }
    let live = true
    drawError = ''
    void (async () => {
      try {
        const res = await fetch(url)
        if (!res.ok) throw new Error(String(res.status))
        const markup = inlineDrawing(await res.text(), key)
        if (!live) return
        drawing = markup
        drawError = markup === '' ? t('imagePane.notADrawing') : ''
      } catch (err) {
        if (!live) return
        drawing = ''
        drawError = t('imagePane.drawingError', { err: String(err) })
      }
    })()
    return () => { live = false }
  })

  // 1:1 for a vector is its viewBox, which is the only natural size it has —
  // the file itself says width="100%" (internal/prompt's drawing layer teaches
  // that, so that a drawing fills whatever it is given). Set on the element
  // rather than in CSS because the element arrives through {@html} and there is
  // no component here to give a prop to.
  $effect(() => {
    const svg = host?.querySelector('svg')
    if (!svg) return
    void drawing // re-run when the file is re-read, not only when `actual` flips
    const box = (svg as SVGSVGElement).viewBox?.baseVal
    svg.setAttribute('style', actual && box && box.width > 0 ? `width:${box.width}px` : '')
  })

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
  {#if drawError}
    <div class="img-note">{drawError}</div>
  {/if}
  <div class="img-scroll" class:actual>
    <button
      type="button" class="img-wrap" class:drawing={isDrawing} onclick={() => (actual = !actual)}
      title={actual ? t('imagePane.fitToPane') : t('imagePane.actualSize')}
    >
      {#if isDrawing}
        <div class="draw" bind:this={host}>{@html drawing}</div>
      {:else}
        <img {src} alt={name} />
      {/if}
    </button>
  </div>

  <!-- A storyboard grid is as much the result of a cut as the clip is, so the
       same strip sits under it — see MediaOriginLine. -->
  <MediaOriginLine {path} />
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
  /* A vector has no pixels to shrink-wrap. The wrapper is a flex item, so it
     takes its width from its content — and a drawing written the way this app
     asks for one (width="100%", size in viewBox units) has no content width to
     give back: it resolved to 0×0 and the pane showed an empty checkerboard.
     The width has to come from the pane, which is the one box that has one. */
  .img-wrap.drawing { width: 100%; }
  .img-scroll.actual .img-wrap.drawing { width: auto; }
  .draw :global(svg) { display: block; max-width: 100%; height: auto; margin: 0 auto; }
  .img-scroll.actual .draw :global(svg) { max-width: none; margin: 0; }
  .img-scroll.actual { align-items: flex-start; justify-content: flex-start; }
  .img-scroll.actual .img-wrap { cursor: zoom-out; max-width: none; }
  img { display: block; max-width: 100%; height: auto; border-radius: var(--r-sm); }
  .img-scroll.actual img { max-width: none; }
</style>
