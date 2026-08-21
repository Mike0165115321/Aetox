<script lang="ts">
  // The working tree, as a room (DECISIONS §161.4).
  //
  // The chat timeline answers "what did that call change" — hunk by hunk, under
  // the row that made it. This answers the other question, the one nothing in
  // the window held: "where does my repository stand right now". A turn is not
  // a session and a session is not an afternoon; by the third turn the second
  // question has no answer on screen at all.
  //
  // Nothing here computes a diff. `GitFileDiff` builds it with the same differ
  // the chat's fold-out uses (internal/skill/hunk.go) and CodeDiff draws it with
  // the same component, so a file's hunks look identical wherever you meet them.
  // Two renderers for one thing is how they drift apart.
  //
  // Rows are collapsed on arrival and fetched on expand. A working tree of forty
  // files is an ordinary state, and forty `git show` calls to draw a list nobody
  // has looked at yet is work done on the chance it is wanted.
  import { onMount } from 'svelte'
  import { GitWorkingTree, GitFileDiff } from '../../../wailsjs/go/main/App'
  import { main } from '../../../wailsjs/go/models'
  import { cockpit } from '../stores/cockpit.svelte'
  import { openFileTab } from '../stores/workbench.svelte'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'
  import CodeDiff from '../CodeDiff.svelte'

  let files = $state<main.GitFileChange[]>([])
  let loaded = $state(false)
  let open = $state<Record<string, boolean>>({})
  let diffs = $state<Record<string, string>>({})
  let loading = $state<Record<string, boolean>>({})

  const branch = $derived(cockpit.project.branch || '')
  const totals = $derived(files.reduce(
    (acc, f) => ({ added: acc.added + (f.added ?? 0), removed: acc.removed + (f.removed ?? 0) }),
    { added: 0, removed: 0 },
  ))

  async function refresh() {
    files = (await GitWorkingTree()) ?? []
    loaded = true
    // A file that stopped being changed while its diff was open would otherwise
    // keep drawing the hunks it no longer has.
    for (const path of Object.keys(diffs)) {
      if (!files.some((f) => f.path === path)) {
        delete diffs[path]
        delete open[path]
      }
    }
  }

  onMount(refresh)

  // Read again the moment a turn ends. The panel's whole job is to say where
  // the repository stands, and the thing that most often moves it is the agent
  // that just finished working — a list that still says "clean" while the chat
  // above it reports three edited files is worse than no list, because it is
  // confidently wrong. `awaitingReply` going false is the cheapest true signal
  // that something may have changed; the refresh button stays for everything
  // else that touches the tree (a commit in a terminal, an editor, the user).
  let wasWorking = false
  $effect(() => {
    const working = cockpit.awaitingReply
    if (wasWorking && !working) void refresh()
    wasWorking = working
  })

  async function toggle(path: string) {
    if (open[path]) {
      open[path] = false
      return
    }
    open[path] = true
    if (diffs[path] === undefined) {
      loading[path] = true
      try {
        diffs[path] = (await GitFileDiff(path)) ?? ''
      } finally {
        loading[path] = false
      }
    }
  }

  const name = (path: string) => path.split('/').pop() ?? path
  const dir = (path: string) => {
    const cut = path.lastIndexOf('/')
    return cut < 0 ? '' : path.slice(0, cut)
  }
</script>

<div class="gitpane">
  <div class="gp-head">
    <span class="gp-where">
      <Icon name="gitBranch" size={13} />
      {#if branch}<b>{branch}</b>{/if}
      <span class="gp-arrow">→</span>
      <span>{t('git.workingTree')}</span>
    </span>
    <span class="gp-right">
      {#if files.length}
        <span class="gp-totals"><span class="add">+{totals.added}</span><span class="del">-{totals.removed}</span></span>
      {/if}
      <button class="icobtn tiny" aria-label={t('git.refresh')} data-tip={t('git.refresh')} onclick={refresh}>
        <Icon name="loaderCircle" size={13} />
      </button>
    </span>
  </div>

  <!-- Said out loud rather than left to be noticed: this room is on the โค้ด
       desk and nowhere else, and a person who cannot find it elsewhere deserves
       to be told why instead of hunting for it. -->
  <div class="gp-note">{t('git.codeDeskOnly')}</div>

  {#if !loaded}
    <div class="gp-empty">{t('git.loading')}</div>
  {:else if files.length === 0}
    <div class="gp-empty">{t('git.clean')}</div>
  {:else}
    <div class="gp-list">
      {#each files as f (f.path)}
        <div class="gp-file">
          <button class="gp-row" aria-expanded={!!open[f.path]} onclick={() => toggle(f.path)}>
            <span class="gp-caret"><Icon name={open[f.path] ? 'chevronDown' : 'chevronRight'} size={12} /></span>
            <span class="gp-name">{name(f.path)}</span>
            {#if dir(f.path)}<span class="gp-dir">{dir(f.path)}</span>{/if}
            <span class="gp-stat">
              <span class="add">+{f.added ?? 0}</span><span class="del">-{f.removed ?? 0}</span>
            </span>
            <span class="gp-badge {f.status}">{f.status}</span>
          </button>
          {#if open[f.path]}
            <div class="gp-diff">
              {#if loading[f.path]}
                <div class="gp-empty small">{t('git.loading')}</div>
              {:else if diffs[f.path]}
                <CodeDiff diff={diffs[f.path]} />
                <button class="gp-open" onclick={() => openFileTab(f.path, name(f.path))}>
                  <Icon name="fileCode" size={12} /> {t('git.openFile')}
                </button>
              {:else}
                <div class="gp-empty small">{t('git.noDiff')}</div>
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
