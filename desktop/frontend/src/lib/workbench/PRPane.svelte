<script lang="ts">
  // Pull requests, as a room.
  //
  // The `pr` tool answers the model in sentences; this answers the person in
  // rows, from the same fetcher (internal/github, through desktop/pr_room.go).
  // What it adds that the tool cannot is seeing without asking: a CI result on
  // screen while you work is a different thing from one you have to remember to
  // go and request.
  //
  // Shaped after GitPane deliberately — same header, same collapsed rows, same
  // fetch-on-expand — because it is the same kind of question about the same
  // repository, and two rooms that answer alike should look alike.
  //
  // Nothing here computes a diff. GitHub hands back its own unified patch per
  // file and CodeDiff draws it, so a pull request's hunks look identical to the
  // ones under a chat row and to the ones in the Git pane.
  import { onMount } from 'svelte'
  import { PullRequests, PullRequestFiles, PullRequestChecks, CreatePullRequest } from '../../../wailsjs/go/main/App'
  import { main, github } from '../../../wailsjs/go/models'
  import { cockpit } from '../stores/cockpit.svelte'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'
  import CodeDiff from '../CodeDiff.svelte'

  let room = $state<main.PRRoom | null>(null)
  let loaded = $state(false)
  let open = $state<Record<number, boolean>>({})
  let files = $state<Record<number, github.PRFile[]>>({})
  let loading = $state<Record<number, boolean>>({})
  // A second fold, under the first. Opening a pull request answers "which
  // files", and that is the question the row was asked; every patch drawn at
  // the same time answers a question nobody asked yet and buries the list that
  // did. Keyed by number AND path, because two pull requests touching the same
  // file are two different rows.
  let openFile = $state<Record<string, boolean>>({})
  // Keyed by head SHA, not by number: the badge is about a commit, and a
  // pull request that gets pushed to is a different commit with the same number.
  let checks = $state<Record<string, github.CheckRun[]>>({})

  const items = $derived(room?.items ?? [])

  // Opening one from here. The form is inline rather than a dialog: it is part
  // of this room's work, and a modal over a list you are reading to decide what
  // to open is a modal in the way.
  let opening = $state(false)
  let form = $state({ title: '', head: '', base: '', body: '', draft: false })
  let submitting = $state(false)
  let formError = $state('')

  function startOpening() {
    // The branch you are standing on is the branch you almost always mean.
    form = { title: '', head: cockpit.project.branch || '', base: '', body: '', draft: false }
    formError = ''
    opening = true
  }

  async function submit() {
    if (!form.title.trim() || !form.head.trim() || submitting) return
    submitting = true
    formError = ''
    try {
      const created = await CreatePullRequest(form.title, form.head, form.base, form.body, form.draft)
      if (created.error) {
        // GitHub's own words. "No commits between main and feature" means push
        // first, and a form that said "failed" would have thrown that away.
        formError = created.error
        return
      }
      opening = false
      await refresh()
    } finally {
      submitting = false
    }
  }

  async function refresh() {
    room = await PullRequests()
    loaded = true
    // Badges after the list, never before it: the rows are the answer and the
    // checks are an ornament on them, so nothing waits for a CI lookup to draw.
    for (const pr of room?.items ?? []) void loadChecks(pr.headSHA)
  }

  async function loadChecks(sha: string) {
    if (!sha || checks[sha]) return
    checks[sha] = await PullRequestChecks(sha)
  }

  function fileKey(n: number, path: string): string {
    return n + ':' + path
  }

  function toggleFile(n: number, path: string) {
    const k = fileKey(n, path)
    openFile[k] = !openFile[k]
  }

  async function toggle(pr: github.PullRequest) {
    const n = pr.number
    open[n] = !open[n]
    if (!open[n] || files[n] || loading[n]) return
    loading[n] = true
    try {
      files[n] = await PullRequestFiles(n)
    } finally {
      loading[n] = false
    }
  }

  // One word for a commit's CI, or '' when nothing has reported. Failures win,
  // then anything still running: a reader needs to know whether to go and look,
  // and "12 passed" beside one failure is not that answer.
  function verdict(sha: string): 'fail' | 'running' | 'pass' | '' {
    const runs = checks[sha]
    if (!runs || runs.length === 0) return ''
    if (runs.some((r) => r.status === 'completed' && !passed(r))) return 'fail'
    if (runs.some((r) => r.status !== 'completed')) return 'running'
    return 'pass'
  }

  function passed(r: github.CheckRun): boolean {
    return r.conclusion === 'success' || r.conclusion === 'neutral' || r.conclusion === 'skipped'
  }

  function failedNames(sha: string): string {
    return (checks[sha] ?? [])
      .filter((r) => r.status === 'completed' && !passed(r))
      .map((r) => `${r.name} (${r.conclusion || 'failed'})`)
      .join('\n')
  }

  onMount(refresh)
</script>

