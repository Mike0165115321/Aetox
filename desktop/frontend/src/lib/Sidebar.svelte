<script lang="ts">
  import { onMount } from 'svelte'
  import {
    cockpit, newSession, openFolder, openProject,
    searchGlobalHistory, selectGlobalSession, deleteSession,
  } from './stores/cockpit.svelte'
  import type { Session } from './types'
  import { UserName, SetUserName } from '../../wailsjs/go/main/App'
  import { t, i18n, setLocale, localeNames, type Locale } from './i18n.svelte'
  import { theme, applyTheme, THEMES, type ThemeName } from './theme.svelte'

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
  const projectGroups = $derived(
    (cockpit.projects || []).map((p) => ({
      project: p,
      sessions: (cockpit.history || []).filter((s) => s.projectName === p.name),
    }))
  )

  // Chats and projects are two views of the same list, so they take turns in
  // one column instead of splitting it into stacked half-panels. Chats opens
  // first — it is the list you actually come back to.
  let tab = $state<'projects' | 'history'>('history')

  function onHistorySearchInput() {
    clearTimeout(historySearchTimer)
    historySearchTimer = setTimeout(() => searchGlobalHistory(historyQuery), 200)
  }
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

<aside class="side">
  <!-- Outside .side-sections so the switch stays put while the list scrolls. -->
  <div class="side-tabs">
    <div class="seg">
      <button type="button" class="seg-btn" class:active={tab === 'history'} onclick={() => (tab = 'history')}>
        <span class="ic">💬</span> {t('sidebar.globalHistory')}
      </button>
      <button type="button" class="seg-btn" class:active={tab === 'projects'} onclick={() => (tab = 'projects')}>
        <span class="ic">📁</span> {t('sidebar.projects')}
      </button>
    </div>
  </div>

  <div class="side-sections">
  <div class="side-panel">
    {#if tab === 'projects'}
      <div class="scroll">
        <button type="button" class="proj-add" onclick={openFolder}>
          <span class="ic">＋</span> {t('sidebar.addProject')}
        </button>
        {#each projectGroups as g (g.project.key)}
          <div class="proj-group">
            <div class="proj-group-row">
              <button type="button" class="proj-group-chev" aria-label={g.project.name}
                onclick={() => (collapsedProjects[g.project.key] = !collapsedProjects[g.project.key])}>
                {collapsedProjects[g.project.key] ? '▸' : '▾'}
              </button>
              <button type="button" class="proj-group-head" class:active={g.project.active} onclick={() => openProject(g.project.path)}>
                <span class="ic">{g.project.active ? '📂' : '📁'}</span>
                <span class="t">{g.project.name}</span>
                {#if g.project.active && cockpit.project.branch}<span class="proj-branch">⑂ {cockpit.project.branch}</span>{/if}
              </button>
            </div>
            {#if !collapsedProjects[g.project.key]}
              {#each expandedProjects[g.project.key] ? g.sessions : g.sessions.slice(0, PROJECT_GROUP_PREVIEW) as s (s.id)}
                <div class="proj-group-sess" class:active={s.active}>
                  <button type="button" class="proj-group-sess-open" onclick={() => selectGlobalSession(s)}>{s.title}</button>
                  <button type="button" class="sess-del" class:confirm={confirmDeleteId === s.id}
                    aria-label={t('sidebar.deleteSession')} onclick={() => onDeleteSession(s)}>
                    {confirmDeleteId === s.id ? t('sidebar.confirmDelete') : '✕'}
                  </button>
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
      </div>
    {:else}
      <div class="scroll">
        <button type="button" class="proj-add" onclick={newSession}>
          <span class="ic">＋</span> {t('sidebar.newSession')}
          <span class="kbd">Ctrl+N</span>
        </button>
        <input class="sess-search" placeholder={t('sidebar.searchHistory')} bind:value={historyQuery} oninput={onHistorySearchInput} />
        {#each cockpit.history as s (s.id)}
          <button type="button" class="sess-row" class:active={s.active} onclick={() => selectGlobalSession(s)}>
            <span class="sess-line">
              <span class="t">{s.title}</span>
              <span class="ago">{s.ago}</span>
              {#if s.active}<span class="dot green"></span>{/if}
              <span class="sess-del" class:confirm={confirmDeleteId === s.id} role="button" tabindex="0"
                aria-label={t('sidebar.deleteSession')}
                onclick={(e) => { e.stopPropagation(); onDeleteSession(s) }}
                onkeydown={(e) => e.key === 'Enter' && (e.stopPropagation(), onDeleteSession(s))}>
                {confirmDeleteId === s.id ? t('sidebar.confirmDelete') : '✕'}
              </span>
            </span>
            <!-- Second line only while searching: the matched excerpt is why
                 this row is in the results. The project name used to live here
                 too, but the projects tab already groups by project — and for
                 a no-focus chat it printed the raw project_key. -->
            {#if s.snippet}<span class="snip">{s.snippet}</span>{/if}
          </button>
        {/each}
        {#if cockpit.history.length === 0}
          <div class="empty">{historyQuery.trim() ? t('sidebar.noResults') : t('sidebar.noHistory')}</div>
        {/if}
      </div>
    {/if}
  </div>
  </div>

  <div class="side-footer-wrap">
    <button type="button" class="side-footer" onclick={() => (profileOpen = !profileOpen)}>
      <span class="avatar">{avatarInitial}</span>
      <span class="label">{profileName || t('sidebar.setYourName')}</span>
      <span class="ic gear">⚙</span>
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
          <span class="ic">🎨</span> {t('settings.themeTitle')}
          <select class="lang-select" value={theme.name} onchange={(e) => applyTheme(e.currentTarget.value as ThemeName)}>
            {#each THEMES as th (th.value)}
              <option value={th.value}>{th.label}</option>
            {/each}
          </select>
        </div>
        <div class="plus-menu-item">
          <span class="ic">🌐</span> {t('settings.languageTitle')}
          <select class="lang-select" value={i18n.locale} onchange={(e) => setLocale(e.currentTarget.value as Locale)}>
            {#each Object.entries(localeNames) as [code, name]}
              <option value={code}>{name}</option>
            {/each}
          </select>
        </div>
        <button class="plus-menu-item" onclick={() => { profileOpen = false; onOpenSettings() }}>
          <span class="ic">⚙</span> {t('sidebar.settings')} <span class="kbd">{t('sidebar.settingsShortcut')}</span>
        </button>
      </div>
    {/if}
  </div>
</aside>
