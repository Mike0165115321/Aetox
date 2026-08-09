<script lang="ts">
  import { onMount } from 'svelte'
  import {
    cockpit, newSession, openFolder, openProject, openDesk, setActiveView,
    searchGlobalHistory, selectGlobalSession, deleteSession, exportChat, importChat,
    newChairSession,
  } from './stores/cockpit.svelte'
  import type { Session } from './types'
  import { UserName, SetUserName, ListModes } from '../../wailsjs/go/main/App'
  import { navFor, deskLabelKey, type NavEntry } from './desks'
  import { shell } from './shell.svelte'
  import { t, i18n, setLocale, localeNames, type Locale, type TKey } from './i18n.svelte'
  import { dayBucket } from './dayBucket'
  import { theme, applyTheme, THEMES, type ThemeName } from './theme.svelte'
  import Icon from './Icon.svelte'

  let { onOpenSettings }: { onOpenSettings: () => void } = $props()

  let historyQuery = $state('')
  let historySearchTimer: ReturnType<typeof setTimeout> | undefined

  // The name is read back from Aetox's preference file, not localStorage —
  // see config.ModelPreference.UserName for why it used to vanish.
  let profileName = $state('')
  let profileOpen = $state(false)
  let nameDraft = $state('')
  const avatarInitial = $derived((profileName.trim()[0] ?? 'A').toUpperCase())

  onMount(async () => {
    try {
      profileName = await UserName()
      nameDraft = profileName
    } catch {
      /* backend not up yet — typing a name still saves it */
    }
  })

  function saveName() {
    profileName = nameDraft.trim()
    void SetUserName(profileName)
  }

  function closeProfileOnOutsideClick(e: MouseEvent) {
    if (!(e.target as HTMLElement).closest('.side-footer-wrap')) profileOpen = false
  }

  function focusOnMount(el: HTMLInputElement) {
    el.focus()
  }

  $effect(() => {
    if (profileOpen) nameDraft = profileName
  })

  // Claude-style grouped switcher: every known project with its recent chats
  // nested beneath (matched by projectName from the global history list).
  const PROJECT_GROUP_PREVIEW = 3
  let expandedProjects = $state<Record<string, boolean>>({})
  // Tracks what the user folded AWAY, so every project shows its recent chats
  // by default — a project row with nothing under it says nothing.
  let collapsedProjects = $state<Record<string, boolean>>({})

  // Two-step delete: first click arms ("ยืนยัน?"), second click deletes.
  let confirmDeleteId = $state('')
  function onDeleteSession(s: Session) {
    if (confirmDeleteId !== s.id) {
      confirmDeleteId = s.id
      return
    }
    confirmDeleteId = ''
    deleteSession(s)
  }

  // Two-step export, same shape as delete: the first click asks the one
  // question a format picker exists to ask — read it (MD) or move it (JSON).
  let exportChoiceId = $state('')
  function onExportSession(s: Session) {
    exportChoiceId = exportChoiceId === s.id ? '' : s.id
  }
  function pickExport(s: Session, format: 'markdown' | 'json') {
    exportChoiceId = ''
    void exportChat(s, format)
  }
  const projectGroups = $derived(
    (cockpit.projects || []).map((p) => ({
      project: p,
      sessions: (cockpit.history || []).filter((s) => s.projectName === p.name),
    }))
  )


  // One list per door, and no switch between them (§86). Chats and projects
  // used to take turns in this column because one window had to serve both
  // kinds of work; the split settled which is which — the storefront keeps
  // conversations (it never focuses a project at all, §19), the workshop keeps
  // projects with their chats nested underneath. A tab offering the other
  // door's list is the mixing the split exists to end.
  const showProjects = $derived(shell.name === 'code')

  // The rooms behind the door the window is showing (§86).
  const rooms = $derived(navFor(shell.name))

  // ---------- The five buttons (COMPANY.md §2) ----------
  // Each desk's own description, straight from its manifest, so a user who
  // edits a mode file sees the change on the button that opens it. Failing to
  // load is not worth an error: the built-in blurb is the fallback.
  let deskBlurbs = $state<Record<string, string>>({})
  onMount(async () => {
    try {
      for (const m of await ListModes()) deskBlurbs[m.name] = m.description
    } catch {
      /* engine not up yet — the built-in blurbs stand in */
    }
  })

  // Which room the window is standing in.
  //
  // A chat held inside a โปรเจกต์ runs at the assistant's desk — that is how it
  // gets its tools — so the desk rule alone lit up ผู้ช่วย and the room the chat
  // actually belongs to stayed dark. True of the engine, wrong on screen: the
  // question this row answers is "where am I", and the answer is the project.
  // The same reasoning already applies to a chair chat, which runs at the
  // office desk and is read back as ทีมเอเจน by its own page being open.
  function navActive(entry: NavEntry): boolean {
    const inChat = cockpit.activeView === 'chat'
    if (inChat && cockpit.space) return entry.id === 'projects'
    if (entry.kind === 'page') return cockpit.activeView === entry.id
    // A room whose conversation is with an agent is lit by who you are talking
    // to, not by the desk. A chair chat runs at the office desk — reading the
    // desk here would light ทีมเอเจน while the user is standing in ระบบออโตเมชั่น.
    if (entry.chair) return inChat && cockpit.chair === entry.chair
    if (entry.kind === 'desk') return inChat && cockpit.desk === entry.id && !cockpit.chair
    return false
  }

  function onNavClick(entry: NavEntry) {
    if (entry.kind === 'soon') return
    if (entry.kind === 'page') {
      setActiveView(entry.id)
      return
    }
    if (entry.chair) {
      // The same door the office roster's "แชทกับเอเจนนี้" uses. One way in, so
      // a chat opened from the nav and a chat opened from the roster are the
      // same chat with the same rules — not two paths that drift.
      setActiveView('chat')
      void newChairSession(entry.chair)
      return
    }
    void openDesk(entry.id)
  }

  // Every chat, always — this column is the chat history, not the chat history
  // of wherever you happen to be standing.
  //
  // It used to follow the desk you were at, on the reasoning that a desk's own
  // chats are the list you came back for. What that missed is who pays when it
  // is wrong: a session is only stamped with a desk if it was opened through a
  // desk button, so every conversation started by opening the app and typing
  // is held at no desk at all. Walking into a room then emptied the column,
  // and a link labelled "ดูทั้งหมด" is not an answer to a list that just lost
  // twenty rows — the row you wanted was on screen a moment ago.
  // cockpit.history is already this door's, scoped in SQL by the engine
  // (deskFilterFor / ListSessionsForDoor) rather than filtered here — so a long
  // run of the other door's sessions cannot eat this list's page.
  const visibleHistory = $derived(cockpit.history || [])

  function onHistorySearchInput() {
    clearTimeout(historySearchTimer)
    historySearchTimer = setTimeout(() => searchGlobalHistory(historyQuery), 200)
  }

  // ---------- Day headers ----------
  // The bucketing lives in ./dayBucket, shared with the office's job feed —
  // two lists that disagree about which day "เมื่อวาน" is would be one copy of
  // this arithmetic too many.

  // Search results rank by match, not by date, so grouping them would print
  // "วันนี้" three times down one list. A flat list is the honest shape there.
  const searching = $derived(historyQuery.trim().length > 0)
  // Inside a project chat this column belongs to that project — see the branch
  // that renders it. Searching is exempt: a search is a question about every
  // chat there is, and its results already say which project each one is in.
  const inSpace = $derived(!!cockpit.space && !searching)
  const historyGroups = $derived.by(() => {
    const out: { key: TKey; items: Session[] }[] = []
    for (const s of visibleHistory) {
      const key = dayBucket(s.updatedAt)
      const last = out.at(-1)
      if (last && last.key === key) last.items.push(s)
      else out.push({ key, items: [s] })
    }
    return out
  })
