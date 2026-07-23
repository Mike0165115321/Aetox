<script lang="ts">
  import {
    cockpit, newSession, openFolder, openProject,
    searchGlobalHistory, selectGlobalSession, deleteSession,
  } from './stores/cockpit.svelte'
  import type { Session } from './types'
  import { identity, loadIdentityFiles, openIdentityFile, saveIdentityFile, createIdentityFile, deleteIdentityFile, identityTemplates } from './identity.svelte'
  import { t, i18n, setLocale, localeNames, type Locale } from './i18n.svelte'

  let { onOpenSettings }: { onOpenSettings: () => void } = $props()

  let historyQuery = $state('')
  let historySearchTimer: ReturnType<typeof setTimeout> | undefined

  let profileName = $state(localStorage.getItem('profileName') ?? '')
  let profileOpen = $state(false)
  // svelte-ignore state_referenced_locally
  let nameDraft = $state(profileName)
  const avatarInitial = $derived((profileName.trim()[0] ?? 'A').toUpperCase())

  function saveName() {
    profileName = nameDraft.trim()
    localStorage.setItem('profileName', profileName)
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
  let collapsedProjects = $state<Record<string, boolean>>({}) // fold a project's session list

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

  let identityOpen = $state(true)
  let projectsOpen = $state(true)
  let historyOpen = $state(false)

  let identityLoadedOnce = false
  let newIdentityName = $state('')
  const identityDirty = $derived(identity.draft !== identity.saved)
  const missingTemplates = $derived(
    identity.loaded && identity.files ? identityTemplates.filter((tpl) => !(identity.files || []).some((f) => f.name === tpl.name)) : []
  )

  $effect(() => {
    if (identityLoadedOnce) return
    identityLoadedOnce = true
    loadIdentityFiles()
  })

  function addIdentityFile() {
    if (!newIdentityName.trim()) return
    createIdentityFile(newIdentityName)
    newIdentityName = ''
  }

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
  <div class="side-sections">
  <div class="side-panel" style={identityOpen ? 'flex:0 0 260px' : 'flex:0 0 auto'}>
    <button type="button" class="side-head" onclick={() => (identityOpen = !identityOpen)}>
      <span class="chev">{identityOpen ? '▾' : '▸'}</span>
      <span class="eyebrow">{t('sidebar.identity')}</span>
    </button>
    {#if identityOpen}
      <div class="identity-body">
        <div class="identity-files">
          {#each identity.files as f (f.name)}
            <div class="identity-file" class:active={identity.activeName === f.name}>
              <button type="button" class="identity-file-open" onclick={() => openIdentityFile(f.name)}>
                <span class="ic">📄</span>
                <span class="t">{f.name}</span>
              </button>
              <button type="button" class="identity-file-del" aria-label={t('settings.remove')} onclick={() => deleteIdentityFile(f.name)}>✕</button>
            </div>
          {/each}
          {#if identity.files.length === 0}
            <div class="empty">{t('sidebar.noIdentityFiles')}</div>
          {/if}
        </div>
        {#if missingTemplates.length > 0}
          <div class="identity-templates">
            {#each missingTemplates as tpl (tpl.name)}
              <button type="button" class="identity-template" onclick={() => createIdentityFile(tpl.name, tpl.content)}>
                ＋ {tpl.name}
              </button>
            {/each}
          </div>
        {/if}
        <div class="identity-newfile">
          <input
            class="identity-newfile-input" placeholder={t('sidebar.newIdentityFile')}
            bind:value={newIdentityName}
            onkeydown={(e) => e.key === 'Enter' && addIdentityFile()}
          />
          <button type="button" class="icobtn tiny" aria-label={t('sidebar.newIdentityFile')} onclick={addIdentityFile}>＋</button>
        </div>
        {#if identity.activeName}
          <textarea
            class="identity-input" placeholder={t('sidebar.identityPlaceholder')}
            bind:value={identity.draft}
          ></textarea>
          <button
            type="button" class="ctrl identity-save"
            disabled={!identityDirty || identity.saving}
            onclick={saveIdentityFile}
          >
            {identity.saving ? t('settings.saving') : t('settings.save')}
          </button>
        {/if}
      </div>
    {/if}
  </div>

  <div class="side-panel">
    <div class="side-head">
      <button type="button" class="side-head-toggle" onclick={() => (projectsOpen = !projectsOpen)}>
        <span class="chev">{projectsOpen ? '▾' : '▸'}</span>
        <span class="eyebrow">{t('sidebar.projects')}</span>
      </button>
      <button type="button" class="icobtn tiny tip-r" aria-label={t('topbar.openFolder')}
        data-tip={t('topbar.openFolder')} onclick={openFolder}>＋</button>
      <button type="button" class="icobtn tiny tip-r" aria-label={t('sidebar.newSession')}
        data-tip="{t('sidebar.newSession')} · Ctrl+N" onclick={newSession}>✎</button>
    </div>
    {#if projectsOpen}
      <div class="scroll">
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
    {/if}
  </div>

  <div class="side-panel">
    <div class="side-head">
      <button type="button" class="side-head-toggle" onclick={() => (historyOpen = !historyOpen)}>
        <span class="chev">{historyOpen ? '▾' : '▸'}</span>
        <span class="eyebrow">{t('sidebar.globalHistory')}</span>
      </button>
      <button type="button" class="icobtn tiny tip-r" aria-label={t('sidebar.newSession')}
        data-tip="{t('sidebar.newSession')} · Ctrl+N" onclick={newSession}>✎</button>
    </div>
    {#if historyOpen}
      <div class="scroll">
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
            {#if s.projectName}<span class="snip">{s.projectName}{#if s.snippet} — {s.snippet}{/if}</span>{/if}
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
