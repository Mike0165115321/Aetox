<script lang="ts">
  // ผลงาน (COMPANY.md §2): every file Aetox has made, read live off the disk.
  //
  // There is no index table behind this and there deliberately never will be —
  // the folder is the half users move, rename and delete without telling us, so
  // an index would show files that are gone and hide files that are there.
  // Deleting a conversation leaves its work alone (§6.7); this page is the one
  // place a produced file is deleted, by the user, on purpose.
  import { onMount } from 'svelte'
  import { ListArtifactsIn, OpenArtifact, DeleteArtifact, ArtifactPreview } from '../../wailsjs/go/main/App'
  import { main } from '../../wailsjs/go/models'
  import { agoLabel, selectGlobalSession, setActiveView } from './stores/cockpit.svelte'
  import { dayBucket } from './dayBucket'
  import { t } from './i18n.svelte'
  import { renderMarkdown } from './markdown'
  import Icon from './Icon.svelte'
  import type { IconName } from './icons'

  let { onClose }: { onClose: () => void } = $props()

  let files = $state<main.Artifact[]>([])
  let loaded = $state(false)
  let error = $state('')

  // Which slice of time the page is showing (DECISIONS §106-era owner call,
  // 2026-08-14: "โหลดมาแสดงล่าสุดแค่ของสัปดาห์นี้เป็นอันเริ่มต้นดีไหม เราเก็บเวลา
  // อยู่ละ จะได้แยกไทม์ไลน์ได้ด้วย").
  //
  // `range` is what the user picked; `served` is what the engine answered with,
  // and they differ when the picked range turned out to be empty and the engine
  // widened. The picker follows `served`, because a control that says "week"
  // over a month of files is lying about what you are looking at.
  const RANGES = ['week', 'month', 'all'] as const
  type Range = (typeof RANGES)[number]
  let range = $state<Range>('week')
  let served = $state<Range>('week')
  let total = $state(0)

  // How many cards are drawn. Everything in range arrives in one reply — a file
  // the page will not send is a file the user cannot find — and what keeps the
  // first paint cheap is drawing a screenful of it and letting the rest wait
  // behind a button. Previews are lazy on top of that, so an undrawn card costs
  // a row of metadata and nothing else.
  const PAGE = 60
  let shown = $state(PAGE)
  // Two-step delete, the same gesture the session list uses: the first click
  // arms the row, the second one does it. These are the user's files.
  let confirmPath = $state('')

  async function refresh() {
    const page = await ListArtifactsIn(range)
    files = page.files ?? []
    served = (RANGES as readonly string[]).includes(page.range) ? (page.range as Range) : 'all'
    total = page.total
    // Back to one screenful whenever the set changes underneath: keeping the
    // old count would paint six hundred cards the moment someone switches to
    // "ทั้งหมด", which is the cost this whole arrangement exists to avoid.
    shown = PAGE
    previews = {}
    loaded = true
  }

  function pick(next: Range) {
    range = next
    void refresh()
  }

  onMount(refresh)

  // The cards actually drawn, and the day heading each one falls under.
  //
  // dayBucket is shared with the sidebar's history and the office's job feed —
  // its own comment says why it lives outside all three, and this is the third
  // caller of the same question. A heading is emitted only when the bucket
  // changes, so the grid reads as a timeline rather than a wall.
  const visible = $derived(files.slice(0, shown))
  const rows = $derived(
    visible.map((f, i) => ({
      file: f,
      head: i === 0 || dayBucket(f.modified) !== dayBucket(visible[i - 1].modified)
        ? dayBucket(f.modified)
        : null,
    })),
  )

  async function open(file: main.Artifact) {
    error = ''
    try {
      await OpenArtifact(file.path)
    } catch (err) {
      error = String(err)
      await refresh() // it was probably deleted underneath us — say so by redrawing
    }
  }

  async function remove(file: main.Artifact) {
    if (confirmPath !== file.path) {
      confirmPath = file.path
      return
    }
    confirmPath = ''
    error = ''
    try {
      await DeleteArtifact(file.path)
    } catch (err) {
      error = String(err)
    }
    await refresh()
  }

  async function openSource(file: main.Artifact) {
    if (!file.sessionId) return
    // The view moves first, the transcript follows — see Office.svelte.
    setActiveView('chat')
    await selectGlobalSession({ id: file.sessionId, title: '', ago: '' })
  }

  function size(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  }

  // ---------- What is inside ----------
  // A grid of filenames answers "what is it called", which is the one thing a
  // person has already forgotten by the time they come looking: two .docx
  // named สรุปผล… and นิสัย… are the same card twice until you can see a line of
  // either. So every card shows its own first few lines — the file rendered,
  // not described.
  //
  // Fetched per card as it scrolls into view, never up front. The sweep is
  // capped at 500 rows and cracking 500 zips open to paint a grid nobody has
  // scrolled to is how a gallery comes to feel broken.
  let previews = $state<Record<string, main.ArtifactPreview | 'loading'>>({})

  // The order previews were last looked at, oldest first, so the ones furthest
  // behind are the ones dropped.
  //
  // Without this the cache only ever grew: an .html artifact previews as a live
  // <iframe srcdoc> and an image as a base64 data URL, so scrolling a long
  // gallery left a hundred documents and a hundred megabytes of string behind
  // it, all of them off screen. A cap of a few screens keeps a scroll back up
  // instant while bounding what the page can hold.
  const PREVIEW_KEEP = 90
  let recent: string[] = []

  function touch(path: string) {
    const at = recent.indexOf(path)
    if (at >= 0) recent.splice(at, 1)
    recent.push(path)
    while (recent.length > PREVIEW_KEEP) {
      const drop = recent.shift()
      if (drop && drop !== path) delete previews[drop]
    }
  }

  async function loadPreview(path: string) {
    touch(path)
    if (previews[path]) return
    previews[path] = 'loading'
    try {
      previews[path] = await ArtifactPreview(path)
    } catch {
      // A file deleted underneath us, or one this side will not read. The card
      // keeps its icon; a preview is a bonus, never the reason the row exists.
      previews[path] = { kind: 'none' } as main.ArtifactPreview
    }
  }

  // Svelte action: ask for this card's preview the first time it is on screen.
  function whenVisible(el: HTMLElement, path: string) {
    // No observer means no viewport to observe (jsdom, an old webview) — so
    // ask straight away rather than leaving every card blank forever. Laziness
    // is an optimisation here; the preview is the feature.
    if (typeof IntersectionObserver === 'undefined') {
      void loadPreview(path)
      return
    }
    // Still observing after the first hit, unlike before: a preview can now be
    // dropped by the cache while its card stays in the list, and unobserving
    // would leave that card permanently blank on the way back up.
    const io = new IntersectionObserver((entries) => {
      for (const e of entries) {
        if (!e.isIntersecting) continue
        void loadPreview(path)
      }
    }, { rootMargin: '200px' }) // a screen ahead, so scrolling meets a drawn card
    io.observe(el)
    return { destroy: () => io.disconnect() }
  }

  // An .html artifact is shown as the page it is, inside a sandboxed frame.
  //
  // Not through the markdown renderer, which is the other way this could go:
  // that pipeline deletes a <style> outside a drawing on purpose (a stylesheet
  // in the app's own document is how a produced file would restyle the app
  // around it), and a brand page stripped of its stylesheet previews as a stack
  // of unstyled headings — the opposite of showing what is inside. sandbox=""
  // is the whole isolation: no scripts, no forms, no navigation, no same-origin.
  const FRAME_W = 900
  const FRAME_SCALE = 0.34

  function markOf(name: string): IconName {
    const ext = name.split('.').pop()?.toLowerCase() ?? ''
    if (['docx', 'pdf', 'md', 'txt'].includes(ext)) return 'fileText'
    if (['xlsx', 'csv'].includes(ext)) return 'chartColumn'
    if (ext === 'pptx') return 'layoutList'
    if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'svg'].includes(ext)) return 'eye'
    return ext ? 'fileCode' : 'package'
  }
