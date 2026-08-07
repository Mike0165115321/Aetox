<script lang="ts">
  // ทีมเอเจน (COMPANY.md §4): the roster, and the work the team has taken in.
  //
  // Two lists and no state of its own. A chair is a profile file, and the jobs
  // feed is a query over `jobs` — the rows every delegation already writes. The
  // page was specified as "a roster plus a feed, no new state, no inbox", and
  // that is exactly as much as this does: it reads, and it lets you walk to the
  // conversation a job came from.
  import { onMount } from 'svelte'
  // The hiring door opens the agents' home. Since the homes split, which
  // folder a file lands in is which kind it is — a chair file dropped into the
  // sub-agents' folder would wake up sick.
  import { ListChairs, ListReceivedJobs, OpenAgentsFolder } from '../../wailsjs/go/main/App'
  import { main } from '../../wailsjs/go/models'
  import { agoLabel, cockpit, newChairSession, selectGlobalSession, setActiveView } from './stores/cockpit.svelte'
  import { t } from './i18n.svelte'
  import Icon from './Icon.svelte'
  import { coverHue } from './coverHue'
  import type { IconName } from './icons'

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

  // The two doors into the shared profile editor (Settings holds the one
  // implementation; two copies of an editor is how they drift). The intent
  // carries kind='agent' by construction — it comes off this roster — so the
  // editor saves through the agents' door without ever reading a file to
  // decide what something is.
  function configure(c: main.Chair) {
    cockpit.settingsIntent = { section: 'agents', agent: c.name }
    setActiveView('settings')
  }
  function createAgent() {
    cockpit.settingsIntent = { section: 'agents', createAgent: true }
    setActiveView('settings')
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
        <button class="pp-card pp-new" onclick={createAgent}>
          <span class="pp-plus">+</span>
          <span class="pp-newtxt">{t('office.newAgent')}</span>
        </button>
        <!-- A face, not an inventory. The tool chips were six per card and five
             of the six were the same on every card — the office ceiling hands
             everyone the same set, so the list said nothing about who anyone
             is while taking half the card to say it. What is left is what the
             card is for: who this is, what they make, and whether they have
             done any of it. The tools are still one click away, in the editor
             the gear opens, where changing them is also possible. -->
        {#each chairs as c (c.name)}
          <div class="pp-card chair-card">
            <span class="chair-cover" style="--h:{coverHue(c.name)}"></span>
            <span class="chair-face" style="--h:{coverHue(c.name)}">
              <Icon name={(c.icon || 'bot') as IconName} size={19} />
            </span>
            <div class="pp-body">
              <span class="pp-title">
                {c.name}
                {#if c.builtin}<span class="badge on">{t('settings.promptBuiltin')}</span>{/if}
                {#if c.overrides}<span class="badge">{t('office.overrides')}</span>{/if}
              </span>
              <span class="pp-desc">{c.description}</span>
            </div>
            <div class="chair-foot">
              {#if c.jobs > 0}
                <span class="stat"><span class="n">{c.jobs}</span> {t('office.jobsDone')} · {agoLabel(c.lastUsed ?? '')}</span>
              {:else}
                <span class="stat idle">{t('office.neverUsed')}</span>
              {/if}
              <button class="icobtn tiny tip-l" aria-label={t('settings.agentConfigure')}
                data-tip={t('settings.agentConfigure')} onclick={() => configure(c)}>
                <Icon name="settings" size={13} />
              </button>
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
      <p class="page-note">
        {t('office.hiringNote')}
        <button class="linklike" onclick={() => OpenAgentsFolder()}>{t('office.openAgentsFolder')}</button>
      </p>
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
              <!-- The line the caller wrote, not the arguments the tool call
                   carried. `request` is the machine's copy and stays available
                   on hover for anyone who wants it. -->
              <div class="d clamp2" title={j.request}>{j.brief || j.request}</div>
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
