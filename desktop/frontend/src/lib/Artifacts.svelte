<script lang="ts">
  // ผลงาน (COMPANY.md §2): every file Aetox has made, read live off the disk.
  //
  // There is no index table behind this and there deliberately never will be —
  // the folder is the half users move, rename and delete without telling us, so
  // an index would show files that are gone and hide files that are there.
  // Deleting a conversation leaves its work alone (§6.7); this page is the one
  // place a produced file is deleted, by the user, on purpose.
  import { onMount } from 'svelte'
  import { ListArtifactsIn, OpenArtifact, DeleteArtifact, ArtifactPreview, CompressArtifacts } from '../../wailsjs/go/main/App'
  import { main } from '../../wailsjs/go/models'
  import { agoLabel, selectGlobalSession, setActiveView } from './stores/cockpit.svelte'
  import { dayBucket, daysAgo } from './dayBucket'
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

  // A menu that only closes by choosing from it is a trap. Escape and a click
  // anywhere else both mean "never mind", and both are what every other menu on
  // the machine does.
  onMount(() => {
    const away = (e: Event) => {
      if (!(e.target as HTMLElement)?.closest?.('.art-span')) spanOpen = false
    }
    const esc = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      if (spanOpen) spanOpen = false
      else if (pickedCount > 0) clearPicked()
    }
    document.addEventListener('pointerdown', away, true)
    document.addEventListener('keydown', esc)
    return () => {
      document.removeEventListener('pointerdown', away, true)
      document.removeEventListener('keydown', esc)
    }
  })

  // The cards actually drawn, and the day heading each one falls under.
  //
  // dayBucket is shared with the sidebar's history and the office's job feed —
  // its own comment says why it lives outside all three, and this is the third
  // caller of the same question. A heading is emitted only when the bucket
  // changes, so the grid reads as a timeline rather than a wall.
  const visible = $derived(files.slice(0, shown))

  // A subfolder under a session is one thing, so it gets one card.
  //
  // The gallery's unit was the file, and for a deliverable that is right — it is
  // what the user asked for and what they came here to find. It is wrong for
  // everything that arrives in sets: the screenshots taken while reading (46 of
  // 244 files on the owner's machine, nine in one session), the pages of an
  // exported site, the frames of an animation. Drawn flat, the one document
  // somebody asked for is the tenth card in a row of ten that look alike.
  //
  // The key is the folder the engine reports (Artifact.Folder), not a filename
  // pattern — page-1.png reads as a set until the day a tool names something
  // else that way, and then the gallery is grouping by coincidence. Root and
  // session are in the key too: two projects may both hold a work/ folder, and
  // they are not the same pile.
  //
  // A day heading belongs to the row that carries it, so `bucket` only advances
  // on a row that is actually drawn. Advancing it on a file swallowed into a
  // deck loses the heading for that day, and the next file lands silently under
  // the day before.
  type Day = ReturnType<typeof dayBucket>
  type Row =
    | { kind: 'file'; key: string; head: Day | null; file: main.Artifact }
    | { kind: 'deck'; key: string; head: Day | null; folder: string; files: main.Artifact[] }

  const rows = $derived.by(() => {
    const out: Row[] = []
    const deckAt = new Map<string, number>()
    let bucket: Day | null = null
    for (const f of visible) {
      const key = f.folder ? `${f.root}|${f.sessionId ?? ''}|${f.folder}` : ''
      const at = key ? deckAt.get(key) : undefined
      if (at !== undefined) {
        const deck = out[at]
        if (deck.kind === 'deck') deck.files.push(f)
        continue
      }
      const day = dayBucket(f.modified)
      const head = day === bucket ? null : day
      bucket = day
      if (key) {
        deckAt.set(key, out.length)
        out.push({ kind: 'deck', key, head, folder: f.folder ?? '', files: [f] })
      } else {
        out.push({ kind: 'file', key: f.path, head, file: f })
      }
    }
    return out
  })

  // Which decks the user has opened. Keyed by the deck's key rather than by
  // index: the list is re-derived on every refresh, and an index would reopen
  // whatever happens to land in that slot next.
  let openDecks = $state<Record<string, boolean>>({})

  // work/ is the one folder this app makes for itself, so it is the one folder
  // whose name is translated. Anything else is the agent's own naming, and
  // renaming it on the card would leave the user hunting for a folder that does
  // not exist on disk under that name.
  function deckLabel(folder: string): string {
    return folder === 'work' ? t('artifacts.deckWork') : folder
  }

  function deckSize(items: main.Artifact[]): number {
    return items.reduce((sum, f) => sum + f.size, 0)
  }

  // Deleting a deck deletes the files in it, one door at a time — the same door
  // a single card uses, bounded by the same check on the engine side. No new
  // "delete this folder" binding: a door that removes a directory is a bigger
  // thing to own than this page needs, and the loop is honest about what it does.
  async function removeDeck(row: Extract<Row, { kind: 'deck' }>) {
    if (confirmPath !== row.key) {
      confirmPath = row.key
      return
    }
    confirmPath = ''
    error = ''
    for (const f of row.files) {
      try {
        await DeleteArtifact(f.path)
      } catch (err) {
        error = String(err)
      }
    }
    await refresh()
  }

  // ---------- Picking more than one ----------
  // Deleting fifty screenshots one confirm at a time is not a feature, it is a
  // punishment, and it is what this page offered until now (owner: "บางทีจะไป
  // เคลียร์หรือลบอ่ะลำบากมาก").
  //
  // Two ways in, because they answer different sizes of the same job: a button
  // when the answer is "all of it", and a drag across the grid when the answer
  // is "those, not those". Clicking a card still opens the file — a page whose
  // click means one thing until you turn on a mode, and something else after,
  // is a page you have to look at before you can use it.
  //
  // Paths are the identity. Not indexes into `rows`, which are re-derived on
  // every refresh, and not the cards themselves, which come and go with the
  // range picker while a selection outlives both.
  let picked = $state<Record<string, true>>({})
  const pickedCount = $derived(Object.keys(picked).length)

  // Every file on screen, folders included. A folder card stands for the files
  // inside it, so picking one picks them — the count in the corner is what the
  // user was answering when they clicked it.
  const filesOf = (row: Row) => (row.kind === 'file' ? [row.file] : row.files)
  const shownFiles = $derived(rows.flatMap(filesOf))

  function togglePick(row: Row) {
    const items = filesOf(row)
    const on = items.every((f) => picked[f.path])
    for (const f of items) {
      if (on) delete picked[f.path]
      else picked[f.path] = true
    }
  }

  const rowPicked = (row: Row) => filesOf(row).every((f) => picked[f.path])

  // "Select all" with a way to mean less than all of it.
  //
  // Windows, not a cutoff each: เมื่อวาน is the day before today and nothing
  // else, which is what the heading over those cards says too — both read the
  // day off daysAgo now, so a selection cannot disagree with the heading it is
  // sitting under. The wider spans are rolling, because that is what the range
  // picker beside them already means.
  const SPANS = [
    { key: 'all', label: 'artifacts.spanAll', needs: 'all', within: (_d: number) => true },
    { key: 'today', label: 'sidebar.today', needs: 'week', within: (d: number) => d <= 0 },
    { key: 'yesterday', label: 'sidebar.yesterday', needs: 'week', within: (d: number) => d === 1 },
    { key: 'week', label: 'sidebar.last7Days', needs: 'week', within: (d: number) => d <= 7 },
    { key: 'month', label: 'sidebar.last30Days', needs: 'month', within: (d: number) => d <= 30 },
    { key: 'year', label: 'artifacts.spanYear', needs: 'all', within: (d: number) => d <= 365 },
  ] as const

  let spanOpen = $state(false)

  // Everything loaded, not everything drawn. A press on "ทั้งหมด" over 244 files
  // with 60 cards painted means all 244 — the button says so — and paging is a
  // drawing decision that has no business changing what a selection covers.
  const WIDTH = { week: 0, month: 1, all: 2 } as const

  async function pickSpan(span: (typeof SPANS)[number]) {
    spanOpen = false
    // A span the loaded range cannot contain would select nothing and say
    // nothing about why, so widen first and wait for the files to arrive.
    if (WIDTH[served] < WIDTH[span.needs]) {
      range = span.needs
      await refresh()
    }
    for (const f of files) {
      if (span.within(daysAgo(f.modified))) picked[f.path] = true
    }
  }

  function pickAll() {
    for (const f of files) picked[f.path] = true
  }

  function clearPicked() {
    picked = {}
    confirmPath = ''
  }

  async function removePicked() {
    if (confirmPath !== 'picked') {
      confirmPath = 'picked'
      return
    }
    confirmPath = ''
    error = ''
    for (const path of Object.keys(picked)) {
      try {
        await DeleteArtifact(path)
      } catch (err) {
        error = String(err)
      }
    }
    picked = {}
    await refresh()
  }

  // ---------- Making them smaller ----------
  // Measured on the owner's own gallery before it was built: 46 browser
  // screenshots came to 18.5 MB, and re-encoding them gives back 75-90% of that.
  // The engine side does the deciding about formats; this only asks, and then
  // says what came back, because "compressed" without a number is a claim the
  // user has to go and check in Explorer.
  // One file per call, on purpose.
  //
  // The engine will happily take the whole list and answer once at the end, and
  // that is what this did first: the button said "กำลังบีบอัด..." and then
  // nothing moved for as long as it took, which reads as a hang, not as work
  // (owner: "กดแล้วเหมือนค้าง แสดงด้วยดิว่ากำลังบีบอัดกี่ไฟล์"). A number that
  // climbs is the difference between a program that is busy and one that is
  // stuck, and the only way the page can count is to be the thing doing the
  // counting. The round trip per file is nothing next to decoding and
  // re-encoding it.
  let squeezing = $state<{ done: number; total: number; files: number; saved: number } | null>(null)
  let squeezed = $state<{ files: number; saved: number; skipped: number } | null>(null)

  const IMAGE_EXT = ['png', 'jpg', 'jpeg']
  const isImage = (f: main.Artifact) => IMAGE_EXT.includes(f.name.split('.').pop()?.toLowerCase() ?? '')
  const pickedImages = $derived(shownFiles.filter((f) => picked[f.path] && isImage(f)))

  async function squeeze() {
    if (squeezing || pickedImages.length === 0) return
    const targets = pickedImages
    squeezing = { done: 0, total: targets.length, files: 0, saved: 0 }
    squeezed = null
    error = ''
    let skipped = 0
    try {
      for (const f of targets) {
        const report = await CompressArtifacts([f.path])
        skipped += report.skipped
        if (report.error && !error) error = report.error
        squeezing = {
          done: squeezing.done + 1,
          total: targets.length,
          files: squeezing.files + report.files,
          saved: squeezing.saved + Math.max(0, report.before - report.after),
        }
      }
      squeezed = { files: squeezing.files, saved: squeezing.saved, skipped }
      picked = {}
      await refresh()
    } catch (err) {
      error = String(err)
    } finally {
      squeezing = null
    }
  }

  // Drag a box across the grid and everything it touches is picked.
  //
  // Started only on the grid's own background: a drag that begins on a card is
  // the user reaching for the card, and hijacking it would make every card
  // click feel unreliable. Below a few pixels it is a click, not a drag, which
  // is what keeps a slightly shaky click on empty space from wiping a selection.
  let band = $state<{ x: number; y: number; w: number; h: number } | null>(null)
  let bandFrom: { x: number; y: number } | null = null
  let bandAdds: string[] = []

  function bandStart(e: PointerEvent) {
    if (e.button !== 0) return
    const el = e.target as HTMLElement
    if (el.closest('.art-card') || el.closest('button')) return
    const grid = e.currentTarget as HTMLElement
    const box = grid.getBoundingClientRect()
    bandFrom = { x: e.clientX - box.left, y: e.clientY - box.top }
    bandAdds = []
    grid.setPointerCapture(e.pointerId)
  }

  function bandMove(e: PointerEvent) {
    if (!bandFrom) return
    const grid = e.currentTarget as HTMLElement
    const box = grid.getBoundingClientRect()
    const x = e.clientX - box.left
    const y = e.clientY - box.top
    const rect = {
      x: Math.min(x, bandFrom.x), y: Math.min(y, bandFrom.y),
      w: Math.abs(x - bandFrom.x), h: Math.abs(y - bandFrom.y),
    }
    if (rect.w < 4 && rect.h < 4) return
    band = rect
    // What the box covers right now, recomputed rather than accumulated: drag
    // back over a card and it lets go again, which is what a selection box does
    // everywhere else.
    for (const path of bandAdds) delete picked[path]
    bandAdds = []
    for (const card of grid.querySelectorAll<HTMLElement>('[data-paths]')) {
      const r = card.getBoundingClientRect()
      const cx = r.left - box.left, cy = r.top - box.top
      const hit = cx < rect.x + rect.w && cx + r.width > rect.x &&
                  cy < rect.y + rect.h && cy + r.height > rect.y
      if (!hit) continue
      for (const path of (card.dataset.paths ?? '').split(' ')) {
        if (!path || picked[path]) continue
        picked[path] = true
        bandAdds.push(path)
      }
    }
  }

  function bandEnd(e: PointerEvent) {
    const grid = e.currentTarget as HTMLElement
    if (grid.hasPointerCapture(e.pointerId)) grid.releasePointerCapture(e.pointerId)
    bandFrom = null
    bandAdds = []
    band = null
  }

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
          <!-- Right-hand end of the same row the ranges live on, because these
               act on what that row is showing. Two buttons at rest: take all of
               it, or drag a box over the part you meant. -->
          <div class="art-bulk">
            {#if pickedCount > 0}
              <span class="art-picked">{t('artifacts.pickedCount', { n: String(pickedCount) })}</span>
              {#if pickedImages.length > 0 || squeezing}
                <button type="button" class="art-range" disabled={!!squeezing} onclick={squeeze}>
                  {#if squeezing}
                    {t('artifacts.squeezingN', {
                      done: String(squeezing.done), total: String(squeezing.total),
                    })}
                  {:else}
                    {t('artifacts.squeeze', { n: String(pickedImages.length) })}
                  {/if}
                </button>
              {/if}
              <button
                type="button" class="art-range danger" class:on={confirmPath === 'picked'}
                onclick={removePicked}
              >
                {confirmPath === 'picked'
                  ? t('artifacts.reallyDelete', { n: String(pickedCount) })
                  : t('artifacts.deletePicked')}
              </button>
              <button type="button" class="art-range" onclick={clearPicked}>{t('artifacts.clearPicked')}</button>
            {:else}
              <!-- One press for the common answer, a chevron for the rest. A
                   menu that has to be opened before "all of it" can be said
                   would put a click in front of the thing people want most. -->
              <div class="art-span">
                <button type="button" class="art-range" onclick={pickAll}>{t('artifacts.pickAll')}</button>
                <button
                  type="button" class="art-range art-span-more" class:on={spanOpen}
                  aria-label={t('artifacts.pickSpan')} aria-expanded={spanOpen}
                  onclick={() => (spanOpen = !spanOpen)}
                ><Icon name="chevronDown" size={12} /></button>
                {#if spanOpen}
                  <!-- svelte-ignore a11y_no_static_element_interactions -->
                  <div class="art-span-menu" role="menu">
                    {#each SPANS as span (span.key)}
                      <button type="button" role="menuitem" onclick={() => pickSpan(span)}>
                        {t(span.label)}
                      </button>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}
          </div>
        </div>
      {/if}
      <!-- What the last run gave back. Sits above the grid rather than in a
           toast: it is the answer to the thing the user just asked for, and an
           answer that disappears on its own is one they have to redo to read. -->
      {#if squeezing}
        <!-- The same strip the summary lands in, so the number the user watches
             climb is the number that is still there when it stops. -->
        <div class="art-squeezed">
          <span class="art-squeeze-spin"><Icon name="loaderCircle" size={13} /></span>
          <span>{t('artifacts.squeezingN', {
            done: String(squeezing.done), total: String(squeezing.total),
          })}</span>
          <span class="art-orphan">{t('artifacts.squeezeSoFar', { size: size(squeezing.saved) })}</span>
          <span class="art-squeeze-bar" style="--p:{Math.round((squeezing.done / squeezing.total) * 100)}%"></span>
        </div>
      {:else if squeezed}
        <div class="art-squeezed">
          <Icon name="check" size={13} />
          <span>{t('artifacts.squeezed', { n: String(squeezed.files), size: size(squeezed.saved) })}</span>
          {#if squeezed.skipped > 0}
            <span class="art-orphan">{t('artifacts.squeezeSkipped', { n: String(squeezed.skipped) })}</span>
          {/if}
          <button type="button" class="art-del" aria-label={t('artifacts.clearPicked')} onclick={() => (squeezed = null)}>
            <Icon name="x" size={12} />
          </button>
        </div>
      {/if}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="art-grid" class:picking={pickedCount > 0}
        onpointerdown={bandStart} onpointermove={bandMove}
        onpointerup={bandEnd} onpointercancel={bandEnd}
      >
        {#if band}
          <div class="art-band" style="left:{band.x}px; top:{band.y}px; width:{band.w}px; height:{band.h}px"></div>
        {/if}
        {#each rows as row (row.key)}
          {#if row.head}
            <!-- A day heading spans the whole grid, so the cards under it read
                 as that day's work rather than as a run of unrelated tiles. -->
            <h3 class="art-day">{t(row.head)}</h3>
          {/if}
          {#if row.kind === 'file'}
            {@render fileCard(row.file)}
          {:else}
            <!-- A folder is a card like any other card, wearing the
                 preview of the newest file in it and a count in the corner.
                 That is the whole of it, and it is where four other designs
                 landed and were thrown out on the way (a stack with paper
                 edges, a fanned pile, a seamless mosaic, a wide card spanning
                 two columns). The owner's call, and the right one: a gallery of
                 work is a grid of equal tiles, and every variant that made the
                 folder look special did it by making the folder look different
                 from the files, which is the one thing the grid is for.
                 The count is the only difference it needs — the preview already
                 says what is inside, and the number says how much of it. -->
            <div
              class="art-card" class:picked={rowPicked(row)}
              data-paths={row.files.map((f) => f.path).join(' ')}
              use:whenVisible={row.files[0].path}
            >
              {@render tick(row)}
              <button
                class="art-open" onclick={() => (openDecks[row.key] = !openDecks[row.key])}
                title={row.files.map((f) => f.name).join('\n')}
                aria-expanded={!!openDecks[row.key]}
              >
                {@render thumb(row.files[0], row.files.length)}
                <span class="art-name-row">
                  <span class="art-mark"><Icon name={openDecks[row.key] ? 'folderOpen' : 'folder'} size={14} /></span>
                  <span class="art-name">{deckLabel(row.folder)}</span>
                  <span class="art-chev" class:on={openDecks[row.key]}><Icon name="chevronDown" size={12} /></span>
                </span>
                <span class="art-meta">
                  {t('artifacts.count', { n: String(row.files.length) })} · {size(deckSize(row.files))} · {agoLabel(row.files[0].modified)}
                </span>
              </button>
              <div class="art-foot">
                {#if row.files[0].sessionId}
                  <button class="linkish" onclick={() => openSource(row.files[0])}>{t('artifacts.fromChat')}</button>
                {:else}
                  <span class="art-orphan">{t('artifacts.noChat')}</span>
                {/if}
                <button
                  class="art-del" class:confirm={confirmPath === row.key}
                  aria-label={t('artifacts.deleteDeck')}
                  onclick={() => removeDeck(row)}
                >
                  {#if confirmPath === row.key}{t('sidebar.confirmDelete')}{:else}<Icon name="x" size={12} />{/if}
                </button>
              </div>
            </div>
            {#if openDecks[row.key]}
              {#each row.files as f (f.path)}
                {@render fileCard(f)}
              {/each}
            {/if}
          {/if}
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

{#snippet fileCard(f: main.Artifact)}
          <div class="art-card" class:picked={picked[f.path]} data-paths={f.path} use:whenVisible={f.path}>
            {@render tick({ kind: 'file', key: f.path, head: null, file: f })}
            <button class="art-open" onclick={() => open(f)} title={f.path}>
              {@render thumb(f, 0)}
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
{/snippet}

<!-- The look inside, and the only place that decides it. Kept above the name
     rather than beside it: this is the thing the eye should land on, and the
     name is the caption under it. A file with no cheap preview (a PDF, a zip)
     shows its mark large in the same box, so every card is the same shape
     whether or not it could be read.

     One snippet for a file and for a folder, because they are the same picture
     of the same kind of thing. Written twice, the folder card had already lost
     .html and spreadsheet previews within the hour — the copy simply had fewer
     branches than the original, and nothing said so. `count` is the whole
     difference: 0 for a file, the size of the pile for a folder. -->
{#snippet thumb(f: main.Artifact, count: number)}
  {@const p = previews[f.path]}
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
    {#if count > 0}<span class="art-deck-count">{count}</span>{/if}
  </span>
{/snippet}

<!-- The tick. Its own button on top of the card rather than a mode the whole
     page enters, so a click on the card still opens the file and a click on the
     tick still means "this one" — nothing about the card changes meaning
     depending on what was pressed earlier.
     Hidden until the card is hovered, or until something is picked: a grid of
     work does not need forty checkboxes drawn on it to look at. -->
{#snippet tick(row: Row)}
  {@const on = rowPicked(row)}
  <button
    type="button" class="art-tick" class:on
    aria-pressed={on} aria-label={t('artifacts.pickThis')}
    onclick={(e) => { e.stopPropagation(); togglePick(row) }}
  >
    {#if on}<Icon name="check" size={11} />{/if}
  </button>
{/snippet}
