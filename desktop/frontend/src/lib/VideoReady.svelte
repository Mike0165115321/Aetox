<script lang="ts">
  // What this machine can already do with video, said once, before anybody is
  // asked to download anything.
  //
  // Owner, 30 ส.ค.: *"สำหรับใช้งานครั้งแรก กดแล้วขึ้นว่า Aetox กำลังตรวจดูว่า
  // เครื่องนี้พร้อมทำงานวิดีโอแค่ไหน แล้วให้มันแสดง Ui ลิสต์มาว่ามีอันนี้ ๆ แล้ว
  // ขาดอะไร ๆ กดเพื่อติดตั้งทั้งหมด"* — and, in the same breath, the rule that
  // decides whether it is any good: *"เช็คดี ๆ นะ แม่งไม่ใช่ยัดหนี้ทางเทคนิคเข้าไป
  // เผื่อมันมีอยู่แล้วแต่ระบบตรวจไม่เจออีก"*.
  //
  // So the panel's whole value is in the check behind it, not in the layout.
  // App.VideoReadiness looks past PATH into scoop, chocolatey, winget and
  // Program Files, because a GUI process started before an install has a stale
  // PATH and would otherwise offer to fetch 90MB the user already has. And it
  // runs `ffmpeg -encoders` on whatever it found, because Aetox itself ships an
  // ffmpeg with no libx264: a check that stopped at "there is a file called
  // ffmpeg" would draw a green tick and fail on the first render.
  //
  // Every row is drawn, including the ones that are fine. A panel that lists
  // only what is broken is a list of faults; this is meant to be an account of
  // the machine.
  import { onMount } from 'svelte'
  import { VideoReadiness, InstallCapabilities, MarkVideoCheckSeen } from '../../wailsjs/go/main/App'
  import { main } from '../../wailsjs/go/models'
  import { capabilities, noteCapabilityRequest } from './capabilities.svelte'
  import { t, type TKey } from './i18n.svelte'
  import Icon from './Icon.svelte'
  import type { IconName } from './icons'

  let { agent, onClose }: {
    /** Whose readiness this is. The two video agents do not need the same
     *  things — one cuts footage that exists and never renders a scene — and a
     *  panel opened from one card answers for that card. */
    agent: string
    onClose: () => void
  } = $props()

  let report = $state<main.VideoReadiness | null>(null)
  let checking = $state(true)
  const busy = $derived(capabilities.phase === 'installing')

  async function check() {
    checking = true
    try {
      report = await VideoReadiness(agent)
    } finally {
      checking = false
    }
  }

  onMount(async () => {
    // Marked on open rather than on action: opening once is the promise, and
    // somebody who read it and closed it has still been told.
    MarkVideoCheckSeen()
    await check()
  })

  // Re-checked after a download rather than assumed: the point of this panel is
  // that it reports what is on the machine, and it would be a poor one if the
  // last thing it said were a guess about its own work.
  let lastPhase = $state('')
  $effect(() => {
    if (capabilities.phase === lastPhase) return
    const was = lastPhase
    lastPhase = capabilities.phase
    if (was === 'installing') void check()
  })

  async function installMissing() {
    // A list, because the two video jobs stopped sharing one download: making a
    // video fetches the renderer and the browser it drives, cutting one fetches
    // the editor, and both fetch the ffmpeg they encode with. The Go side
    // answers with whichever pair this card needs (videoCapabilitiesFor).
    const what = report?.capabilities ?? []
    if (what.length === 0) return
    noteCapabilityRequest(what)
    try {
      await InstallCapabilities(what)
    } catch {
      /* the strip over the window reports it */
    }
  }

  const LABEL: Record<string, TKey> = {
    editor: 'ready.editor',
    templates: 'ready.templates',
    ffmpeg: 'ready.ffmpeg',
    h264: 'ready.h264',
    renderer: 'ready.renderer',
    browser: 'ready.browser',
    gsap: 'ready.gsap',
  }
  // What a row says when the check found nothing, per row: "not found anywhere
  // we looked" is a different sentence from "this one is optional".
  const SAID: Record<string, TKey> = {
    'editor.missing': 'ready.editorMissing',
    'ffmpeg.missing': 'ready.ffmpegMissing',
    'h264.warn': 'ready.h264Warn',
    'renderer.missing': 'ready.rendererMissing',
    'browser.missing': 'ready.browserMissing',
    'gsap.warn': 'ready.gsapWarn',
    'templates.ok': 'ready.templatesOk',
  }
  const MARK: Record<string, IconName> = {
    ok: 'check',
    missing: 'x',
    warn: 'alertTriangle',
    optional: 'dot',
  }

  const mb = (bytes: number) => Math.round(bytes / (1 << 20))
  // A path is the honest answer to "where", and the tail is the part that
  // identifies it. The head is a data folder nobody reads twice.
  const short = (path: string) => (path.length > 46 ? '…' + path.slice(-45) : path)
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="confirm-overlay" role="dialog" tabindex="-1" aria-modal="true"
  aria-labelledby="ready-title"
  onkeydown={(e) => { if (e.key === 'Escape') { e.stopPropagation(); onClose() } }}>
  <button class="confirm-backdrop" aria-label={t('settings.close')} onclick={onClose}></button>
  <div class="confirm-card ready-card">
    <h3 id="ready-title" class="confirm-title">{t('ready.title')}</h3>
    <p class="confirm-message">{checking ? t('ready.checking') : t('ready.intro')}</p>

    {#if report}
      <div class="ready-list">
        {#each report.rows as row (row.id)}
          <div class="ready-row" class:miss={row.state === 'missing'} class:warn={row.state === 'warn'}>
            <Icon name={MARK[row.state] ?? 'dot'} size={16} />
            <span class="k">{t(LABEL[row.id] ?? 'ready.editor')}</span>
            <span class="d">
              {#if SAID[`${row.id}.${row.state}`]}
                {t(SAID[`${row.id}.${row.state}`])}
              {:else if row.where}
                {short(row.where)}
              {/if}
            </span>
            {#if row.state !== 'ok' && row.bytes > 0}
              <span class="sz">{t('lock.mb', { n: mb(row.bytes) })}</span>
            {/if}
          </div>
        {/each}
      </div>
    {/if}

    <div class="confirm-actions ready-actions">
      <span class="ready-sum">
        {#if checking}{t('ready.checking')}
        {:else if report?.ready}{t('ready.allSet')}
        {:else if report && report.missingBytes > 0}{t('ready.missing', { n: mb(report.missingBytes) })}
        <!-- Missing, but nobody knows how big yet: a component that is not in
             the manifest reports zero bytes, and summing that to "nothing to
             fetch" put a reassuring sentence under two red crosses. -->
        {:else if report && report.capabilities?.length}{t('ready.missingUnknown')}
        <!-- Red rows and nothing this build can fetch for them. Said plainly
             rather than hidden behind "all set": the reader is looking at two
             crosses, and the one thing they must not be told is that there is
             nothing to do. -->
        {:else if report && !report.ready}{t('ready.missingNoFetch')}
        {:else}{t('ready.nothingToFetch')}{/if}
      </span>
      <button class="ctrl" disabled={checking || busy} onclick={check}>{t('ready.recheck')}</button>
      <!-- Aetox installs it, or it says plainly that it cannot yet. There is
           no third button sending somebody to a download page: that was asked
           for once and built anyway, twice, and it was never the answer. -->
      {#if report && report.capabilities?.length && !report.ready}
        <button class="ctrl ctrl-primary" disabled={busy || checking} onclick={installMissing}>
          <Icon name="download" size={14} />
          {busy ? t('lock.installing') : t('ready.installAll')}
        </button>
      {:else}
        <button class="ctrl ctrl-primary" onclick={onClose}>{t('ready.done')}</button>
      {/if}
    </div>
  </div>
</div>
