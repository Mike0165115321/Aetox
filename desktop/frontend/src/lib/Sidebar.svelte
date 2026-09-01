<script lang="ts">
  import { onMount } from 'svelte'
  import {
    cockpit, newSession, openFolder, openProject, openDesk, setActiveView,
    searchGlobalHistory, selectGlobalSession, deleteSession, exportChat, importChat,
    sessionWorking,
    newChairSession, openSpace,
  } from './stores/cockpit.svelte'
  import type { Session, SpaceRow } from './types'
  import {
    UserName, SetUserName, ListModes, ProviderAccountFor,
    AccountStatus, AccountRefresh,
  } from '../../wailsjs/go/main/App'
  import ProviderAccount from './ProviderAccount.svelte'
  import { navFor, deskLabelKey, type NavEntry } from './desks'
  import { shell, shellHasChats } from './shell.svelte'
  import { t, i18n, setLocale, localeNames, type Locale, type TKey } from './i18n.svelte'
  import { dayBucket } from './dayBucket'
  import { theme, applyTheme, THEMES, type ThemeName } from './theme.svelte'
  import { isShortcut, shortcutLabel } from './shortcuts'
  import {
    updater, updatePct, checkNow, startDownload, restartToUpdate, loadCurrentVersion,
  } from './selfUpdate.svelte'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import Icon from './Icon.svelte'

  let { onOpenSettings }: { onOpenSettings: () => void } = $props()

  let historyQuery = $state('')
  let historySearchTimer: ReturnType<typeof setTimeout> | undefined

  // The name is read back from Aetox's preference file, not localStorage —
  // see config.ModelPreference.UserName for why it used to vanish.
  let profileName = $state('')
  let profileOpen = $state(false)

  // The provider in use, and nothing else. Listing the others answered a
  // question nobody asked and made the menu look like it was naming the wrong
  // engine — a balance sitting next to a provider you are not talking to is
  // worse than no balance at all. Every other account is a Settings question.
  let account = $state<any>(null)

  // Fetched when the menu opens rather than on a timer, and only for the one
  // provider shown: a number nobody is looking at is not worth a round trip.
  async function loadAccount() {
    try {
      account = await ProviderAccountFor(cockpit.model.provider)
    } catch {
      account = null
    }
  }

  // The Aetox account, which is not the provider account above and not the name
  // beside it either: that name is what the agent calls you and you typed it.
  let aetox = $state<{ configured: boolean; signed_in: boolean; display: string; user?: { email?: string } } | null>(null)

  // Reachable, as of the last time the menu was opened. Being unreachable is
  // not being signed out — the engine only drops a session when the server
  // says the grant itself is dead — so this changes a dot and nothing else.
  let aetoxOnline = $state(false)

  async function loadAetoxAccount() {
    // Disk first, and for two of the three outcomes that is the whole answer:
    // a build with no id server, or a session that does not exist, has nothing
    // to ask anybody about and must not open a socket to find that out.
    try {
      aetox = await AccountStatus()
    } catch {
      aetox = null
      return
    }
    if (!aetox?.configured || !aetox.signed_in) {
      aetoxOnline = false
      return
    }
    try {
      // Asks the server, which is what makes the dot mean anything.
      aetox = await AccountRefresh()
      aetoxOnline = true
    } catch {
      // Offline, or the server is down. What is on disk still stands: the
      // identity stays, only the dot goes out.
      aetoxOnline = false
    }
  }
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

  // ---------- which Aetox is this, and is there a newer one ----------
  // The version used to be answerable only from Settings -> About, a page most
  // people never open — so "a new Aetox is out" was news that reached only
  // whoever went looking for it. It belongs here for the same reason the
  // account and the language do: this menu is where the app says what it IS,
  // and which build you are running is part of that sentence.
  //
  // Nothing about the update is decided here. selfUpdate.svelte owns the answer
  // and both acts, internal/update decides which action a channel deserves, and
  // this row draws what they say.
  const updateBusy = $derived(updater.phase === 'downloading' || updater.phase === 'restarting')
  const newVersion = $derived(updater.staged || updater.status?.latest || '')

  // Asked once per run, the first time the menu is opened — not on every open,
  // and never on a timer of its own: the daily check (update_notify.go) is what
  // keeps this current, and this only covers the gap before its first answer
  // lands. A menu that opens saying nothing about a release that came out
  // yesterday is the whole problem this row exists to fix. The button re-asks
  // whenever the user wants it re-asked.
  function refreshUpdate() {
    loadCurrentVersion()
    if (updater.status || updater.checking || updater.checkError) return
    void checkNow()
  }

  // The button, which is a different question than the one above. Opening the
  // menu asks "has anybody checked yet"; pressing the button asks "check now",
  // and the answer to the second is never "somebody already did".
  //
  // Both were wired to refreshUpdate, so from the first answer onwards its
  // guard returned before reaching checkNow and the button did nothing at all
  // — for the whole run, silently, with the row still reading ตรวจอัปเดต. The
  // owner found it the only way it can be found: by giving up and walking to
  // Settings → About, whose button has always called checkNow directly (owner,
  // 26 ส.ค.). `disabled` already covers the in-flight case.
  function checkUpdateNow() {
    loadCurrentVersion()
    void checkNow()
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

  /** How this column was left, read back the next time it is drawn.
   *
   * Which projects are folded and which are pinned are facts about the window,
   * not about the work: they belong to this machine's view of the sidebar, so
   * they live where the rail width and the collapsed panels already live
   * (App.svelte). Guarded, because a browser with site data switched off throws
   * on the accessor itself rather than returning null — and a sidebar that
   * cannot be drawn is a worse answer than one that opens every folder. */
  function readFlags(key: string): Record<string, boolean> {
    try {
      const raw = localStorage.getItem(key)
      return raw ? (JSON.parse(raw) as Record<string, boolean>) : {}
    } catch {
      return {}
    }
  }
  function writeFlags(key: string, flags: Record<string, boolean>): void {
    try {
      // Only the true ones. A project unfolded and refolded a dozen times
      // would otherwise leave a dozen `false` entries behind forever, and the
      // list of projects a user has ever opened only grows.
      const on: Record<string, boolean> = {}
      for (const [k, v] of Object.entries(flags)) if (v) on[k] = true
      localStorage.setItem(key, JSON.stringify(on))
    } catch {
      // Nothing to do and nothing worth saying: the fold still works for this
      // run, it just will not be remembered.
    }
  }

  // Tracks what the user folded AWAY, so every project shows its recent chats
  // by default — a project row with nothing under it says nothing.
  //
  // Kept across restarts since 26 ส.ค. It was in-memory only, so a project
  // folded shut opened itself again on the next launch and the fold read as a
  // control that did not work.
  let collapsedProjects = $state<Record<string, boolean>>(readFlags('projectsCollapsed'))
  $effect(() => writeFlags('projectsCollapsed', collapsedProjects))

  // The projects the user wants at the top, in the order the list already has.
  // Pinning does not reorder within the pinned half: the point is "these ones
  // first", and a second ordering rule the user cannot see would make the list
  // move for reasons nobody asked for.
  let pinnedProjects = $state<Record<string, boolean>>(readFlags('projectsPinned'))
  $effect(() => writeFlags('projectsPinned', pinnedProjects))

  // The same thing one level down: the chats worth keeping at the top of the
  // assistant's history, which has no projects to group by and is the one list
  // that grows without limit.
  let pinnedChats = $state<Record<string, boolean>>(readFlags('chatsPinned'))
  $effect(() => writeFlags('chatsPinned', pinnedChats))

  // And the storefront's โปรเจกต์ (§84), which are a third list and need a
  // third map: the two above are keyed by a workshop project's `key` and by a
  // session id, and a space has neither — its name IS its key, because a space
  // exists because its folder exists (desktop/spaces.go).
  //
  // One map rather than folding spaces into pinnedProjects, even though both
  // are called "project" on screen. They are different things behind two
  // different doors (COMPANY.md §8), and a shared map would let a workshop
  // folder and a storefront project named the same thing pin each other.
  let pinnedSpaces = $state<Record<string, boolean>>(readFlags('spacesPinned'))
  $effect(() => writeFlags('spacesPinned', pinnedSpaces))

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
      pinned: !!pinnedProjects[p.key],
    }))
      // Pinned first, and stable within each half: the engine's order is
      // most-recently-opened, which is the order to keep for everything the
      // user has not spoken about.
      .sort((a, b) => Number(b.pinned) - Number(a.pinned))
  )


  // One list per door, and no switch between them (§86). Chats and projects
  // used to take turns in this column because one window had to serve both
  // kinds of work; the split settled which is which — the storefront keeps
  // conversations (it never focuses a project at all, §19), the workshop keeps
  // projects with their chats nested underneath. A tab offering the other
  // door's list is the mixing the split exists to end.
  const showProjects = $derived(shell.name === 'code')

  // Whether this door has a conversation column at all. ทีม does not: nothing
  // behind it opens a session, so the search box, the import button and the
  // history list would all be controls over an empty set — or worse, over
  // another door's list, since deskFilterFor has no answer for a door that
  // holds no chats (§158.3). The rooms row above stays; that is the door.
  const showChats = $derived(shellHasChats(shell.name))

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
    // A page is lit while it is open, and also while the conversation it sent
    // you into is the one on screen. งานวิดีโอ is the case that needed the second
    // half: it asks which of the two jobs this is, opens that agent's chat, and
    // closes — so without it the column went blank the moment the room did its
    // job, leaving somebody talking to `video` with no row saying where they
    // were standing.
    if (entry.kind === 'page') {
      if (cockpit.activeView === entry.id) return true
      return inChat && !!cockpit.chair && (entry.chairs ?? []).includes(cockpit.chair)
    }
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

  // Open a project and start work in it, from the + on its row.
  //
  // The same door clicking the project's name uses, deliberately: opening a
  // project already starts a fresh session on it (App.OpenProjectPath calls
  // startNewSession), so a second path that made its own would be a second
  // definition of what "open a project" means. What this adds is the view —
  // pressed from the projects page or settings, the click should land in the
  // chat it just made rather than leave the user where they were.
  function startChatIn(path: string): void {
    setActiveView('chat')
    void openProject(path)
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
  // Pinned chats, in the order the history already has them. They come out of
  // the day groups entirely rather than being highlighted inside them: the
  // point of pinning is not having to look for the row, and a row still filed
  // under เมื่อวาน is still a row you have to scroll to.
  const pinnedHistory = $derived(visibleHistory.filter((s) => pinnedChats[s.id]))

  const historyGroups = $derived.by(() => {
    const out: { key: TKey; items: Session[] }[] = []
    for (const s of visibleHistory) {
      if (pinnedChats[s.id]) continue
      const key = dayBucket(s.updatedAt)
      const last = out.at(-1)
      if (last && last.key === key) last.items.push(s)
      else out.push({ key, items: [s] })
    }
    return out
  })

  // How many โปรเจกต์ the rail draws before it stops and points at the page.
  // The rail's job is the ones you are actually working in; a person with
  // thirty projects must not lose the chat list under them.
  const SPACE_PREVIEW = 8

  // The projects the user pinned, and the rest — the same split the chats
  // above make, drawn from the same gesture, so ที่ปักหมุด is one heading over
  // both rather than two headings that mean the same word.
  const pinnedSpaces_ = $derived(cockpit.spaces.filter((p) => pinnedSpaces[p.name]))
  const looseSpaces = $derived(cockpit.spaces.filter((p) => !pinnedSpaces[p.name]))
  const shownSpaces = $derived(looseSpaces.slice(0, SPACE_PREVIEW))
</script>

<svelte:window
  onclick={profileOpen ? closeProfileOnOutsideClick : undefined}
  onkeydown={(e) => {
    if (isShortcut(e, 'newSession')) {
      e.preventDefault()
      newSession()
    }
  }}
/>

<!-- One row, two callers: the flat search results and the day-grouped list are
     the same rows in a different order, and a copy of this markup in each
     branch is a copy that drifts. -->
{#snippet sessionRow(s: Session, hideDesk: boolean)}
  <button
    type="button" class="sess-row"
    class:active={s.active}
    class:working={sessionWorking(s)}
    class:draft={s.draft}
    class:pinned={pinnedChats[s.id]}
    onclick={() => selectGlobalSession(s)}
  >
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
      {:else if !hideDesk && deskLabelKey(s.mode)}
        <!-- Not inside a โปรเจกต์. Every chat in one runs at the assistant's
             desk, so this chip could only ever read ผู้ช่วย, on every row, for
             ever — measured in the running app on 30 ส.ค., where both rows of
             a project's list said "ผู้ช่วย · 1 นาที" and "ผู้ช่วย · 50 นาที".
             A label that cannot vary is not a label, it is furniture, and it
             was taking the half of the line the time actually needs. -->
        <span class="sess-desk">{t(deskLabelKey(s.mode) as TKey)}</span>
      {/if}
      <span class="ago">{s.ago}</span>
      <span class="sess-acts">
        <!-- First of the three, because it is the only one that changes where
             the row is rather than what happens to it — and unlike its
             neighbours it stays visible once it is on, since the mark is the
             reason this row is at the top. -->
        <span class="sess-pin" class:on={pinnedChats[s.id]} role="button" tabindex="0"
          aria-label={pinnedChats[s.id] ? t('sidebar.unpinChat') : t('sidebar.pinChat')}
          aria-pressed={!!pinnedChats[s.id]}
          onclick={(e) => { e.stopPropagation(); pinnedChats[s.id] = !pinnedChats[s.id] }}
          onkeydown={(e) => e.key === 'Enter' && (e.stopPropagation(), (pinnedChats[s.id] = !pinnedChats[s.id]))}>
          <Icon name="pin" size={12} />
        </span>
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

<!-- One โปรเจกต์ in the rail (§84). Deliberately the same markup the workshop
     draws its projects with — .proj-group-row and friends — because it is the
     same row: a folder icon, the name, and a pin that stays lit once it is on.
     A second set of classes for a row that already exists is how two doors
     start looking like two apps. -->
{#snippet spaceRow(p: SpaceRow)}
  {@const here = cockpit.space === p.name}
  <div class="proj-group-row">
    <button type="button" class="proj-group-head" class:active={here} onclick={() => openSpace(p.name)}>
      <!-- folderOpen for the one you are standing in. The app's own way of
           saying "this is the one", already used on the workshop's rows, so
           the rail needs no accent bar or dot of its own to say it again. -->
      <span class="ic"><Icon name={here ? 'folderOpen' : 'folder'} size={14} /></span>
      <span class="t">{p.name}</span>
    </button>
    <button type="button" class="proj-group-pin tip-r" class:on={pinnedSpaces[p.name]}
      data-tip={pinnedSpaces[p.name] ? t('sidebar.unpinProject') : t('sidebar.pinProject')}
      aria-label={pinnedSpaces[p.name] ? t('sidebar.unpinProject') : t('sidebar.pinProject')}
      aria-pressed={!!pinnedSpaces[p.name]}
      onclick={() => (pinnedSpaces[p.name] = !pinnedSpaces[p.name])}>
      <Icon name="pin" size={13} />
    </button>
  </div>
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
  {#if showChats}
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
      data-tip="{t('sidebar.newSession')} · {shortcutLabel('newSession')}" onclick={newSession}
    ><Icon name="pencil" size={15} /></button>
  </div>
  {/if}

  {#if showChats}
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
          {#each visibleHistory as s (s.id)}{@render sessionRow(s, false)}{/each}
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
              <!-- Starting work in a project was reachable only by clicking its
                   name, which every other list in the app treats as "show me
                   this" rather than "start something here". The action gets its
                   own control, and the row stops depending on the user having
                   learned that the two are the same click. -->
              <!-- tip-r, not the default: this button sits hard against the
                   column's right edge, and a tooltip centred under it grows
                   straight into the edge the panel clips at (overflow on .side
                   and .side-sections), so half the label was cut off. Pinned
                   right, it opens leftward into the column. Same class every
                   other right-edge tooltip in this panel uses. -->
              <!-- Pin, beside the + and quieter than it: this one changes
                   where the row sits, which is worth doing once and then
                   forgetting about, while + is the thing you press every day.
                   A pinned project keeps the mark lit whether or not the mouse
                   is here, because that mark is the reason the row is at the
                   top and a row that moved for no visible reason is worse than
                   no pinning at all. -->
              <button type="button" class="proj-group-pin tip-r" class:on={g.pinned}
                data-tip={g.pinned ? t('sidebar.unpinProject') : t('sidebar.pinProject')}
                aria-label={g.pinned ? t('sidebar.unpinProject') : t('sidebar.pinProject')}
                aria-pressed={g.pinned}
                onclick={() => (pinnedProjects[g.project.key] = !pinnedProjects[g.project.key])}>
                <Icon name="pin" size={13} />
              </button>
              <button type="button" class="proj-group-new tip-r" data-tip={t('sidebar.newChatIn')}
                aria-label={t('sidebar.newChatIn')} onclick={() => startChatIn(g.project.path)}>
                <Icon name="plus" size={13} />
              </button>
            </div>
            {#if !collapsedProjects[g.project.key]}
              {#each expandedProjects[g.project.key] ? g.sessions : g.sessions.slice(0, PROJECT_GROUP_PREVIEW) as s (s.id)}
                <!-- The ring goes on this list too. It was on .sess-row only,
                     which is the flat history the storefront draws — so on the
                     workshop side, where every chat lives nested under its
                     project, a running turn had no mark anywhere in the column
                     (owner, 22 ส.ค.). Same fact, same light, same class name. -->
                <div class="proj-group-sess" class:active={s.active} class:working={sessionWorking(s)} class:draft={s.draft}>
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
      <!-- Three sections, one column (owner, 30 ส.ค.): what you pinned, your
           projects, then the chats. Measured before it was drawn — standing
           inside a project on a 900px-tall window, this column held 186px of
           list in the 537px it is given, and the projects were four clicks away
           behind a page you only ever passed through. The sections go in the
           351px that was already empty. -->
      <div class="scroll">
        {#if searching}
          <!-- A search is a question about every chat there is, so it answers
               with chats and nothing else. Sections here would be three
               headings over one flat set of hits. -->
          {#each visibleHistory as s (s.id)}{@render sessionRow(s, false)}{/each}
          {#if visibleHistory.length === 0}
            <div class="empty">{t('sidebar.noResults')}</div>
          {/if}
        {:else}
          <!-- ที่ปักหมุด — one heading over both kinds. Pinning a project and
               pinning a chat are the same gesture meaning the same thing, "keep
               this at the top", and two headings for one sentence is how a
               column stops being read. Absent entirely until something is
               pinned. -->
          {#if pinnedSpaces_.length || pinnedHistory.length}
            <div class="sess-day-head sect">{t('sidebar.pinned')}</div>
            {#each pinnedSpaces_ as p (p.name)}{@render spaceRow(p)}{/each}
            {#each pinnedHistory as s (s.id)}{@render sessionRow(s, false)}{/each}
          {/if}

          <!-- โปรเจกต์. The rail is the door now: clicking a name opens a fresh
               chat inside that project (openSpace), so the page is where you go
               to make and manage them, not where you go to talk. Going back to
               an earlier conversation is the list below. -->
          <div class="sess-day-head sect">{t('desk.projects')}</div>
          {#each shownSpaces as p (p.name)}{@render spaceRow(p)}{/each}
          {#if looseSpaces.length > shownSpaces.length}
            <button type="button" class="linkish space-all" onclick={() => setActiveView('projects')}>
              {t('projects.allProjects')}
            </button>
          {/if}
          <button type="button" class="space-new" onclick={() => setActiveView('projects')}>
            <span class="ic"><Icon name="plus" size={13} /></span>{t('projects.create')}
          </button>

          <!-- แชท. Inside a project this is that project's own chats (§90) —
               the general list drops them on purpose, so without this branch
               the chat you are in would be nowhere on the column it is drawn
               beside. The heading names the project rather than a day, because
               a list that changed under you has to say why. -->
          {#if inSpace}
            <div class="sess-day-head sect">{t('sidebar.chatsInSpace', { name: cockpit.space })}</div>
            {#each cockpit.spaceHistory as s (s.id)}{@render sessionRow(s, true)}{/each}
            {#if cockpit.spaceHistory.length === 0}
              <div class="sess-empty">{t('projects.noChats')}</div>
            {/if}
          {:else}
            {#each historyGroups as g (g.key)}
              <div class="sess-day-head">{t(g.key)}</div>
              {#each g.items as s (s.id)}{@render sessionRow(s, false)}{/each}
            {/each}
            {#if visibleHistory.length === 0}
              <div class="empty">{t('sidebar.noHistory')}</div>
            {/if}
          {/if}
        {/if}
      </div>
    {/if}
  </div>
  </div>
  {/if}

  <div class="side-footer-wrap">
    <button type="button" class="side-footer" onclick={() => { profileOpen = !profileOpen; if (profileOpen) { loadAccount(); loadAetoxAccount(); refreshUpdate() } }}>
      <span class="avatar">{avatarInitial}</span>
      <!-- The name you chose wins; the account name stands in when you never
           chose one, so a signed-in sidebar stops asking for something it
           already knows. -->
      <span class="label">{profileName || aetox?.display || t('sidebar.setYourName')}</span>
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
        {#if aetox?.signed_in}
          <div class="acct-menu">
            <div class="acct-menu-row">
              <span class="acct-menu-name">
                <span class="acct-dot" class:on={aetoxOnline}
                  title={aetoxOnline ? t('sidebar.accountOnline') : t('sidebar.accountOffline')}></span>
                {aetox.display}
              </span>
              {#if aetox.user?.email && aetox.user.email !== aetox.display}
                <span class="acct-menu-sub">{aetox.user.email}</span>
              {/if}
            </div>
          </div>
        {:else if aetox?.configured}
          <button class="plus-menu-item" onclick={() => { profileOpen = false; onOpenSettings() }}>
            <span class="ic"><Icon name="circleUser" size={14} /></span> {t('sidebar.accountSignIn')}
          </button>
        {/if}
        {#if account}
          <div class="menu-sep"></div>
          <div class="acct-menu">
            <div class="acct-menu-row">
              <span class="acct-menu-name">{account.provider}</span>
              <ProviderAccount {account} compact showBlank />
            </div>
          </div>
        {/if}
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
        <div class="menu-sep"></div>
        <!-- One row, one sentence: which Aetox this is, and the only thing
             worth knowing beside it. The second line appears only when there IS
             something to say — an up-to-date app has no news, and a row that
             reports "no news" every day teaches people to stop reading it. -->
        <div class="ver-menu">
          <div class="ver-row">
            <span class="ic"><Icon name="package" size={14} /></span>
            <span class="ver-name">Aetox {updater.current ? 'v' + updater.current : '—'}</span>
            <button
              class="ver-check" onclick={checkUpdateNow}
              disabled={updater.checking || updateBusy}
            >
              {updater.checking ? t('update.checking') : t('update.check')}
            </button>
          </div>

          {#if updateBusy}
            <!-- Percent, not megabytes: the card carries the bytes, and this
                 row has one line to spend. -->
            <div class="ver-news">
              <span class="ver-note">
                {updater.phase === 'restarting'
                  ? t('update.restarting')
                  : updatePct() >= 0
                    ? t('update.downloadingPct', { pct: String(updatePct()) })
                    : t('update.downloading', { version: newVersion })}
              </span>
            </div>
            <div
              class="ver-bar" class:indeterminate={updatePct() < 0}
              role="progressbar" aria-valuemin="0" aria-valuemax="100"
              aria-valuenow={updatePct() >= 0 ? updatePct() : undefined}
              aria-label={t('update.progress')}
            >
              <span class="ver-bar-fill" style={updatePct() >= 0 ? `width:${updatePct()}%` : ''}></span>
            </div>
          {:else if updater.phase === 'ready'}
            <!-- Already on disk. The restart is the user's to time, so this
                 offers it rather than taking it. -->
            <div class="ver-news">
              <span class="ver-new">{t('update.readyToRestart', { version: newVersion })}</span>
              <button class="ver-go" onclick={restartToUpdate}>{t('update.restartNow')}</button>
            </div>
          {:else if updater.error}
            <div class="ver-news">
              <span class="ver-note ver-warn">{t('update.failed')}</span>
              <button class="ver-go" onclick={startDownload}>{t('update.retry')}</button>
            </div>
          {:else if updater.status?.available}
            <!-- The three endings internal/update already decided between: one
                 click when this channel can finish the job, the package
                 manager's own command when it installed us, the release page
                 for everything else. -->
            <div class="ver-news">
              <span class="ver-new">{t('update.ready', { version: newVersion })}</span>
              {#if updater.status.canAuto}
                <button class="ver-go" onclick={startDownload}>{t('update.now')}</button>
              {:else if !updater.status.hint}
                <button class="ver-go" onclick={() => BrowserOpenURL(updater.status?.url ?? '')}>
                  {t('update.openRelease')}
                </button>
              {/if}
            </div>
            {#if !updater.status.canAuto && updater.status.hint}
              <code class="ver-cmd">{updater.status.hint}</code>
            {/if}
          {:else if updater.checkError}
            <!-- Offline, rate-limited, a proxy in the way. Muted and in one
                 line: a check that could not run is not a broken app. -->
            <div class="ver-news"><span class="ver-note">{t('update.checkFailed')}</span></div>
          {:else if updater.status?.disabled}
            <div class="ver-news">
              <span class="ver-note">{t('update.checkOff')}</span>
              {#if updater.status.url}
                <button class="ver-go" onclick={() => BrowserOpenURL(updater.status?.url ?? '')}>
                  {t('update.openRelease')}
                </button>
              {/if}
            </div>
          {:else if updater.status}
            <div class="ver-news"><span class="ver-note">{t('update.upToDate')}</span></div>
          {/if}
        </div>

        <div class="menu-sep"></div>
        <!-- Parked 2026-08-14, see MobileRemote.svelte: the entry point comes
             back when the phone surface has been designed, not before. -->
        <button class="plus-menu-item" onclick={() => { profileOpen = false; onOpenSettings() }}>
          <span class="ic"><Icon name="settings" size={14} /></span> {t('sidebar.settings')} <span class="kbd">{shortcutLabel('settings')}</span>
        </button>
      </div>
    {/if}
  </div>
</aside>
