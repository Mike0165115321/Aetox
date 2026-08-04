<script lang="ts">
  // What a browser tab shows before it has a URL.
  //
  // It is not the user's blank slate. This browser lives on the agent's desk
  // (ARCHITECTURE.md §80) and the agent opens pages in it itself, so the page
  // that fills the gap answers "where has Aetox been?" rather than inviting
  // browsing — the old copy said "เริ่มท่องเว็บ", which cast the panel as the
  // user's browser and left them to supply everything.
  //
  // The list has two sources and shows them as one. What the agent opened has
  // always been on disk and never once been read back — every browser_open is a
  // tool_runs row (RecentAgentPages). Everything else the browser navigated to,
  // typed or clicked, comes from recentVisits.
  //
  // Clicking a row sets this tab's url, which is exactly what typing in the
  // address bar does — the native window then covers this pane and this DOM
  // unmounts, so nothing here is persistent chrome.
  import { onMount } from 'svelte'
  import { cockpit, agoLabel } from '../stores/cockpit.svelte'
  import { labelForUrl, recentVisits, type WorkbenchTab } from '../stores/workbench.svelte'
  import { RecentAgentPages } from '../../../wailsjs/go/main/App'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'

  let { tab }: { tab: WorkbenchTab } = $props()

  type AgentPage = { url: string; title: string; time: string }

  /** A device preset letterboxes the pane — a 1280×800 preset in a 384px column
   *  is ~384×240, and a phone preset can go under 280px wide. The head fits
   *  there; a list does not, so the same component drops to the head alone. */
  const compact = $derived(!!tab.viewport)

  const VISIBLE = 6

  let pages = $state<AgentPage[]>([])
  let loaded = $state(false)
  let expanded = $state(false)

  const stamp = (t: string) => {
    const ms = Date.parse(t)
    return Number.isNaN(ms) ? 0 : ms
  }

  async function load() {
    let agent: AgentPage[] = []
    try {
      agent = (await RecentAgentPages(24)) as unknown as AgentPage[]
    } catch {
      // "Nothing to show you" is the honest content of a database that will not
      // open, too. An error card here is a string nobody would read.
      agent = []
    }
    // Two sources, one list: what the agent opened (durable, in tool_runs) and
    // every other navigation this browser completed (recentVisits). A page that
    // is in both — the agent opened it, so it also navigated — appears once,
    // carrying whichever record is newer.
    //
    // Compared as instants, never as strings: Go writes RFC3339 with a local
    // offset and the frontend writes an ISO string in UTC, so "…+07:00" and
    // "…Z" sort wrong against each other while naming the same moment.
    const seen = new Set<string>()
    pages = [...agent, ...recentVisits()]
      .sort((a, b) => stamp(b.time) - stamp(a.time))
      .filter((p) => (seen.has(p.url) ? false : (seen.add(p.url), true)))
      .slice(0, 24)
    loaded = true
  }

  onMount(load)

  // The one other moment a new row can exist is the end of a turn. No timer,
  // no refresh button, no refetch on every activation.
  let wasBusy = false
  $effect(() => {
    const busy = cockpit.awaitingReply
    if (wasBusy && !busy) load()
    wasBusy = busy
  })

  const rowTitle = (p: AgentPage) => p.title.trim() || labelForUrl(p.url)

  function rowSub(p: AgentPage): string {
    try {
      const u = new URL(p.url)
      if (u.protocol === 'file:') return decodeURIComponent(u.pathname).replace(/^\//, '')
      return u.hostname + (u.pathname === '/' ? '' : u.pathname)
    } catch {
      return p.url
    }
  }

  // The same two assignments the address bar makes (Workbench.svelte's
  // navigate). BrowserPane's effect picks the url up and opens the real window.
  function open(p: AgentPage) {
    tab.url = p.url
    tab.name = labelForUrl(p.url)
  }
</script>

<div class="bp-start" class:compact>
  <div class="bp-head">
    {#if compact}<span class="ic"><Icon name="globe" size={20} /></span>{/if}
    <div class="eyebrow">{t('browserPane.deskEyebrow')}</div>
    <div class="t">{t('browserPane.blankTab')}</div>
    <!-- One line that swaps in place, so a turn starting or ending never
         reflows the page under the pointer. -->
    <div class="d">{cockpit.awaitingReply ? t('browserPane.agentBusy') : t('browserPane.blankTabSub')}</div>
  </div>

  {#if !compact && loaded}
    <!-- Rendered only once the query has resolved: a one-frame "nothing found"
         that then fills in is worse than a one-frame gap. -->
    <div class="bp-sec">
      <div class="bp-sec-h">
        <!-- clock, not bot: the list stopped being only the agent's the moment
             the user's own navigations joined it. -->
        <span class="ic"><Icon name="clock" size={14} /></span>
        <span class="eyebrow">{t('browserPane.openedTitle')}</span>
      </div>
      <div class="panel">
        {#if pages.length === 0}
          <div class="bp-note">{t('browserPane.openedEmpty')}</div>
        {:else}
          {#each expanded ? pages : pages.slice(0, VISIBLE) as p (p.url)}
            <button type="button" class="bp-row" title={p.url} onclick={() => open(p)}>
              <span class="ic"><Icon name={p.url.startsWith('file:') ? 'fileText' : 'globe'} size={14} /></span>
              <span class="tw">
                <span class="t">{rowTitle(p)}</span>
                <span class="d">{rowSub(p)}</span>
              </span>
              <span class="ago">{agoLabel(p.time)}</span>
            </button>
          {/each}
          {#if !expanded && pages.length > VISIBLE}
            <!-- Expands this list in place. Never a link to another kind of tab:
                 a browser start page must not grow a button that leaves. -->
            <button type="button" class="bp-more" onclick={() => (expanded = true)}>
              {t('browserPane.showAllPages', { total: String(pages.length) })}
            </button>
          {/if}
        {/if}
      </div>
    </div>
  {/if}

  {#if !compact}
    <!-- Not a button: the address bar is a sibling outside this component, and
         a caption that focuses something is a control in disguise. It is the
         guaranteed next move, which is what makes the list above safe to be
         empty. -->
    <div class="bp-foot">{t('browserPane.typeAddress')}</div>
  {/if}
</div>

<style>
  /* This pane owns its own scroll. .native-host is the element BrowserPane
     hands to getBoundingClientRect() to place the native window, so letting
     .insp-slot scroll instead would offset that rect by scrollTop and open the
     OS window off-register. */
  .bp-start { height: 100%; overflow-y: auto; display: flex; flex-direction: column; gap: 16px; padding: 14px; }
  .bp-start.compact { justify-content: center; align-items: center; text-align: center; gap: 6px; padding: 0 16px; }

  .bp-head .ic { display: block; margin: 0 auto 8px; color: var(--text-muted); opacity: 0.6; }
  .bp-head .eyebrow { margin-bottom: 4px; }
  .bp-head .t { color: var(--text-primary); font-weight: 600; font-size: var(--fs-md); }
  .bp-head .d { color: var(--text-muted); font-size: var(--fs-xs); margin-top: 2px; line-height: 1.45; }

  .bp-sec-h { display: flex; align-items: center; gap: 7px; margin-bottom: 6px; min-width: 0; }
  .bp-sec-h .ic { color: var(--text-dim); display: flex; }
  .bp-sec-h .eyebrow { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  /* The global button reset already gives these font/colour/background/border/
     cursor, so a row declares only its own layout. */
  .bp-row { display: flex; align-items: center; gap: 8px; width: 100%; min-width: 0; padding: 7px 10px; }
  /* --border-default, never --border-subtle: the latter IS --surface-sunken on
     rose-pine, rose-pine-dawn and solarized-light, where it would vanish. */
  .bp-row + .bp-row { border-top: 1px solid var(--border-default); }
  .bp-row:hover { background: var(--surface-row-hover); }
  .bp-row:active { background: var(--surface-hover); }
  .bp-row .ic { flex: none; color: var(--text-muted); display: flex; }
  .bp-row .tw { flex: 1; min-width: 0; }
  .bp-row .t { display: block; color: var(--text-primary); font-size: var(--fs-md); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .bp-row .d { display: block; margin-top: 1px; color: var(--text-muted); font-size: var(--fs-xs); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .bp-row .ago { flex: none; color: var(--text-muted); font-size: var(--fs-xs); }

  .bp-note { padding: 10px 12px; color: var(--text-muted); font-size: var(--fs-xs); line-height: 1.5; }
  .bp-more { width: 100%; padding: 8px 10px; border-top: 1px solid var(--border-default); color: var(--text-muted); font-size: var(--fs-xs); text-align: center; }
  .bp-more:hover { background: var(--surface-row-hover); color: var(--text-primary); }

  .bp-foot { margin-top: auto; padding-top: 4px; color: var(--text-muted); font-size: var(--fs-xs); line-height: 1.5; }
</style>