<div class="pr-pane">
  <div class="pr-head">
    <span class="repo"><Icon name="gitBranch" size={13} /> {room?.repo || '—'}</span>
    {#if loaded && !room?.reason}
      <span class="count">{t('prPane.count', { count: String(items.length) })}</span>
    {/if}
    <button type="button" class="icobtn" title={t('prPane.refresh')} onclick={refresh}>
      <Icon name="refreshCw" size={13} />
    </button>
    {#if loaded && !room?.reason}
      <button type="button" class="icobtn" title={t('prPane.newTitle')} onclick={startOpening}>
        <Icon name="plus" size={13} />
      </button>
    {/if}
  </div>
  <!-- The room says why it is empty rather than leaving the absence to be
       discovered — the rule GitPane set for the desk that has no project. -->
  <p class="pr-scope">{t('prPane.scope')}</p>

  {#if opening}
    <div class="pr-form">
      <input class="pr-in" placeholder={t('prPane.newTitleField')} bind:value={form.title} />
      <div class="pr-branch-row">
        <input class="pr-in mono" placeholder={t('prPane.newHead')} bind:value={form.head} />
        <span class="arrow">→</span>
        <input class="pr-in mono" placeholder={t('prPane.newBase')} bind:value={form.base} />
      </div>
      <textarea class="pr-in pr-body" rows="4" placeholder={t('prPane.newBody')} bind:value={form.body}></textarea>
      <label class="pr-draft"><input type="checkbox" bind:checked={form.draft} /> {t('prPane.newDraft')}</label>
      {#if formError}<p class="pr-error">{formError}</p>{/if}
      <div class="pr-form-buttons">
        <button type="button" class="pr-cancel" onclick={() => (opening = false)}>{t('prPane.newCancel')}</button>
        <button
          type="button"
          class="pr-submit"
          disabled={submitting || !form.title.trim() || !form.head.trim()}
          onclick={submit}
        >{submitting ? t('prPane.newOpening') : t('prPane.newConfirm')}</button>
      </div>
      <!-- The one fact that costs a wasted attempt to learn, said before the
           attempt: GitHub compares branches it HAS, not the working tree. -->
      <p class="pr-hint">{t('prPane.newPushFirst')}</p>
    </div>
  {/if}

  {#if !loaded}
    <p class="pr-empty">{t('prPane.loading')}</p>
  {:else if room?.reason}
    <p class="pr-empty">{room.reason}</p>
    {#if !room.connected}
      <p class="pr-empty hint">{t('prPane.connect')}</p>
    {/if}
  {:else if items.length === 0}
    <p class="pr-empty">{t('prPane.none')}</p>
  {:else}
    <div class="pr-list">
      {#each items as pr (pr.number)}
        <div class="pr-row">
          <button type="button" class="pr-title" onclick={() => toggle(pr)}>
            <span class="chev"><Icon name={open[pr.number] ? 'chevronDown' : 'chevronRight'} size={12} /></span>
            <span class="num">#{pr.number}</span>
            <span class="ttl">{pr.title}</span>
            {#if pr.draft}<span class="tag">{t('prPane.draft')}</span>{/if}
            {#if verdict(pr.headSHA)}
              <span class="ci {verdict(pr.headSHA)}" title={failedNames(pr.headSHA)}>
                {verdict(pr.headSHA) === 'fail' ? '✗' : verdict(pr.headSHA) === 'running' ? '⋯' : '✓'}
              </span>
            {/if}
            <span class="stat"><span class="add">+{pr.additions}</span> <span class="del">-{pr.deletions}</span></span>
          </button>
          <div class="pr-branches">{pr.headRef} → {pr.baseRef}</div>
          {#if open[pr.number]}
            {#if loading[pr.number]}
              <p class="pr-empty">{t('prPane.loading')}</p>
            {:else}
              {#each files[pr.number] ?? [] as f (f.path)}
                <div class="pr-file">
                  <button
                    type="button" class="pr-file-head"
                    aria-expanded={!!openFile[fileKey(pr.number, f.path)]}
                    onclick={() => toggleFile(pr.number, f.path)}
                  >
                    <span class="chev"><Icon name={openFile[fileKey(pr.number, f.path)] ? 'chevronDown' : 'chevronRight'} size={12} /></span>
                    <span class="st">{f.status}</span>
                    <span class="path">{f.path}</span>
                    <span class="stat"><span class="add">+{f.additions}</span> <span class="del">-{f.deletions}</span></span>
                  </button>
                  {#if openFile[fileKey(pr.number, f.path)]}
                    <!-- No patch is not no change: GitHub omits it for a binary
                         and for anything it judged too large, and an empty box
                         would read as "this file is unchanged". -->
                    {#if f.patch}
                      <CodeDiff diff={f.patch} />
                    {:else}
                      <p class="pr-empty">{t('prPane.noPatch')}</p>
                    {/if}
                  {/if}
                </div>
              {/each}
            {/if}
            <a class="pr-link" href={pr.url} target="_blank" rel="noreferrer">{t('prPane.openOnGitHub')}</a>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