</script>

<svelte:window
  onclick={profileOpen ? closeProfileOnOutsideClick : undefined}
  onkeydown={(e) => {
    if (e.ctrlKey && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'n') {
      e.preventDefault()
      newSession()
    }
  }}
/>

<!-- One row, two callers: the flat search results and the day-grouped list are
     the same rows in a different order, and a copy of this markup in each
     branch is a copy that drifts. -->
{#snippet sessionRow(s: Session)}
  <button type="button" class="sess-row" class:active={s.active} onclick={() => selectGlobalSession(s)}>
    <!-- The title gets the line to itself. It used to share one line with the
         chip, the age and two hover-only buttons — all of them flex:none, so
         the only thing that could give way was the title, and in a 280px rail
         "ค้นไฟล์ทั้งเครื่องแล้วสรุป" arrived on screen as "ค้นไฟล์ทั้งเครื่..." with an
         inch of empty space to its right. A list whose rows cannot say what
         they are is not a history. -->
    <span class="sess-line">
      <span class="t">{s.title}</span>
      {#if s.active}<span class="dot green"></span>{/if}
    </span>
    <!-- Everything that describes the row rather than names it, on the second
         line where there is room to spare — and that spare room is where the
         hover actions live, so revealing them costs the title nothing. -->
    <span class="sess-meta">
      {#if s.space}
        <!-- Only a search result can carry this: the lists drop project chats,
             because they belong to the project's own list (§90). Saying which
             project is what keeps a searched-up row from reading as a chat that
             is in two places. -->
        <span class="sess-desk space">{s.space}</span>
      {:else if s.agent}
        <!-- A direct chat is labelled with *who*, which says more than
             where: every chair lives in the office, so the agent's
             name subsumes the desk chip below. -->
        <span class="sess-desk agent">{s.agent}</span>
      {:else if deskLabelKey(s.mode)}
        <span class="sess-desk">{t(deskLabelKey(s.mode) as TKey)}</span>
      {/if}
      <span class="ago">{s.ago}</span>
      <span class="sess-acts">
        <span class="sess-exp" class:armed={exportChoiceId === s.id} role="button" tabindex="0"
          aria-label={t('sidebar.exportSession')}
          onclick={(e) => { e.stopPropagation(); onExportSession(s) }}
          onkeydown={(e) => e.key === 'Enter' && (e.stopPropagation(), onExportSession(s))}>
          {#if exportChoiceId === s.id}
            <span class="fmt" role="button" tabindex="0"
              onclick={(e) => { e.stopPropagation(); pickExport(s, 'markdown') }}
              onkeydown={(e) => e.key === 'Enter' && (e.stopPropagation(), pickExport(s, 'markdown'))}>MD</span>
            <span class="fmt" role="button" tabindex="0"
              onclick={(e) => { e.stopPropagation(); pickExport(s, 'json') }}
              onkeydown={(e) => e.key === 'Enter' && (e.stopPropagation(), pickExport(s, 'json'))}>JSON</span>
          {:else}<Icon name="download" size={12} />{/if}
        </span>
        <span class="sess-del" class:confirm={confirmDeleteId === s.id} role="button" tabindex="0"
          aria-label={t('sidebar.deleteSession')}
          onclick={(e) => { e.stopPropagation(); onDeleteSession(s) }}
          onkeydown={(e) => e.key === 'Enter' && (e.stopPropagation(), onDeleteSession(s))}>
          {#if confirmDeleteId === s.id}{t('sidebar.confirmDelete')}{:else}<Icon name="x" size={12} />{/if}
        </span>
      </span>
    </span>
    <!-- Second line only while searching: the matched excerpt is why
         this row is in the results. The project name used to live here
         too, but the projects tab already groups by project — and for
         a no-focus chat it printed the raw project_key. -->
    {#if s.snippet}<span class="snip">{s.snippet}</span>{/if}
  </button>
{/snippet}

<aside class="side">
  <!-- The rooms behind this door (COMPANY.md §2). Some open a session, some
       are views over data, and โปรเจกต์ and ระบบออโตเมชั่น are rooms with nothing in
       them yet — shown rather than hidden, because the shape of the product is
       the thing being promised and a button that appears later reads as a new
       feature rather than a finished plan. Which rooms appear is the door's
       business (§86): the workshop draws none of the office's, and vice versa. -->
  <nav class="desk-nav" aria-label={t('desk.navLabel')}>
    {#each rooms as entry (entry.id)}
      <button
        type="button" class="desk-btn"
        class:active={navActive(entry)}
        class:soon={entry.kind === 'soon'}
        disabled={entry.kind === 'soon'}
        title={entry.kind === 'soon' ? t('desk.soon') : (deskBlurbs[entry.id] || t(entry.blurbKey))}
        onclick={() => onNavClick(entry)}
      >
        <span class="ic"><Icon name={entry.icon} size={15} /></span>
        <span class="t">{t(entry.labelKey)}</span>
        {#if entry.kind === 'soon'}<span class="soon-tag">{t('desk.soon')}</span>{/if}
      </button>
    {/each}
  </nav>

  <!-- The column's own two actions, above whichever list it is showing. They
       belong to the whole column, not to one list, so they sit outside the
       scroller with the rooms — a row that scrolls away is a row you go
       looking for. As icons rather than the two blocks they used to be: this
       is the top of a list, and the list is what the column is for. -->
  <div class="side-actions">
    <span class="side-search">
      <span class="ic"><Icon name="search" size={14} /></span>
      <input placeholder={t('sidebar.searchHistory')} aria-label={t('sidebar.searchHistory')}
        bind:value={historyQuery} oninput={onHistorySearchInput} />
    </span>
    <button
      type="button" class="icobtn tip-r" aria-label={t('sidebar.importSession')}
      data-tip={t('sidebar.importSession')} onclick={() => void importChat()}
    ><Icon name="upload" size={15} /></button>
    <button
      type="button" class="icobtn tip-r" aria-label={t('sidebar.newSession')}
      data-tip="{t('sidebar.newSession')} · Ctrl+N" onclick={newSession}
    ><Icon name="pencil" size={15} /></button>
  </div>

  <div class="side-sections">
  <div class="side-panel">
    {#if showProjects}
      <div class="scroll">
        <!-- No new-session button here: it is on the header row now, where it
             serves both lists instead of being repeated above each. Adding a
             project is this list's own action and stays. -->
        <button type="button" class="proj-add" onclick={openFolder}>
          <span class="ic"><Icon name="folder" size={14} /></span> {t('sidebar.addProject')}
        </button>
        <!-- Searching replaces the grouped list: a hit belongs to whatever
             project it belongs to, and re-nesting it under headings the user
             is not looking at buries the thing they searched for. The box was
             wired to the store here but had no renderer on this side, so
             typing in it did nothing at all in this window. -->
        {#if searching}
          {#each visibleHistory as s (s.id)}{@render sessionRow(s)}{/each}
          {#if visibleHistory.length === 0}
            <div class="sess-empty">{t('sidebar.noMatches')}</div>
          {/if}
        {:else}
        {#each projectGroups as g (g.project.key)}
          <div class="proj-group">
            <div class="proj-group-row">
              <button type="button" class="proj-group-chev" aria-label={g.project.name}
                onclick={() => (collapsedProjects[g.project.key] = !collapsedProjects[g.project.key])}>
                <Icon name={collapsedProjects[g.project.key] ? 'chevronRight' : 'chevronDown'} size={13} />
              </button>
              <button type="button" class="proj-group-head" class:active={g.project.active} onclick={() => openProject(g.project.path)}>
                <span class="ic"><Icon name={g.project.active ? 'folderOpen' : 'folder'} size={14} /></span>
                <span class="t">{g.project.name}</span>
                {#if g.project.active && cockpit.project.branch}<span class="proj-branch"><Icon name="gitBranch" size={11} /> {cockpit.project.branch}</span>{/if}
              </button>
            </div>
            {#if !collapsedProjects[g.project.key]}
              {#each expandedProjects[g.project.key] ? g.sessions : g.sessions.slice(0, PROJECT_GROUP_PREVIEW) as s (s.id)}
                <div class="proj-group-sess" class:active={s.active}>
                  <button type="button" class="proj-group-sess-open" onclick={() => selectGlobalSession(s)}>{s.title}</button>
                  <!-- Floated over the row's right end rather than sitting in
                       it: this row has only one line, and two invisible buttons
                       holding 50px open is 50px the chat's name never gets. -->
                  <span class="sess-acts float">
                    <button type="button" class="sess-exp" class:armed={exportChoiceId === s.id}
                      aria-label={t('sidebar.exportSession')} onclick={() => onExportSession(s)}>
                      {#if exportChoiceId === s.id}
                        <span class="fmt" role="button" tabindex="0"
                          onclick={(e) => { e.stopPropagation(); pickExport(s, 'markdown') }}
                          onkeydown={(e) => e.key === 'Enter' && (e.stopPropagation(), pickExport(s, 'markdown'))}>MD</span>
                        <span class="fmt" role="button" tabindex="0"
                          onclick={(e) => { e.stopPropagation(); pickExport(s, 'json') }}
                          onkeydown={(e) => e.key === 'Enter' && (e.stopPropagation(), pickExport(s, 'json'))}>JSON</span>
                      {:else}<Icon name="download" size={12} />{/if}
                    </button>
                    <button type="button" class="sess-del" class:confirm={confirmDeleteId === s.id}
                      aria-label={t('sidebar.deleteSession')} onclick={() => onDeleteSession(s)}>
                      {#if confirmDeleteId === s.id}{t('sidebar.confirmDelete')}{:else}<Icon name="x" size={12} />{/if}
                    </button>
                  </span>
                </div>
              {/each}
              {#if g.sessions.length > PROJECT_GROUP_PREVIEW}
                <button type="button" class="proj-group-more" onclick={() => (expandedProjects[g.project.key] = !expandedProjects[g.project.key])}>
                  {expandedProjects[g.project.key] ? t('sidebar.showLess') : t('sidebar.showMore')}
                </button>
              {/if}
            {/if}
          </div>
        {/each}
        {/if}
      </div>
    {:else}
      <div class="scroll">
        {#if searching}
          {#each visibleHistory as s (s.id)}{@render sessionRow(s)}{/each}
        {:else if inSpace}
          <!-- Standing inside a โปรเจกต์, this column is that project's chats
               (§90). The general list cannot show them — they were taken out of
               it on purpose — so leaving it up meant a column of unrelated
               conversations with the chat you were in nowhere on it. The header
               names the project, so a list that changed under you says why. -->
          <div class="sess-day-head space-head">
            <Icon name="folder" size={12} /> {cockpit.space}
          </div>
          {#each cockpit.spaceHistory as s (s.id)}{@render sessionRow(s)}{/each}
          {#if cockpit.spaceHistory.length === 0}
            <div class="sess-empty">{t('projects.noChats')}</div>
          {/if}
          <button type="button" class="linkish space-all" onclick={() => setActiveView('projects')}>
            {t('projects.allProjects')}
          </button>
        {:else}
          {#each historyGroups as g (g.key)}
            <div class="sess-day-head">{t(g.key)}</div>
            {#each g.items as s (s.id)}{@render sessionRow(s)}{/each}
          {/each}
        {/if}
        {#if visibleHistory.length === 0}
          <div class="empty">
            {#if historyQuery.trim()}{t('sidebar.noResults')}
            {:else}{t('sidebar.noHistory')}{/if}
          </div>
        {/if}
      </div>
    {/if}
  </div>
  </div>

  <div class="side-footer-wrap">
    <button type="button" class="side-footer" onclick={() => (profileOpen = !profileOpen)}>
      <span class="avatar">{avatarInitial}</span>
      <span class="label">{profileName || t('sidebar.setYourName')}</span>
      <!-- A mark on the way into settings when the agent is waiting to be
           allowed to remember something. Not a count and not a chip in the
           conversation: it is not work the user has to do now, but a queue
           they are never told about is one that never gets emptied — which
           would turn "nothing takes effect without you" into "nothing takes
           effect". -->
      <span class="ic gear" class:has-pending={cockpit.pendingLearned > 0}>
        <Icon name="settings" size={15} />
      </span>
    </button>
    {#if profileOpen}
      <div class="plus-menu profile-menu up">
        <div class="profile-head">
          <span class="avatar lg">{avatarInitial}</span>
          <input
            class="name-input" bind:value={nameDraft}
            placeholder={t('sidebar.setYourName')}
            use:focusOnMount
            onkeydown={(e) => e.key === 'Enter' && saveName()}
            onblur={saveName}
          />
        </div>
        <div class="menu-sep"></div>
        <div class="plus-menu-item">
          <span class="ic"><Icon name="palette" size={14} /></span> {t('settings.themeTitle')}
          <select class="lang-select" value={theme.name} onchange={(e) => applyTheme(e.currentTarget.value as ThemeName)}>
            {#each THEMES as th (th.value)}
              <option value={th.value}>{th.label}</option>
            {/each}
          </select>
        </div>
        <div class="plus-menu-item">
          <span class="ic"><Icon name="globe" size={14} /></span> {t('settings.languageTitle')}
          <select class="lang-select" value={i18n.locale} onchange={(e) => setLocale(e.currentTarget.value as Locale)}>
            {#each Object.entries(localeNames) as [code, name]}
              <option value={code}>{name}</option>
            {/each}
          </select>
        </div>
        <button class="plus-menu-item" onclick={() => { profileOpen = false; onOpenSettings() }}>
          <span class="ic"><Icon name="settings" size={14} /></span> {t('sidebar.settings')} <span class="kbd">{t('sidebar.settingsShortcut')}</span>
        </button>
      </div>
    {/if}
  </div>
</aside>
