<script lang="ts">
  // ผลงาน (COMPANY.md §2): every file Aetox has made, read live off the disk.
  //
  // There is no index table behind this and there deliberately never will be —
  // the folder is the half users move, rename and delete without telling us, so
  // an index would show files that are gone and hide files that are there.
  // Deleting a conversation leaves its work alone (§6.7); this page is the one
  // place a produced file is deleted, by the user, on purpose.
  import { onMount } from 'svelte'
  import { ListArtifacts, OpenArtifact, DeleteArtifact } from '../../wailsjs/go/main/App'
  import { main } from '../../wailsjs/go/models'
  import { agoLabel, selectGlobalSession, setActiveView } from './stores/cockpit.svelte'
  import { t } from './i18n.svelte'
  import Icon from './Icon.svelte'
  import type { IconName } from './icons'

  let { onClose }: { onClose: () => void } = $props()

  let files = $state<main.Artifact[]>([])
  let loaded = $state(false)
  let error = $state('')
  // Two-step delete, the same gesture the session list uses: the first click
  // arms the row, the second one does it. These are the user's files.
  let confirmPath = $state('')

  async function refresh() {
    files = await ListArtifacts()
    loaded = true
  }

  onMount(refresh)

  async function open(file: main.Artifact) {
    error = ''
    try {
      await OpenArtifact(file.path)
    } catch (err) {
      error = String(err)
      await refresh() // it was probably deleted underneath us — say so by redrawing
    }
  }

  async function remove(file: main.Artifact) {
    if (confirmPath !== file.path) {
      confirmPath = file.path
      return
    }
    confirmPath = ''
    error = ''
    try {
      await DeleteArtifact(file.path)
    } catch (err) {
      error = String(err)
    }
    await refresh()
  }

  async function openSource(file: main.Artifact) {
    if (!file.sessionId) return
    // The view moves first, the transcript follows — see Office.svelte.
    setActiveView('chat')
    await selectGlobalSession({ id: file.sessionId, title: '', ago: '' })
  }

  function size(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  }

  // The mark on a card, by what the file is. Deliberately coarse: what a person
  // is scanning for here is "the deck or the workbook", not a mime database.
  function markOf(name: string): IconName {
    const ext = name.split('.').pop()?.toLowerCase() ?? ''
    if (['docx', 'pdf', 'md', 'txt'].includes(ext)) return 'fileText'
    if (['xlsx', 'csv'].includes(ext)) return 'chartColumn'
    if (ext === 'pptx') return 'layoutList'
    if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'svg'].includes(ext)) return 'eye'
    return ext ? 'fileCode' : 'package'
  }
</script>

<div class="page-shell">
  <header class="page-head">
    <button class="settings-back" onclick={onClose}><Icon name="arrowLeft" size={14} /> {t('settings.backToApp')}</button>
    <div class="page-title">
      <h2>{t('desk.artifacts')}</h2>
      <p>{t('artifacts.intro')}</p>
    </div>
  </header>

  <div class="page-body">
    <div class="settings-inner wide">
      {#if error}<div class="page-error">{error}</div>{/if}
      {#if loaded && files.length === 0}
        <div class="page-empty">
          <Icon name="package" size={22} />
          <p>{t('artifacts.empty')}</p>
        </div>
      {/if}
      <div class="art-grid">
        {#each files as f (f.path)}
          <div class="art-card">
            <button class="art-open" onclick={() => open(f)} title={f.path}>
              <span class="art-mark"><Icon name={markOf(f.name)} size={18} /></span>
              <span class="art-name">{f.name}</span>
              <span class="art-meta">{size(f.size)} · {agoLabel(f.modified)}</span>
            </button>
            <div class="art-foot">
              {#if f.sessionId}
                <button class="linkish" onclick={() => openSource(f)}>{t('artifacts.fromChat')}</button>
              {:else}
                <span class="art-orphan">{t('artifacts.noChat')}</span>
              {/if}
              <button
                class="art-del" class:confirm={confirmPath === f.path}
                aria-label={t('artifacts.delete')}
                onclick={() => remove(f)}
              >
                {#if confirmPath === f.path}{t('sidebar.confirmDelete')}{:else}<Icon name="x" size={12} />{/if}
              </button>
            </div>
          </div>
        {/each}
      </div>
    </div>
  </div>
</div>
