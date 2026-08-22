<script lang="ts">
  // โปรเจกต์ (COMPANY.md §84, DECISIONS §90): chats grouped, with files that
  // ride into every session inside.
  //
  // Two levels, not two columns. The list answers "which project" and nothing
  // else, so it is cards a person scans; the detail answers "what is in this
  // one", which is the context folder and the chats. Putting both on screen at
  // once made the list a narrow sidebar — the wrong shape for the question
  // asked most often, which is finding the project you meant.
  //
  // Read live off the disk for the same reason ผลงาน is: a project exists
  // because its folder exists, and an index behind this page would start
  // disagreeing with the disk the first time the user moved one.
  //
  // There is no description on a card, deliberately. Aetox has nowhere to store
  // one — the folder is the whole record — and inventing a file convention to
  // hold a sentence would be a second source of truth for the sake of a subtitle.
  // The card carries what is already true: how many chats, how much context,
  // when it last changed.
  import { onMount } from 'svelte'
  import { Spaces, CreateSpace, OpenSpaceFolder, SessionsInSpace, AddSpaceContext, RemoveSpaceContext } from '../../wailsjs/go/main/App'
  import { main } from '../../wailsjs/go/models'
  import { agoLabel, newSpaceSession, selectGlobalSession, sessionWorking, setActiveView } from './stores/cockpit.svelte'
  import { t } from './i18n.svelte'
  import Icon from './Icon.svelte'
  import { coverHue } from './coverHue'

  let { onClose }: { onClose: () => void } = $props()

  let projects = $state<main.Space[]>([])
  let chats = $state<main.SessionMeta[]>([])
  let openName = $state('')
  let loaded = $state(false)
  let creating = $state(false)
  let draftName = $state('')
  let query = $state('')
  let sortBy = $state<'updated' | 'name'>('updated')
  let error = $state('')
  let busy = $state('')
  let confirmFile = $state('')

  const open = $derived(projects.find((p) => p.name === openName))
  const shown = $derived(
    projects
      .filter((p) => p.name.toLowerCase().includes(query.trim().toLowerCase()))
      // The backend hands them back newest first; sorting by name is the only
      // reordering this page does, so it is the only branch here.
      .sort((a, b) => (sortBy === 'name' ? a.name.localeCompare(b.name) : 0)),
  )

  async function refresh() {
    projects = (await Spaces()) ?? []
    loaded = true
    if (openName && !projects.some((p) => p.name === openName)) openName = ''
  }

  onMount(refresh)

  async function enter(name: string) {
    openName = name
    chats = (await SessionsInSpace(name)) ?? []
  }

  async function create() {
    const name = draftName.trim()
    if (!name) return
    error = ''
    busy = 'create'
    try {
      await CreateSpace(name)
      draftName = ''
      creating = false
      await refresh()
      await enter(name)
    } catch (err) {
      error = String(err)
    }
    busy = ''
  }

  // Files are copied in, so the project keeps working when the original is
  // moved (see AddSpaceContext). The list comes back from the disk that was
  // just changed rather than being guessed at here.
  async function addFiles(name: string) {
    error = ''
    busy = 'add'
    try {
      const files = await AddSpaceContext(name)
      applyContext(name, files ?? [])
    } catch (err) {
      error = String(err)
    }
    busy = ''
  }

  // Two-step, the same gesture ผลงาน uses: the first click arms the row, the
  // second one deletes. These are the user's own files.
  async function removeFile(name: string, file: string) {
    if (confirmFile !== file) {
      confirmFile = file
      return
    }
    confirmFile = ''
    error = ''
    try {
      applyContext(name, (await RemoveSpaceContext(name, file)) ?? [])
    } catch (err) {
      error = String(err)
    }
  }

  function applyContext(name: string, files: string[]) {
    projects = projects.map((p) => (p.name === name ? main.Space.createFrom({ ...p, contextFiles: files }) : p))
  }

  async function openFolder(name: string) {
    error = ''
    try {
      await OpenSpaceFolder(name)
    } catch (err) {
      error = String(err)
      await refresh()
    }
  }

  // Opening a chat inside a project is the point of the room, so it leaves the
  // room — through the store, never the binding directly. Calling
  // NewSessionInSpace here was the bug: the engine opened the new session while
  // the window kept showing the one already on screen, so the chat the click
  // had just created was unreachable and the project looked like it had
  // vanished. newSpaceSession is the door that moves both.
  async function startChat(name: string) {
    error = ''
    busy = 'chat'
    try {
      setActiveView('chat')
      await newSpaceSession(name)
      onClose()
    } catch (err) {
      error = String(err)
    }
    busy = ''
  }

  async function openChat(chat: main.SessionMeta) {
    onClose()
    setActiveView('chat')
    await selectGlobalSession({ id: chat.id, title: chat.title, ago: '' })
  }
