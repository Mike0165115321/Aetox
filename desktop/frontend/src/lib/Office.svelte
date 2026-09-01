<script lang="ts">
  // เอเจนเฉพาะทาง (COMPANY.md §4): the roster, and the work the team has taken in.
  //
  // Two lists and no state of its own. A chair is a profile file, and the jobs
  // feed is a query over `jobs` — the rows every delegation already writes. The
  // page was specified as "a roster plus a feed, no new state, no inbox", and
  // that is exactly as much as this does: it reads, and it lets you walk to the
  // conversation a job came from.
  //
  // It was split in two and moved behind the ทีม door for about an hour on
  // 2026-08-20 (§158) and the owner sent it straight back: the page is where you
  // walk in to talk to a specialist, and that belongs beside the assistant. The
  // name is the only thing that stayed changed.
  import { onMount } from 'svelte'
  // The hiring door opens the agents' home. Since the homes split, which
  // folder a file lands in is which kind it is — a chair file dropped into the
  // sub-agents' folder would wake up sick.
  import {
    ListChairs, ListReceivedJobs, OpenAgentsFolder, AgentGate,
    DelegateSwitches, SetDelegateOff,
  } from '../../wailsjs/go/main/App'
  import { main, subagent } from '../../wailsjs/go/models'
  import { agoLabel, cockpit, newChairSession, selectGlobalSession, setActiveView } from './stores/cockpit.svelte'
  import { t, type TKey } from './i18n.svelte'
  import { dayBucket } from './dayBucket'
  import Icon from './Icon.svelte'
  import AgentLock from './AgentLock.svelte'
  import AgentFace from './AgentFace.svelte'

  let { onClose }: { onClose: () => void } = $props()

  let chairs = $state<main.Chair[]>([])
  let jobs = $state<main.ReceivedJob[]>([])
  let loaded = $state(false)
  // Which teammate's work the feed is showing. Empty is everyone, and the
  // filter row is only drawn once more than one name is in the feed — a
  // one-choice filter is furniture.
  let who = $state('')
  // Whether each teammate can work, and why not. Answered in Go
  // (App.AgentGate) so this page, ห้องงานวิดีโอ and the chat menu all draw the
  // same verdict on the same agent.
  let gates = $state<Record<string, main.AgentGate>>({})
  // The roster waits for the verdicts rather than drawing ahead of them. A card
  // that appears usable and is veiled a moment later has already told the
  // reader something untrue; the empty moment is shorter than the wrong one.
  let gated = $state(false)

  async function loadNeeds(roster: main.Chair[]) {
    const answers = await Promise.all(roster.map((c) => AgentGate(c.name)))
    gates = Object.fromEntries(roster.map((c, i) => [c.name, answers[i]]))
    gated = true
  }

  // Whether the main assistant may hand each of these agents work. It used to
  // be answerable only from the settings page, which is why this roster — the
  // page a person actually opens to look at their team — could not say the one
  // thing that decides whether anyone gets used.
  //
  // Absent rather than fatal when the call fails: the roster's job is to show
  // who works here, and it can do that whole job without the switches. Then
  // `banded` is false and the deck is drawn as one group with no switch on any
  // card, which is exactly the page as it stood before today.
  let delegate = $state<main.DelegateSettings | null>(null)
  let delegateBusy = $state('')
  async function loadDelegate() {
    try {
      delegate = await DelegateSwitches()
    } catch {
      delegate = null
    }
  }

  // One agent's reach, looked up in the agents block alone. Helpers live in the
  // other block and never appear on this page.
  function reachOf(name: string): { on: boolean; off: boolean } | null {
    if (!delegate) return null
    const w = delegate.agents.workers.find((x) => x.name === name)
    return w ? { on: w.on, off: delegate.agents.off } : null
  }
  function reaches(c: main.Chair): boolean {
    const w = reachOf(c.name)
    return !!w && w.on && !w.off
  }

  async function toggleAll() {
    if (!delegate || delegateBusy) return
    delegateBusy = 'all'
    try {
      delegate = await SetDelegateOff('agents', delegate.agents.off === false)
    } finally {
      delegateBusy = ''
    }
  }

  // The split the page is drawn in. Only drawn as two groups when there is a
  // switch to answer with and both groups have somebody in them — a heading
  // over an empty deck is a label for nothing, and on a fresh install (where
  // delegation ships off) it would put every card under "ยังไม่ได้เปิด" with an
  // empty group above it.
  let onDuty = $derived(chairs.filter(reaches))
  let offDuty = $derived(chairs.filter((c) => !reaches(c)))
  let banded = $derived(!!delegate && onDuty.length > 0 && offDuty.length > 0)

  onMount(async () => {
    const [roster, feed] = await Promise.all([ListChairs(), ListReceivedJobs(30), loadDelegate()])
    chairs = roster
    jobs = feed
    await loadNeeds(roster)
    loaded = true
  })

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

  // A duration is only worth a slot when it says something. Every row printing
  // "0.0s" is six copies of "this was instant" competing with the line that
  // says what the job was.
  function secs(ms: number): string {
    return ms >= 100 ? `${(ms / 1000).toFixed(1)}s` : ''
  }

  // The face a job wears is its author's, resolved off the roster so one agent
  // cannot show two faces on one page. Only the PROP needs resolving: the person
  // is drawn from the name, so a job whose profile has since been deleted keeps
  // the same face and loses nothing but what they were holding.
  const icons = $derived(new Map(chairs.map((c) => [c.name, c.icon ?? ''])))
  function iconOf(name: string): string {
    return icons.get(name) ?? ''
  }

  // Who is in the feed, in the order the roster lists them — so the filter row
  // reads in the same order as the cards above it.
  const feedNames = $derived.by(() => {
    const seen = new Set(jobs.map((j) => j.chair))
    const ordered = chairs.map((c) => c.name).filter((n) => seen.has(n))
    for (const n of seen) if (!ordered.includes(n)) ordered.push(n)
    return ordered
  })

  // Grouped by calendar day, because a column of "2 วัน" on every row is a wall
  // rather than a list. The rows arrive newest-first from Go, so consecutive
  // runs are already the groups.
  const feedGroups = $derived.by(() => {
    const out: { key: TKey; items: main.ReceivedJob[] }[] = []
    for (const j of jobs) {
      if (who && j.chair !== who) continue
      const key = dayBucket(j.time)
      const last = out.at(-1)
      if (last && last.key === key) last.items.push(j)
      else out.push({ key, items: [j] })
    }
    return out
  })

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
  //
  // 'team' is เอเจน; 'agents' is the ซับเอเจน page next to it in Settings.
  // Sending 'agents' still opened the right editor — the handler forces
  // kind='agent' — so the header read ตั้งค่าเอเจน and everything about the
  // form was correct. What it got wrong was the page *underneath*: closing the
  // editor put the user on the ซับเอเจน roster, a page they had not asked
  // for and could not have reached from here, with no way back to the team
  // they came from. A back button that lands somewhere else is worse than no
  // back button.
  function configure(c: main.Chair) {
    cockpit.settingsIntent = { section: 'team', agent: c.name }
    setActiveView('settings')
  }
  function createAgent() {
    cockpit.settingsIntent = { section: 'team', createAgent: true }
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
      <!-- The hiring door is a control on the section, not a card in the grid.
           As a card it was a 180px dashed box holding the first slot, so the
           first thing the eye landed on was the space where nobody is — and it
           pushed a real teammate onto a row of their own. -->
      <div class="sec-head">
        <div class="eyebrow section-label">{t('office.roster')}</div>
        <!-- The one sentence this page exists to change, said as a sentence
             rather than left for the reader to infer from a row of switches.
             It counts live: flipping any card's switch moves a card between the
             two decks below and changes this number in the same frame. -->
        {#if delegate}
          <span class="ag-reach">
            {#if delegate.agents.off}
              {t('office.reachNone')}
            {:else}
              {t('office.reachSome', { n: onDuty.length, total: chairs.length })}
            {/if}
          </span>
          <label class="mswitch" title={t('office.delegateAll')}>
            <input
              type="checkbox" checked={!delegate.agents.off} disabled={delegateBusy !== ''}
              aria-label={t('office.delegateAll')} onchange={toggleAll}
            />
            <span></span>
          </label>
        {/if}
        <button class="ctrl" onclick={createAgent}><Icon name="plus" size={13} /> {t('office.newAgent')}</button>
      </div>
      <!-- A face, not an inventory. The tool chips were six per card and five
           of the six were the same on every card — the office ceiling hands
           everyone the same set, so the list said nothing about who anyone is
           while taking half the card to say it. What is left is what the card
           is for: who this is, what they make, and whether they have done any
           of it. The tools are one click away behind the gear, which is also
           the only place they can be changed. -->
      <!-- One card, drawn twice — once per band. A snippet rather than a copy
           because the two decks differ in nothing except which agents are in
           them, and a second copy is a second thing to keep true. -->
      {#snippet chairCard(c: main.Chair)}
          {@const locked = gates[c.name]?.blocked ?? false}
          <!-- No switch, and the card never cools (owner, 31 ส.ค.): *"มันเหมือน
               ไม่เปิดใช้งาน ทั้งที่มันก็แชทได้ปกติ"*.
               The switch and the drained card were both answering "may the MAIN
               assistant hand this one work", and both were read as "this agent
               is off" — a card that greys beside an off switch says disabled in
               two ways at once, and the face made it worse: a person with the
               colour pulled out of them reads as gone, not as undelegated.
               The band this card sits under already carries that answer, with a
               count and a line saying the chat still opens, so the state is on
               screen once instead of three times. Changing it is the gear,
               which is where the rest of this agent's settings already live. -->
          <div class="chair-card agc" class:locked>
            <div class="chair-body">
              <div class="chair-who">
                <AgentFace name={c.name} icon={c.icon} size={38} />
                <span class="chair-name">{c.name}</span>
              </div>
              <p class="chair-desc">{c.description}</p>
              <!-- What this agent has actually done, as a quiet line inside the
                   card rather than a column of the foot (owner, 30 ส.ค.). It is
                   a fact ABOUT the agent, like the sentence above it; the foot
                   is where the card's actions are, and a number sharing that
                   row was what kept the chat button down to an icon.
                   A chip since 31 ส.ค., beside the one badge that is worth a
                   slot. Only facts that DIFFER between agents are drawn here —
                   a badge every card carries is a badge that says nothing. -->
              <div class="chair-chips">
                {#if c.overrides}<span class="chip mine">{t('office.overrides')}</span>{/if}
                {#if c.jobs > 0}
                  <span class="chair-stat"><span class="n">{c.jobs}</span> {t('office.jobsDone')} · {agoLabel(c.lastUsed ?? '')}</span>
                {:else}
                  <span class="chair-stat idle">{t('office.neverUsed')}</span>
                {/if}
              </div>
            </div>
            <!-- The one thing this page is for: walking in and talking to a
                 specialist (COMPANY.md, the reason the roster sits behind the
                 storefront and not in another building). It was a 13px sparkles
                 icon until 30 ส.ค. — the smallest thing on the card, wearing a
                 mark that means "chat" to nobody. Reported as "ไม่ใช่ไอค่อนโง่ ๆ
                 แบบปัจจุบัน".
                 
                 It takes the whole row and the gear keeps its icon: a cog reads
                 as settings anywhere, and settings is the errand you run
                 occasionally rather than the reason you opened the page.
                 
                 Named with the agent, not "this agent", because that is what
                 walking in is — and the row cannot overflow, which the version
                 sharing a line with the job count could the first time somebody
                 hired an agent with a long name. -->
            <div class="chair-foot">
              <button class="chair-talk" onclick={() => talkTo(c)}>
                <Icon name="messageSquare" size={14} />
                <span class="t">{t('office.chatWith', { name: c.name })}</span>
              </button>
              <button class="icobtn tiny tip-l" aria-label={t('settings.agentConfigure')}
                data-tip={t('settings.agentConfigure')} onclick={() => configure(c)}>
                <Icon name="settings" size={13} />
              </button>
            </div>
            <AgentLock agent={c.name} label={c.name} gate={gates[c.name] ?? null}
              onInstalled={() => loadNeeds(chairs)} />
          </div>
      {/snippet}

      <!-- Split by the one thing this page can change, not by who wrote the
           file. Which band an agent sits in IS its delegation state, so no card
           needs a badge for it: flip a switch and the card moves between the
           two decks, and the sentence above counts differently.
           One deck when there is nothing to split on — no switches loaded, or
           every agent on the same side of the line. -->
      {#if banded}
        <div class="ag-band">
          <span class="lab">{t('office.bandOn')}</span><span class="n">{onDuty.length}</span>
          <span class="rule"></span>
        </div>
        <div class="office-grid">
          {#each gated ? onDuty : [] as c (c.name)}{@render chairCard(c)}{/each}
        </div>
        <div class="ag-band">
          <span class="lab">{t('office.bandOff')}</span><span class="n">{offDuty.length}</span>
          <span class="rule"></span>
          <span class="say">{t('office.bandOffNote')}</span>
        </div>
        <div class="office-grid">
          {#each gated ? offDuty : [] as c (c.name)}{@render chairCard(c)}{/each}
        </div>
      {:else}
        <div class="office-grid">
          {#each gated ? chairs : [] as c (c.name)}{@render chairCard(c)}{/each}
          {#if loaded && chairs.length === 0}
            <div class="chair-card empty"><div class="chair-body"><p class="chair-desc">{t('office.noChairs')}</p></div></div>
          {/if}
        </div>
      {/if}
      <p class="office-note">
        {t('office.hiringNote')}
        <button class="linklike" onclick={() => OpenAgentsFolder()}>{t('office.openAgentsFolder')}</button>
      </p>

      <div class="sec-head feed-head">
        <div class="eyebrow section-label">{t('office.feed')}</div>
        <!-- Filtering is the question this list is actually asked once more
             than one teammate has worked: "what has doc been doing?". Drawn
             only when the answer could differ from the whole list. -->
        {#if feedNames.length > 1}
          <div class="feed-filter">
            <button class="pill" class:on={who === ''} onclick={() => (who = '')}>{t('office.filterAll')}</button>
            {#each feedNames as n (n)}
              <button class="pill" class:on={who === n} onclick={() => (who = n)}>{n}</button>
            {/each}
          </div>
        {/if}
      </div>
      <!-- One row per job, one line each. The row is the door — a boxed button
           repeated down the right edge was the loudest thing on a page whose
           subject is the left-hand line it sat beside. -->
      {#each feedGroups as g (g.key)}
        <div class="feed-day">{t(g.key)}</div>
        <div class="settings-card feed-card">
          {#each g.items as j (j.id)}
            <button class="job-row" disabled={!j.sessionId}
              aria-label={t('office.openSource')} onclick={() => openSource(j)}>
              <!-- The same face as the card above it. The feed names who did
                   the work, so drawing them a second way here would make one
                   agent two people on one page. -->
              <AgentFace name={j.chair} icon={iconOf(j.chair)} size={22} />
              <!-- The line the caller wrote, not the arguments the tool call
                   carried. `request` is the machine's copy and stays available
                   on hover for anyone who wants it. -->
              <span class="job-brief" title={j.request}>{j.brief || j.request}</span>
              {#if j.outcome === 'good'}<span class="ok"><Icon name="thumbsUp" size={12} /></span>{/if}
              {#if j.outcome === 'bad'}<span class="bad"><Icon name="thumbsDown" size={12} /></span>{/if}
              <span class="job-meta">{t('office.toolCalls', { n: j.toolCount })}</span>
              {#if secs(j.durationMs)}<span class="job-meta">{secs(j.durationMs)}</span>{/if}
              {#if j.sessionId}
                <span class="job-go">{t('office.openSource')}</span>
                <Icon name="chevronRight" size={14} />
              {/if}
            </button>
          {/each}
        </div>
      {/each}
      {#if loaded && feedGroups.length === 0}
        <div class="settings-card feed-card">
          <div class="set-row"><div class="set-txt"><div class="d">{who ? t('office.noJobsFor', { name: who }) : t('office.noJobs')}</div></div></div>
        </div>
      {/if}

      <!-- Where the rest of them are. This page is the roster — who takes work
           and what they have done — and it is not every profile the engine
           runs: the assistant's own delegates never sit here. Saying so is what
           keeps two pages from reading as one list that disagrees with itself.
           It sits at the foot because it is a footnote: mid-page it was a wall
           of prose between the team and their work. -->
      <p class="office-note foot">{t('office.settingsNote')}</p>
    </div>
  </div>
</div>
