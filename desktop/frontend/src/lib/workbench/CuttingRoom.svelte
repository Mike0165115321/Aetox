<script lang="ts">
  // ห้องตัด — this session's cut, as a ledger with a player.
  //
  // The file tabs answer "what is this file"; this room answers the question a
  // cutting session grows into by its second render: "what has this cut
  // produced, and which of these five files is which". Same split that makes
  // DeckRoom a room, and the same rule inherited from it: the reader is not
  // rewritten here. The player is MediaPane whole and the picture pane is
  // ImagePane whole — both already carry the origin line and the plan bar —
  // because a second player is two places answering how a clip looks, and the
  // day they disagree nobody can say which is right.
  //
  // Everything drawn here is the editor's own JSON, relayed through
  // MediaOrigin (desktop/video_desk.go). No agent prose reaches this room.
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'
  import { cutroom, mediaLedger, fileView } from '../stores/workbench.svelte'
  import { fileURL } from '../fileUrl'
  import MediaPane from './MediaPane.svelte'
  import ImagePane from './ImagePane.svelte'

  const ledger = $derived(mediaLedger())
  // Sources first, in arrival order, then results in arrival order: the
  // material above the work, the way a bin sits above a timeline.
  const sources = $derived(ledger.filter((row) => row.role === 'source'))
  const results = $derived(ledger.filter((row) => row.role === 'result'))

  // The row on the player: the explicit pick, or the newest thing there is.
  // The fallback matters after a restart — the ledger is in-memory, so a
  // restored room starts empty and fills as work resumes.
  const shown = $derived(ledger.find((row) => row.path === cutroom.pick) ?? ledger.at(-1))
  const view = $derived(shown ? fileView(shown.path) : undefined)

  const newest = $derived(results.at(-1)?.path)

  /** The tool's own word for the row's act — kinocut's spelling, never a
   *  translation of it (MediaOriginLine says why at length). */
  function op(row: { operation?: string; tool?: string }): string {
    return row.operation || (row.tool ?? '').replace(/^kinocut_/, '')
  }

  function secs(n?: number): string {
    return n ? t('mediaOrigin.seconds', { n: String(Math.round(n * 10) / 10) }) : ''
  }
</script>

<div class="cutroom">
  {#if ledger.length === 0}
    <div class="empty">
      <span class="ic"><Icon name="clapperboard" size={26} /></span>
      <p>{t('cutroom.empty')}</p>
    </div>
  {:else}
    <div class="ledger">
      <div class="head">{t('cutroom.ledger')}</div>
      {#each sources as row (row.path)}
        <button type="button" class="row" class:on={shown?.path === row.path} onclick={() => (cutroom.pick = row.path)} title={row.path}>
          <span class="ic"><Icon name="clapperboard" size={14} /></span>
          <span class="body">
            <span class="name">{row.name}</span>
            <span class="sub">{t('cutroom.source')}</span>
          </span>
        </button>
      {/each}
      {#if sources.length > 0 && results.length > 0}<div class="sep"></div>{/if}
      {#each results as row (row.path)}
        <button type="button" class="row" class:on={shown?.path === row.path} onclick={() => (cutroom.pick = row.path)} title={row.path}>
          <span class="ic"><Icon name={fileView(row.path) === 'image' ? 'image' : 'clapperboard'} size={14} /></span>
          <span class="body">
            <span class="name">{row.name}</span>
            <span class="sub">
              {#if op(row)}<code>{op(row)}</code>{/if}
              {#if row.duration}{secs(row.duration)}{/if}
              {#if row.path === newest}· {t('cutroom.latest')}{/if}
            </span>
          </span>
        </button>
      {/each}
    </div>

    <div class="stage">
      {#if shown && view === 'image'}
        <!-- Keyed on the path so a switch remounts the pane: both panes copy
             their src once, the same reason Workbench keys file tabs on rev. -->
        {#key shown.path}
          <ImagePane src={fileURL(shown.path)} name={shown.name} path={shown.path} />
        {/key}
      {:else if shown && (view === 'video' || view === 'audio')}
        {#key shown.path}
          <MediaPane path={shown.path} name={shown.name} kind={view} />
        {/key}
      {/if}
    </div>
  {/if}
</div>

<style>
  .cutroom { display: flex; height: 100%; min-height: 0; }

  .empty {
    flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center;
    gap: 8px; color: var(--text-secondary); font-size: var(--fs-sm); text-align: center; padding: 16px;
  }
  .empty .ic { opacity: 0.5; }
  .empty p { max-width: 30ch; }

  .ledger {
    flex: none; width: 190px; min-width: 0; overflow-y: auto;
    border-right: 1px solid var(--border-default); padding: 6px 0;
  }
  .head { padding: 4px 12px 8px; font-size: var(--fs-xs); color: var(--text-dim); }
  .sep { height: 1px; background: var(--border-default); margin: 6px 12px; }

  .row {
    display: flex; align-items: flex-start; gap: 8px; width: 100%;
    appearance: none; background: none; border: 0; border-left: 2px solid transparent;
    padding: 7px 12px 7px 10px; cursor: pointer; text-align: left;
    font: inherit; color: var(--text-secondary);
  }
  .row:hover { background: var(--surface-sunken); }
  .row.on { background: var(--surface-sunken); border-left-color: var(--interactive); color: var(--text-primary); }
  .row .ic { flex: none; margin-top: 1px; opacity: 0.7; }
  .row .body { min-width: 0; display: flex; flex-direction: column; gap: 1px; }
  .row .name { font-size: var(--fs-sm); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .row .sub { font-size: var(--fs-xs); color: var(--text-dim); display: flex; gap: 5px; align-items: baseline; white-space: nowrap; overflow: hidden; }
  .row .sub code {
    font-family: var(--mono, monospace); font-size: 10px;
    background: var(--surface-sunken); border: 1px solid var(--border-default);
    border-radius: var(--r-sm); padding: 0 4px; color: var(--text-muted);
  }
  .row.on .sub { color: var(--text-muted); }

  .stage { flex: 1; min-width: 0; min-height: 0; }
</style>
