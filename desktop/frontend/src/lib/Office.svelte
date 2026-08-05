<script lang="ts">
  // ทีมเอเจน (COMPANY.md §4): the roster, and the work the team has taken in.
  //
  // Two lists and no state of its own. A chair is a profile file, and the jobs
  // feed is a query over `jobs` — the rows every delegation already writes. The
  // page was specified as "a roster plus a feed, no new state, no inbox", and
  // that is exactly as much as this does: it reads, and it lets you walk to the
  // conversation a job came from.
  import { onMount } from 'svelte'
  import { ListChairs, ListReceivedJobs, OpenSubagentsFolder } from '../../wailsjs/go/main/App'
  import { main } from '../../wailsjs/go/models'
  import { agoLabel, newChairSession, selectGlobalSession, setActiveView } from './stores/cockpit.svelte'
  import { t } from './i18n.svelte'
  import Icon from './Icon.svelte'

  let { onClose }: { onClose: () => void } = $props()

  let chairs = $state<main.Chair[]>([])
  let jobs = $state<main.ReceivedJob[]>([])
  let loaded = $state(false)

  async function refresh() {
    const [roster, feed] = await Promise.all([ListChairs(), ListReceivedJobs(30)])
    chairs = roster
    jobs = feed
    loaded = true
  }

  onMount(refresh)

  // Walking from a job to the conversation that sent it. The job row carries
  // the caller's session id, which is the only link there is — and the only one
  // there needs to be, since the file it produced went to that session's folder.
  async function openSource(job: main.ReceivedJob) {
    if (!job.sessionId) return
    // The view moves first and the transcript follows. Loading a session
    // switches project, workbench and history behind it — leaving the user
    // looking at this page until all of that lands reads as a dead click.
    setActiveView('chat')
    await selectGlobalSession({ id: job.sessionId, title: '', ago: '' })
  }

  function secs(ms: number): string {
    return `${(ms / 1000).toFixed(1)}s`
  }

  // Walking into an agent's room (§85): a fresh session bound to that agent —
  // its tools, its memory, its prompt. The view moves first for the same
  // reason openSource's does: a click that waits for a bootstrap before
  // showing anything reads as a dead click.
  async function talkTo(chair: main.Chair) {
    setActiveView('chat')
    await newChairSession(chair.name)
  }

  // Same name-to-hue hash the preset gallery uses (Settings.svelte), so an
  // agent keeps one colour for life and the two galleries read as one system.
  function coverHue(name: string): number {
    let h = 0
    for (const ch of name) h = (h * 31 + ch.codePointAt(0)!) % 360
    return h
  }

  // The card shows six chips, so order decides what a face says. Alphabetical
  // order buried `slides_write` behind the +N on the very card it defines —
  // the writers are the office's signature, so they come first; everything
  // else keeps the engine's order.
  function orderedTools(c: main.Chair): string[] {
    const all = c.tools ?? []
    return [...all.filter((tool) => tool.endsWith('_write')), ...all.filter((tool) => !tool.endsWith('_write'))]
  }
</script>

<div class="page-shell">
  <header class="page-head">
    <button class="settings-back" onclick={onClose}><Icon name="arrowLeft" size={14} /> {t('settings.backToApp')}</button>
    <div class="page-title">
      <h2>{t('desk.office')}</h2>
      <p>{t('office.intro')}</p>
    </div>
  </header>

  <div class="page-body">
    <div class="settings-inner">
      <div class="eyebrow section-label">{t('office.roster')}</div>
      <!-- The same card system the preset gallery uses (pp-*): one visual
           language for "things you pick from a shelf". Difference worth
           keeping: these cards are the actual staff, with history — not
           templates to instantiate. -->
      <div class="pp-grid office-grid">
        <button class="pp-card pp-new" onclick={() => OpenSubagentsFolder()}>
          <span class="pp-plus">+</span>
          <span class="pp-newtxt">{t('office.newAgent')}</span>
        </button>
        {#each chairs as c (c.name)}
          <div class="pp-card chair-card">
            <span class="pp-cover" style="--h:{coverHue(c.name)}">
              <span class="pp-mono">{c.name}</span>
            </span>
            <div class="pp-body">
              <span class="pp-title">
                {c.name}
                {#if c.builtin}<span class="badge on">{t('settings.promptBuiltin')}</span>{/if}
                {#if c.overrides}<span class="badge">{t('office.overrides')}</span>{/if}
              </span>
              <span class="pp-desc">{c.description}</span>
              <!-- What the chair actually gets, after the office ceiling — not
                   what its file asked for. This is the card a person checks the
                   ceiling on, so showing the request would defeat it. Capped at
                   six on the card — a card is a face, not an inventory — with
                   the full list one hover away on the +N chip. -->
              <div class="chips">
                {#each orderedTools(c).slice(0, 6) as tool (tool)}<span class="chip">{tool}</span>{/each}
                {#if orderedTools(c).length > 6}
                  <span class="chip more" title={orderedTools(c).join(' · ')}>+{orderedTools(c).length - 6}</span>
                {/if}
              </div>
            </div>
            <div class="chair-foot">
              {#if c.jobs > 0}
                <span class="stat"><span class="n">{c.jobs}</span> {t('office.jobsDone')} · {agoLabel(c.lastUsed ?? '')}</span>
              {:else}
                <span class="stat idle">{t('office.neverUsed')}</span>
              {/if}
              <button class="ctrl chair-chat" onclick={() => talkTo(c)}>
                <Icon name="sparkles" size={12} /> {t('office.chat')}
              </button>
            </div>
          </div>
        {/each}
        {#if loaded && chairs.length === 0}
          <div class="pp-card chair-card empty"><div class="pp-body"><span class="pp-desc">{t('office.noChairs')}</span></div></div>
        {/if}
      </div>
      <p class="page-note">{t('office.hiringNote')}</p>
      <!-- Where the rest of them are. This page is the roster — who takes work
           and what they have done — and it is not every profile the engine
           runs: the assistant's own delegates never sit here. Saying so is what
           keeps two pages from reading as one list that disagrees with itself. -->
      <p class="page-note">{t('office.settingsNote')}</p>

      <div class="eyebrow section-label">{t('office.feed')}</div>
      <div class="settings-card">
        {#each jobs as j (j.id)}
          <div class="set-row job-row">
            <div class="set-txt">
              <div class="t"><span class="chip strong">{j.chair}</span> <span class="job-when">{agoLabel(j.time)}</span></div>
              <div class="d clamp2">{j.request}</div>
              <div class="job-meta">
                <span>{t('office.toolCalls', { n: j.toolCount })}</span>
                <span>{secs(j.durationMs)}</span>
                {#if j.outcome === 'good'}<span class="ok"><Icon name="thumbsUp" size={12} /></span>{/if}
                {#if j.outcome === 'bad'}<span class="bad"><Icon name="thumbsDown" size={12} /></span>{/if}
              </div>
            </div>
            {#if j.sessionId}
              <button class="ctrl" onclick={() => openSource(j)}>{t('office.openSource')}</button>
            {/if}
          </div>
        {/each}
        {#if loaded && jobs.length === 0}
          <div class="set-row"><div class="set-txt"><div class="d">{t('office.noJobs')}</div></div></div>
        {/if}
      </div>
    </div>
  </div>
</div>