</script>

<div class="page-shell">
  <header class="page-head">
    <button class="settings-back" onclick={onClose}><Icon name="arrowLeft" size={14} /> {t('settings.backToApp')}</button>
    <div class="page-title">
      <h2>{t('desk.artifacts')}</h2>
      <p>{t('artifacts.intro')}</p>
    </div>
  </header>

  <div class="page-body">
    <div class="settings-inner wide">
      {#if error}<div class="page-error">{error}</div>{/if}
      {#if loaded && files.length === 0}
        <div class="page-empty">
          <Icon name="package" size={22} />
          <p>{t('artifacts.empty')}</p>
        </div>
      {/if}
      <!-- Which slice of time is on screen. Bound to `served`, not to what was
           clicked: an empty week widens on the engine's side, and the control
           has to say what you are actually looking at. -->
      {#if loaded && (files.length > 0 || served !== 'week')}
        <div class="art-ranges">
          {#each RANGES as r (r)}
            <button type="button" class="art-range" class:on={served === r} onclick={() => pick(r)}>
              {t(`artifacts.range.${r}`)}
            </button>
          {/each}
          <span class="art-count">{t('artifacts.count', { n: String(total) })}</span>
        </div>
      {/if}
      <div class="art-grid">
        {#each rows as row (row.file.path)}
          {@const f = row.file}
          {@const p = previews[f.path]}
          {#if row.head}
            <!-- A day heading spans the whole grid, so the cards under it read
                 as that day's work rather than as a run of unrelated tiles. -->
            <h3 class="art-day">{t(row.head)}</h3>
          {/if}
          <div class="art-card" use:whenVisible={f.path}>
            <button class="art-open" onclick={() => open(f)} title={f.path}>
              <!-- The look inside. Kept above the name rather than beside it:
                   this is the thing the eye should land on, and the name is the
                   caption under it. A file with no cheap preview (a PDF, a zip)
                   shows its mark large in the same box, so every card is the
                   same shape whether or not it could be read. -->
              <span class="art-thumb" class:plain={!p || p === 'loading' || p.kind === 'none'}>
                {#if p && p !== 'loading' && p.kind === 'image'}
                  <img class="art-thumb-img" src={p.dataUrl} alt="" loading="lazy" />
                {:else if p && p !== 'loading' && p.kind === 'html'}
                  <iframe
                    class="art-thumb-frame" title={f.name} sandbox="" srcdoc={p.text}
                    style="width:{FRAME_W}px; height:{Math.round(360 / FRAME_SCALE)}px; transform:scale({FRAME_SCALE})"
                  ></iframe>
                {:else if p && p !== 'loading' && p.kind === 'markdown'}
                  <div class="art-thumb-md markdown-body">{@html renderMarkdown(p.text ?? '')}</div>
                {:else if p && p !== 'loading' && p.kind === 'sheet'}
                  <table class="art-thumb-sheet">
                    <tbody>
                      {#each p.rows ?? [] as row, i}
                        <tr>{#each row as cell}<td class:head={i === 0}>{cell}</td>{/each}</tr>
                      {/each}
                    </tbody>
                  </table>
                {:else if p && p !== 'loading' && p.kind === 'text'}
                  <pre class="art-thumb-text">{p.text}</pre>
                {:else}
                  <span class="art-mark lg"><Icon name={markOf(f.name)} size={26} /></span>
                {/if}
              </span>
              <span class="art-name-row">
                <span class="art-mark"><Icon name={markOf(f.name)} size={14} /></span>
                <span class="art-name">{f.name}</span>
              </span>
              <span class="art-meta">{size(f.size)} · {agoLabel(f.modified)}</span>
            </button>
            <div class="art-foot">
              {#if f.sessionId}
                <button class="linkish" onclick={() => openSource(f)}>{t('artifacts.fromChat')}</button>
              {:else}
                <span class="art-orphan">{t('artifacts.noChat')}</span>
              {/if}
              <button
                class="art-del" class:confirm={confirmPath === f.path}
                aria-label={t('artifacts.delete')}
                onclick={() => remove(f)}
              >
                {#if confirmPath === f.path}{t('sidebar.confirmDelete')}{:else}<Icon name="x" size={12} />{/if}
              </button>
            </div>
          </div>
        {/each}
      </div>
      <!-- Everything in range is already here; this only decides how much is
           drawn. The count is the point — "แสดงเพิ่ม" alone does not say whether
           it is hiding four files or four hundred. -->
      {#if files.length > shown}
        <button type="button" class="art-more" onclick={() => (shown += PAGE)}>
          <Icon name="chevronDown" size={13} />
          {t('artifacts.more', { n: String(files.length - shown) })}
        </button>
      {/if}
    </div>
  </div>
</div>