</script>

<div class="page-shell">
  <!-- The title is the level you are on. Inside a project the room's own name
       and blurb would be the second and third heading in a row saying nothing
       about the project you opened — so the project takes the title, and the
       breadcrumb above it is both the trail and the way back. -->
  <header class="page-head">
    <button class="settings-back" onclick={onClose}><Icon name="arrowLeft" size={14} /> {t('settings.backToApp')}</button>
    <div class="page-title">
      {#if open}
        <nav class="proj-crumbs">
          <button onclick={() => (openName = '')}>{t('desk.projects')}</button>
          <span>/</span>
          <span class="proj-crumb-here">{open.name}</span>
        </nav>
        <h2>{open.name}</h2>
      {:else}
        <h2>{t('desk.projects')}</h2>
        <p>{t('projects.intro')}</p>
      {/if}
    </div>
  </header>

  <div class="page-body">
    <div class="settings-inner wide">
      {#if error}<div class="page-error">{error}</div>{/if}

      {#if open}
        <!-- One project: the work on the left, what the project carries on the
             right. The title and the trail live in the page header above. -->
        <div class="proj-detail">
          <section class="proj-work">
            <button class="proj-start" disabled={busy === 'chat'} onclick={() => startChat(open.name)}>
              <Icon name="sparkles" size={15} />
              <span>{t('projects.newChat')}</span>
            </button>

            <div class="settings-group-label eyebrow">{t('projects.chats')}</div>
            {#if chats.length === 0}
              <div class="proj-none">{t('projects.noChats')}</div>
            {:else}
              <ul class="proj-chats">
                {#each chats as chat (chat.id)}
                  <li>
                    <!-- Same ring the sidebar draws, on the same fact: this
                         chat still has a turn running in it. The page is a
                         list of the project's conversations, so it is one of
                         the places you walk back to in order to find out. -->
                    <button class:working={sessionWorking(chat)} onclick={() => openChat(chat)}>
                      <Icon name="messageSquare" size={13} />
                      <span class="proj-chat-title">{chat.title}</span>
                      <span class="proj-chat-ago">{agoLabel(chat.updatedAt)}</span>
                    </button>
                  </li>
                {/each}
              </ul>
            {/if}
          </section>

          <!-- The rail is one card, not three. The reference has Instructions
               and Scheduled beside Context; neither is ours to put here yet —
               recurring work is the ระบบออโตเมชั่น room (COMPANY.md §7, still
               ⏳), and a second door to a room that does not exist is a promise
               the app cannot keep. -->
          <aside class="proj-rail">
            <div class="proj-rail-card">
              <div class="proj-rail-head">
                <h4>{t('projects.context')}</h4>
                <button class="icobtn tiny" aria-label={t('projects.addFiles')} title={t('projects.addFiles')}
                  disabled={busy === 'add'} onclick={() => addFiles(open.name)}><Icon name="plus" size={14} /></button>
              </div>
              <p class="proj-hint">{t('projects.contextHint')}</p>

              {#if open.contextFiles.length === 0}
                <button class="proj-drop" disabled={busy === 'add'} onclick={() => addFiles(open.name)}>
                  <Icon name="fileText" size={18} />
                  <span>{t('projects.contextEmpty')}</span>
                </button>
              {:else}
                <ul class="proj-files">
                  {#each open.contextFiles as file (file)}
                    <li>
                      <Icon name="fileText" size={13} />
                      <span class="proj-file-name">{file}</span>
                      <button
                        class="proj-file-del" class:confirm={confirmFile === file}
                        aria-label={t('projects.removeFile')}
                        onclick={() => removeFile(open.name, file)}
                      >
                        {#if confirmFile === file}{t('sidebar.confirmDelete')}{:else}<Icon name="x" size={11} />{/if}
                      </button>
                    </li>
                  {/each}
                </ul>
              {/if}

              <button class="linkish proj-rail-foot" onclick={() => openFolder(open.name)}>
                {t('projects.openFolder')}
              </button>
            </div>
          </aside>
        </div>
      {:else}
        <!-- The list, in the same card system the preset gallery and the roster
             use (pp-*): one visual language across every shelf in the app. A
             project has no picture to ship, so its cover is a colour derived
             from its own name — the gallery reads as a gallery on the day it is
             created, with nothing in the installer. -->
        {#if projects.length > 0}
          <div class="proj-bar">
            <label class="proj-search">
              <Icon name="search" size={13} />
              <input bind:value={query} placeholder={t('projects.search')} />
            </label>
            <div class="proj-sort">
              <span>{t('projects.sortBy')}</span>
              <select class="ctrl" bind:value={sortBy}>
                <option value="updated">{t('projects.sortUpdated')}</option>
                <option value="name">{t('projects.sortName')}</option>
              </select>
            </div>
          </div>
        {/if}

        <div class="pp-grid proj-gallery">
          {#if creating}
            <!-- Creating is a card in the same slot the new-project card sits
                 in, with the cover already wearing the colour the name will
                 keep. The first thing a person makes here should look like the
                 thing they are making, not like a form about it. -->
            <form class="pp-card proj-draft" onsubmit={(e) => { e.preventDefault(); create() }}>
              <span class="pp-cover" style="--h:{coverHue(draftName || t('desk.projects'))}">
                <span class="pp-mono">{draftName || '—'}</span>
              </span>
              <div class="pp-body">
                <!-- svelte-ignore a11y_autofocus -->
                <input
                  class="ctrl" autofocus bind:value={draftName} placeholder={t('projects.namePlaceholder')}
                  onkeydown={(e) => { if (e.key === 'Escape') { creating = false; draftName = '' } }}
                />
                <span class="pp-desc">{t('projects.createHint')}</span>
                <div class="proj-draft-actions">
                  <button class="ctrl ctrl-primary" type="submit" disabled={busy === 'create' || !draftName.trim()}>
                    {busy === 'create' ? t('settings.saving') : t('projects.createShort')}
                  </button>
                  <button class="ctrl" type="button" onclick={() => { creating = false; draftName = '' }}>
                    {t('settings.cancel')}
                  </button>
                </div>
              </div>
            </form>
          {:else}
            <button class="pp-card pp-new" onclick={() => (creating = true)}>
              <span class="pp-plus">+</span>
              <span class="pp-newtxt">{t('projects.create')}</span>
            </button>
          {/if}

          {#each shown as p (p.name)}
            <button class="pp-card proj-card" onclick={() => enter(p.name)} title={p.path}>
              <span class="pp-cover" style="--h:{coverHue(p.name)}">
                <span class="pp-mono">{p.name}</span>
              </span>
              <div class="pp-body">
                <span class="pp-title">{p.name}</span>
                <span class="pp-desc">
                  {#if p.chats === 0 && p.contextFiles.length === 0}
                    {t('projects.cardEmpty')}
                  {:else}
                    {[
                      p.chats > 0 ? t('projects.chatCount', { n: p.chats }) : '',
                      p.contextFiles.length > 0 ? t('projects.fileCount', { n: p.contextFiles.length }) : '',
                    ].filter(Boolean).join(' · ')}
                  {/if}
                </span>
              </div>
              <span class="proj-card-foot">{agoLabel(p.updatedAt)}</span>
            </button>
          {/each}
        </div>

        <!-- Only ever about the search. The first cut showed this whenever the
             grid was empty, so a brand-new install read "no project matches that
             search" before anything had been searched for. -->
        {#if loaded && query.trim() !== '' && shown.length === 0}
          <div class="proj-none">{t('projects.noMatches')}</div>
        {/if}
        {#if loaded && projects.length === 0 && !creating}
          <p class="page-note proj-first">{t('projects.empty')}</p>
        {/if}
      {/if}
    </div>
  </div>
</div>
