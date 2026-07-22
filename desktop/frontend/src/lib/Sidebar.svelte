<script lang="ts">
  import { cockpit, selectSession, newSession, searchSessions, toggleNode, visibleTree } from './stores/cockpit.svelte'
  import { workbench, openFileTab } from './stores/workbench.svelte'
  import { t, i18n, setLocale, localeNames, type Locale } from './i18n.svelte'

  let { onOpenSettings }: { onOpenSettings: () => void } = $props()

  let query = $state('')
  let searchTimer: ReturnType<typeof setTimeout> | undefined

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

  let filesOpen = $state(true)
  let chatOpen = $state(true)
  let filesHeight = $state(280)

  function onSearchInput() {
    clearTimeout(searchTimer)
    searchTimer = setTimeout(() => searchSessions(query), 200)
  }

  function startResize(e: PointerEvent) {
    e.preventDefault()
    const startY = e.clientY
    const startH = filesHeight
    function onMove(ev: PointerEvent) {
      filesHeight = Math.min(Math.max(startH + (ev.clientY - startY), 120), window.innerHeight - 220)
    }
    function onUp() {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }
</script>

<svelte:window onclick={profileOpen ? closeProfileOnOutsideClick : undefined} />

<aside class="side">
  <div class="side-panel" style={filesOpen ? `flex:0 0 ${filesHeight}px` : 'flex:0 0 auto'}>
    <button type="button" class="side-head" onclick={() => (filesOpen = !filesOpen)}>
      <span class="chev">{filesOpen ? '▾' : '▸'}</span>
      <span class="eyebrow">{t('sidebar.filesWorking')}</span>
    </button>
    {#if filesOpen}
      <div class="scroll">
        <div class="proj">
          {#each rows as node (node.label + node.depth)}
            <button
              type="button" class="row" class:active={workbench.activeId === 'file-' + node.path} title={node.path}
              style="padding-left:{6 + node.depth * 14}px"
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
      </div>
    {/if}
  </div>

  {#if filesOpen && chatOpen}
    <div class="side-resize" role="separator" aria-orientation="horizontal" onpointerdown={startResize}></div>
  {/if}

  <div class="side-panel grow">
    <button type="button" class="side-head" onclick={() => (chatOpen = !chatOpen)}>
      <span class="chev">{chatOpen ? '▾' : '▸'}</span>
      <span class="eyebrow">{t('sidebar.chatHistory')}{cockpit.project.name ? ` — ${cockpit.project.name}` : ''}</span>
    </button>
    {#if chatOpen}
      <div class="scroll">
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
