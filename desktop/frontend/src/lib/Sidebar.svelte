<script lang="ts">
  import {
    cockpit, selectSession, newSession, searchSessions, toggleNode, visibleTree, openFolder, openProject,
    refreshWorkspace, searchGlobalHistory, selectGlobalSession,
  } from './stores/cockpit.svelte'
  import { workbench, openFileTab } from './stores/workbench.svelte'
  import { identity, loadIdentityFiles, openIdentityFile, saveIdentityFile, createIdentityFile, deleteIdentityFile } from './identity.svelte'
  import { t, i18n, setLocale, localeNames, type Locale } from './i18n.svelte'

  let { onOpenSettings }: { onOpenSettings: () => void } = $props()

  let query = $state('')
  let searchTimer: ReturnType<typeof setTimeout> | undefined
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

  const rows = $derived(visibleTree(cockpit.tree))

  let identityOpen = $state(true)
  let projectsOpen = $state(true)
  let historyOpen = $state(false)

  let identityLoadedOnce = false
  let newIdentityName = $state('')
  const identityDirty = $derived(identity.draft !== identity.saved)

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

  function onSearchInput() {
    clearTimeout(searchTimer)
    searchTimer = setTimeout(() => searchSessions(query), 200)
  }

  function onHistorySearchInput() {
    clearTimeout(historySearchTimer)
    historySearchTimer = setTimeout(() => searchGlobalHistory(historyQuery), 200)
  }
</script>

<svelte:window onclick={profileOpen ? closeProfileOnOutsideClick : undefined} />

<aside class="side">
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

  <div class="side-panel grow">
    <button type="button" class="side-head" onclick={() => (projectsOpen = !projectsOpen)}>
      <span class="chev">{projectsOpen ? '▾' : '▸'}</span>
      <span class="eyebrow">{t('sidebar.projects')}</span>
    </button>
    {#if projectsOpen}
      <div class="scroll">
        <div class="proj-row">
          <span class="proj-name">{cockpit.project.name || t('topbar.openFolder')}</span>
          {#if cockpit.project.branch}<span class="proj-branch">⑂ {cockpit.project.branch}</span>{/if}
          <button type="button" class="icobtn tiny" aria-label={t('sidebar.refreshTip')} data-tip={t('sidebar.refreshTip')} onclick={refreshWorkspace}>⟳</button>
          <button type="button" class="icobtn tiny" aria-label={t('topbar.openFolder')} data-tip={t('topbar.openFolder')} onclick={openFolder}>⋯</button>
        </div>
        <div class="side-sub-head first">{t('sidebar.explorer')}</div>
        <div class="proj">
          {#each rows as node (node.label + node.depth)}
            <button
              type="button" class="row" class:active={workbench.activeId === 'file-' + node.path} title={node.path}
              style="padding-left:{20 + node.depth * 14}px"
              onclick={() => (node.kind === 'dir' ? toggleNode(node) : openFileTab(node.path))}
            >
              {#if node.kind === 'dir'}
                <span class="tw" class:open={node.open}></span>
              {/if}
              <span class="ic">{node.kind === 'dir' ? (node.open ? '📂' : '📁') : '📄'}</span> {node.label}
              {#if node.status === 'M'}<span class="st m">M</span>{/if}
              {#if node.status === 'U'}<span class="st u">U</span>{/if}
            </button>
          {/each}
          {#if cockpit.tree.length === 0}
            <div class="empty">{t('sidebar.noFiles')}</div>
          {/if}
        </div>

        <div class="side-sub-head">{t('sidebar.projectChats')}</div>
        <input class="sess-search" placeholder={t('sidebar.searchHistory')} bind:value={query} oninput={onSearchInput} />
        {#each cockpit.sessions as s (s.id)}
          <button type="button" class="sess-row" class:active={s.active} onclick={() => selectSession(s)}>
            <span class="sess-line">
              <span class="t">{s.title}</span>
              <span class="ago">{s.ago}</span>
              {#if s.active}<span class="dot green"></span>{/if}
            </span>
            {#if s.snippet}<span class="snip">{s.snippet}</span>{/if}
          </button>
        {/each}
        {#if cockpit.sessions.length === 0}
          <div class="empty">{query.trim() ? t('sidebar.noResults') : t('sidebar.noHistory')}</div>
        {/if}
        <button class="newbtn" onclick={newSession}>{t('sidebar.newSession')}</button>

        {#if cockpit.projects.length > 0}
          <div class="side-sub-head">{t('sidebar.switchProject')}</div>
          {#each cockpit.projects as p (p.key)}
            <button type="button" class="proj-card" class:active={p.active} onclick={() => openProject(p.path)}>
              <span class="proj-card-head">
                <span class="ic">📁</span>
                <span class="t">{p.name}</span>
              </span>
              {#if p.snippet}<span class="proj-card-sub">{p.snippet}</span>{/if}
            </button>
          {/each}
        {/if}
      </div>
    {/if}
  </div>

  <div class="side-panel">
    <button type="button" class="side-head" onclick={() => (historyOpen = !historyOpen)}>
      <span class="chev">{historyOpen ? '▾' : '▸'}</span>
      <span class="eyebrow">{t('sidebar.globalHistory')}</span>
    </button>
    {#if historyOpen}
      <div class="scroll capped">
        <input class="sess-search" placeholder={t('sidebar.searchHistory')} bind:value={historyQuery} oninput={onHistorySearchInput} />
        {#each cockpit.history as s (s.id)}
          <button type="button" class="sess-row" class:active={s.active} onclick={() => selectGlobalSession(s)}>
            <span class="sess-line">
              <span class="t">{s.title}</span>
              <span class="ago">{s.ago}</span>
              {#if s.active}<span class="dot green"></span>{/if}
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
