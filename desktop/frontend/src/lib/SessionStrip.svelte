<script lang="ts">
  // สรุปห้อง — what this conversation holds, behind a button in the window's
  // top-right corner.
  //
  // It was built onto the composer first, on the reasoning that everything else
  // which must not scroll away (BackgroundWork §105, the suggestion chips, the
  // scroll-back button) is mounted there. That is the right rule for things the
  // room pushes AT you; this is a thing you go and look at, and it belongs at
  // the corner, where a panel is opened rather than watched.
  //
  // The corner costs what a corner costs: the button says the same thing
  // whatever is inside, so nothing is learned until it is opened. The badge is
  // what buys some of that back — it appears only while a plan is unfinished,
  // so the corner is not silent about work that is running.
  //
  // Four sections, and all four are readings of something that already
  // exists. Nothing here is a second copy of a fact:
  //   แผน         — cockpit.todos, written by todo_write
  //   แหล่งที่มา   — SessionSources, read off tool_runs (desktop/sources.go)
  //   ไฟล์ที่สร้างหรือแก้   — SessionEdits, read off the same table (session_edits.go)
  //   repo        — GitChangedFiles + the project's branch
  // A section with nothing in it still draws its heading and says so in words.
  // In a panel somebody opened on purpose, "there is no plan" is the answer
  // they came for, and the heading teaches what will appear there later.
  //
  // แหล่งที่มา and ไฟล์ที่สร้างหรือแก้ are the two halves of one question and sit next
  // to each other on purpose: what this room read, then what it changed. The
  // repo section below them answers something else — the state of the working
  // tree, the user's own dirty files included — and is not a substitute for
  // either, which is what it was quietly being used as.
  import { cockpit, clearPlan } from './stores/cockpit.svelte'
  import { t } from './i18n.svelte'
  import { SessionSources, SessionSourceCount, SessionEdits, GitChangedFiles, CurrentSessionID } from '../../wailsjs/go/main/App'
  import { main } from '../../wailsjs/go/models'
  import { openFileTab, openUrlInWorkbench } from './stores/workbench.svelte'
  import Icon from './Icon.svelte'
  import type { IconName } from './icons'

  let open = $state(false)
  let sources = $state<main.Source[]>([])
  let sourceTotal = $state(0)
  let edits = $state<main.EditedFile[]>([])
  let editTotal = $state(0)
  let changed = $state<main.ChangedFile[]>([])
  let showAllSources = $state(false)
  let showAllEdits = $state(false)
  // Only while a re-read is in the air, so the button can say it heard the
  // click. The panel keeps showing the last answer meanwhile: blanking three
  // lists for the length of a disk read would make a refresh look like a wipe.
  let reloading = $state(false)

  const todos = $derived(cockpit.todos)
  const planDone = $derived(todos.filter((td) => td.status === 'completed').length)
  const hasPlan = $derived(todos.length > 0)

  // Rides on the button so the corner is not mute while work is running, and
  // clears once every row is struck through: a finished plan is a record, and a
  // badge that never clears is one nobody reads.
  //
  // What is LEFT, not "2/5". A 30px button has room for one number, and of the
  // two, the one worth putting there is the one that changes what you do next —
  // "1/3" and "2/4" describe different situations and the same amount of work
  // remaining. The full ratio is a click away, on rows you can actually read.
  const badge = $derived(hasPlan && planDone < todos.length ? String(todos.length - planDone) : '')

  // Read on open, not on a timer. Every call here touches the disk — three read
  // the store, one shells out to git — and a panel nobody has opened has no
  // business doing any of it on every turn.
  async function load() {
    const id = await CurrentSessionID()
    const [list, total, wrote, files] = await Promise.all([
      SessionSources(id), SessionSourceCount(id), SessionEdits(id), GitChangedFiles(),
    ])
    sources = list ?? []
    sourceTotal = total ?? 0
    edits = wrote?.files ?? []
    editTotal = wrote?.total ?? 0
    changed = files ?? []
  }

  // The way back to a fresh answer without closing the panel.
  //
  // Reading on open alone was right until the panel became something you leave
  // open while a turn runs: the lists then describe the moment it was opened,
  // and the section that says which files the agent just changed is exactly the
  // one nobody wants a stale copy of. A button rather than a subscription —
  // this is a panel you go and look at, and a list that reorders itself under
  // the cursor is worse than one you asked to be brought up to date.
  async function reload() {
    if (reloading) return
    reloading = true
    try {
      await load()
    } finally {
      reloading = false
    }
  }

  function toggle() {
    open = !open
    showAllSources = false
    showAllEdits = false
    if (open) void load()
  }

  // Six is what fits without the panel becoming a page. The rest are one click
  // away and the button says how many — a list that will not say how truncated
  // it is reads as complete, which is the one thing it must never do.
  const SHOWN = 6
  const visibleSources = $derived(showAllSources ? sources : sources.slice(0, SHOWN))
  const hiddenCount = $derived(sourceTotal - visibleSources.length)
  const visibleEdits = $derived(showAllEdits ? edits : edits.slice(0, SHOWN))
  const hiddenEdits = $derived(editTotal - visibleEdits.length)

  const todoIcon = (status: string): IconName =>
    status === 'completed' ? 'check' : status === 'in_progress' ? 'chevronRight' : 'circle'

  async function openSource(s: main.Source) {
    open = false
    if (s.kind === 'url') await openUrlInWorkbench(s.path)
    else await openFileTab(s.path, s.label)
  }

  async function openChanged(f: main.ChangedFile) {
    open = false
    await openFileTab(f.path, f.path.split(/[\\/]/).pop() ?? f.path)
  }

  async function openEdited(f: main.EditedFile) {
    open = false
    await openFileTab(f.path, f.label)
  }

  function closeOnOutsideClick(e: MouseEvent) {
    if (!(e.target as HTMLElement).closest('.summary')) open = false
  }
