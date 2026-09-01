<script lang="ts">
  // A clip on the desk, played on the desk.
  //
  // The file is never loaded — `src` is a URL into the file host, so the
  // webview fetches only the part being watched and scrubbing to the middle of
  // an hour-long recording costs nothing. This is the pane that could not exist
  // while every file had to cross a binding as a string.
  //
  // Same header as ImagePane deliberately: a file tab should feel like one room
  // whatever is inside it, and the way out to the real app belongs in the same
  // corner every time.
  import { OpenFileExternally } from '../../../wailsjs/go/main/App'
  import { fileURL } from '../fileUrl'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'
  import MediaOriginLine from './MediaOriginLine.svelte'

  let { path, name, kind }: { path: string; name: string; kind: 'video' | 'audio' } = $props()

  const src = $derived(fileURL(path))

  let failure = $state('')
  // A container this webview can open is not the same as a codec it can decode:
  // .mkv and .avi routinely hold H.265 or DivX, which Chromium refuses. The
  // element fires `error` and nothing appears, so the pane has to say why —
  // silence here reads as the app being broken rather than the codec being
  // unlicensed.
  let unplayable = $state(false)

  async function openExternally() {
    failure = ''
    try {
      await OpenFileExternally(path)
    } catch (err) {
      failure = t('workbench.openFileError', { err: String(err) })
    }
  }
</script>

<div class="media-pane">
  <div class="media-head">
    <span class="media-name" title={name}>{name}</span>
    <button type="button" class="ctrl" onclick={openExternally}>
      <Icon name="folderOpen" size={13} /> {t('workbench.openExternally')}
    </button>
  </div>
  {#if failure}
    <div class="media-note err">{failure}</div>
  {/if}

  <div class="media-body" class:audio={kind === 'audio'}>
    {#if unplayable}
      <div class="media-fallback">
        <span class="ic"><Icon name={kind === 'video' ? 'clapperboard' : 'headphones'} size={28} /></span>
        <p>{t('mediaPane.unplayable')}</p>
      </div>
    {:else if kind === 'video'}
      <!-- svelte-ignore a11y_media_has_caption -->
      <video {src} controls preload="metadata" onerror={() => (unplayable = true)}></video>
    {:else}
      <div class="audio-card">
        <span class="ic"><Icon name="headphones" size={22} /></span>
        <audio {src} controls preload="metadata" onerror={() => (unplayable = true)}></audio>
      </div>
    {/if}
  </div>

  <!-- Under the player rather than in the header: it describes what is in the
       frame, and it is only there at all when a tool put this file here. -->
  <MediaOriginLine {path} />
</div>

<style>
  .media-pane { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .media-head { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-bottom: 1px solid var(--border-default); flex: none; }
  .media-name { font-size: var(--fs-sm); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .media-head .ctrl {
    margin-left: auto; flex: none; appearance: none;
    background: var(--surface-sunken); border: 1px solid var(--border-strong);
    border-radius: var(--r-sm); color: var(--text-secondary);
    font: inherit; font-size: var(--fs-sm); padding: 4px 10px; cursor: pointer;
  }
  .media-head .ctrl:hover { border-color: var(--interactive); color: var(--text-primary); }
  .media-note { flex: none; padding: 6px 10px; font-size: var(--fs-xs); }
  .media-note.err { color: var(--status-danger); }

  .media-body {
    flex: 1; min-height: 0; display: flex; align-items: center; justify-content: center;
    padding: 12px; background: #000; /* letterboxing belongs to the video, not the theme */
  }
  .media-body.audio { background: var(--surface-sunken); }

  video { max-width: 100%; max-height: 100%; border-radius: var(--r-sm); }

  .audio-card {
    display: flex; flex-direction: column; align-items: center; gap: 12px;
    width: 100%; max-width: 420px; color: var(--text-muted);
  }
  .audio-card audio { width: 100%; }

  .media-fallback {
    display: flex; flex-direction: column; align-items: center; gap: 8px;
    text-align: center; color: var(--text-secondary); font-size: var(--fs-sm); max-width: 34ch;
  }
  .media-fallback .ic { opacity: 0.5; }
</style>