</script>

<svelte:window
  onclick={open ? closeOnOutsideClick : undefined}
  onkeydown={open ? (e) => e.key === 'Escape' && (open = false) : undefined}
/>

<div class="summary">
  <button
    type="button"
    class="icobtn tip-r"
    class:on={open}
    aria-haspopup="dialog"
    aria-expanded={open}
    aria-label={t('summary.toggle')}
    data-tip={t('summary.toggle')}
    onclick={toggle}
  >
    <Icon name="layoutList" size={15} />
    {#if badge}<span class="summary-badge">{badge}</span>{/if}
  </button>

  {#if open}
    <div class="summary-menu" role="dialog" aria-label={t('summary.toggle')}>
      <!-- The panel finally says its own name, and carries the one control that
           belongs to all of it rather than to a section: everything below is
           read on open, so everything below goes stale together, and one
           button is the honest number of buttons for that. -->
      <div class="summary-head">
        <span>{t('summary.toggle')}</span>
        <button
          type="button"
          class="summary-reload"
          class:spinning={reloading}
          aria-label={t('summary.reload')}
          title={t('summary.reload')}
          disabled={reloading}
          onclick={reload}
        ><Icon name="refreshCw" size={12} /></button>
      </div>

      <section class="summary-sec">
        <h3>
          {t('strip.plan')}
          <!-- The off switch. This list is written only by the model, and once
               it outlives its turn the only one who can know it has been
               abandoned is the person who abandoned it. Present only when there
               is something to put down. -->
          {#if hasPlan}
            <button type="button" class="summary-clear" title={t('summary.clearPlan')} onclick={clearPlan}>
              <Icon name="x" size={11} />
            </button>
          {/if}
        </h3>
        {#if hasPlan}
          {#each todos as td}
            <div class="todo-item {td.status}">
              <span class="mark"><Icon name={todoIcon(td.status)} size={12} /></span>
              <span class="t">{td.content}</span>
            </div>
          {/each}
        {:else}
          <p class="summary-none">{t('strip.planEmpty')}</p>
        {/if}
      </section>

      <section class="summary-sec">
        <h3>{t('summary.sources')}</h3>
        {#if visibleSources.length}
          {#each visibleSources as s (s.path)}
            <!-- title carries the whole path: the label is shortened on
                 purpose, so the full answer has to stay reachable. -->
            <button type="button" class="summary-row" title={s.path} onclick={() => openSource(s)}>
              <span class="ic"><Icon name={s.kind === 'url' ? 'globe' : 'fileText'} size={13} /></span>
              <span class="lbl">{s.label}</span>
              <!-- Only present when another row carries the same label, and
                   then on every row in the group. -->
              {#if s.dir}<span class="dir">{s.dir}</span>{/if}
            </button>
          {/each}
          {#if hiddenCount > 0}
            <button type="button" class="summary-more" onclick={() => (showAllSources = true)}>
              {t('summary.viewAll', { count: String(hiddenCount) })}
            </button>
          {/if}
        {:else}
          <p class="summary-none">{t('summary.sourcesEmpty')}</p>
        {/if}
      </section>

      <section class="summary-sec">
        <h3>{t('summary.edits')}</h3>
        {#if visibleEdits.length}
          {#each visibleEdits as f (f.path)}
            <!-- A file that is no longer there is still what this room did, so
                 the row stays — as a line rather than a button, because the one
                 thing worse than no row is a row that opens nothing. -->
            {#if f.gone}
              <div class="summary-line gone" title={f.path}>
                <span class="st {f.status}">{f.status}</span>
                <span class="lbl">{f.label}</span>
                {#if f.dir}<span class="dir">{f.dir}</span>{/if}
              </div>
            {:else}
              <button type="button" class="summary-row" title={f.path} onclick={() => openEdited(f)}>
                <span class="st {f.status}">{f.status}</span>
                <span class="lbl">{f.label}</span>
                {#if f.dir}<span class="dir">{f.dir}</span>{/if}
              </button>
            {/if}
          {/each}
          {#if hiddenEdits > 0}
            <button type="button" class="summary-more" onclick={() => (showAllEdits = true)}>
              {t('summary.viewAll', { count: String(hiddenEdits) })}
            </button>
          {/if}
        {:else}
          <p class="summary-none">{t('summary.editsEmpty')}</p>
        {/if}
      </section>

      <section class="summary-sec">
        <h3>{t('summary.repo')}</h3>
        {#if cockpit.project.focused}
          <div class="summary-line">
            <span class="ic"><Icon name="gitBranch" size={13} /></span>
            <span class="lbl">{cockpit.project.branch || t('summary.noBranch')}</span>
          </div>
          {#each changed as f (f.path)}
            <button type="button" class="summary-row" title={f.path} onclick={() => openChanged(f)}>
              <span class="st {f.status}">{f.status}</span>
              <span class="lbl">{f.path}</span>
            </button>
          {/each}
          {#if !changed.length}
            <p class="summary-none">{t('summary.noChanges')}</p>
          {/if}
        {:else}
          <p class="summary-none">{t('summary.noProject')}</p>
        {/if}
      </section>
    </div>
  {/if}
</div>
