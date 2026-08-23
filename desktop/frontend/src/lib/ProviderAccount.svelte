<script lang="ts">
  import { t } from './i18n.svelte'
  import Icon from './Icon.svelte'

  // One renderer for both places this appears — the provider card in Settings
  // and the profile menu — so the two can never disagree about what a number
  // means. `compact` drops the explanatory blanks: the menu is for a glance,
  // and a row saying "this provider cannot tell you" belongs on the page that
  // exists to explain, not in a menu.
  // showBlank forces the explanation even in compact form. The profile menu
  // sets it for the provider actually in use: that row has to say something,
  // because a name with nothing under it looks like a number that failed to
  // load rather than an account with no number to give.
  let { account, compact = false, showBlank = false }:
    { account: any; compact?: boolean; showBlank?: boolean } = $props()

  const balance = $derived(account?.balance ?? null)
  const quotas = $derived(account?.quotas ?? [])

  // Money is only ever shown when the provider actually sent a figure. A failed
  // fetch leaves hasAmount false, which is why the error text is a separate
  // branch rather than a zero.
  const hasMoney = $derived(!!balance?.hasAmount)

  const money = (amount: number, currency: string) => {
    const symbol = currency === 'USD' ? '$' : currency === 'CNY' ? '¥' : ''
    const figure = amount.toFixed(2)
    return symbol ? `${symbol}${figure}` : `${figure} ${currency}`
  }

  // Providers report windows from a minute to a week, so the unit comes from
  // the gap itself. A fixed "this week" would be wrong on most of them.
  const untilText = (iso: string) => {
    const ms = new Date(iso).getTime() - Date.now()
    if (!isFinite(ms) || ms <= 0) return ''
    const mins = Math.round(ms / 60000)
    if (mins < 60) return t('account.inMinutes', { n: String(mins) })
    const hours = Math.round(mins / 60)
    if (hours < 24) return t('account.inHours', { n: String(hours) })
    return t('account.inDays', { n: String(Math.round(hours / 24)) })
  }

  const clock = (iso: string) => {
    const d = new Date(iso)
    return isNaN(d.getTime()) ? '' : d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }

  // Below a fifth left the bar is a warning, not a readout.
  const barTone = (percent: number) => (percent <= 20 ? 'warn' : 'ok')

  // Part labels and window names arrive from Go as stable keys, so the locale
  // key is built rather than written out. The cast is the cost of that: the
  // key union cannot know a string assembled at runtime. A missing key falls
  // through to the key itself, which is visible in testing and harmless live.
  const tk = (key: string) => t(key as Parameters<typeof t>[0])
</script>

{#if balance}
  <div class="acct" class:acct-compact={compact} class:acct-danger={!balance.sufficient}>
    {#if hasMoney}
      <div class="acct-money">
        {#if !balance.sufficient}<Icon name="alertTriangle" size={15} />{/if}
        <span class="acct-figure">{money(balance.amount, balance.currency)}</span>
        {#if !balance.sufficient}
          <span class="acct-note">{t('account.insufficient')}</span>
        {:else if balance.parts?.length}
          <span class="acct-note">
            {balance.parts.map((p: any) => `${tk('account.part.' + p.label)} ${money(p.amount, balance.currency)}`).join(' · ')}
          </span>
        {/if}
      </div>
    {:else if balance.kind === 'free'}
      <div class="acct-plain"><Icon name="monitor" size={14} /> {t('account.free')}</div>
    {:else if !compact || showBlank}
      <!-- The blanks are different facts and must not look alike: a plan has
           no wallet, a web-only account has one we cannot read, and an error is
           a provider that should have answered and did not. -->
      <div class="acct-plain acct-muted">
        {#if account.error}
          {t('account.unreadable')}
        {:else if balance.kind === 'subscription'}
          <!-- Only when there is no window to show instead. A plan's number IS
               its quota, so "a subscription, not credit" beside a quota bar is
               a caption for something already on screen. -->
          {#if !account.expectsQuota}{t('account.subscription')}{/if}
        {:else}
          {t('account.webOnly')}
        {/if}
      </div>
    {/if}

    {#each quotas as q}
      <div class="acct-quota">
        <span class="acct-window">{tk('account.window.' + q.window)}</span>
        <span class="acct-bar"><span class="acct-fill {barTone(q.remainingPercent)}" style="width:{Math.round(q.remainingPercent)}%"></span></span>
        <span class="acct-pct">
          {t('account.remaining', { n: String(Math.round(q.remainingPercent)) })}{#if untilText(q.resetAt)} · {untilText(q.resetAt)}{/if}
        </span>
      </div>
    {/each}

    <!-- The one place the missing-quota sentence is written. It used to be here
         AND in the blank above, which printed it twice on the Codex card, and
         it guessed from the balance kind instead of asking whether this
         provider states a window at all. -->
    {#if quotas.length === 0 && account.expectsQuota && !account.quotaKnown}
      <div class="acct-plain acct-muted">{t('account.quotaNotYet')}</div>
    {/if}

    {#if !compact}
      {#if quotas.length}
        <!-- Never presented as live. Which sentence depends on where the
             numbers came from: a header dialect overhears them on a turn that
             happened, while OpenRouter and OpenCode Go are asked outright and
             can answer before any turn exists. -->
        <div class="acct-stamp">
          {account.quotaFetched
            ? t('account.quotaAsOf', { time: clock(quotas[0].observedAt) })
            : t('account.fromLastTurn', { time: clock(quotas[0].observedAt) })}
        </div>
      {/if}
      {#if hasMoney}
        <div class="acct-stamp">{t('account.fetchedAt', { time: clock(balance.fetchedAt) })}</div>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .acct { display: flex; flex-direction: column; gap: 6px; margin: 2px 0 10px; }
  .acct-compact { gap: 4px; margin: 0; }
  .acct-money { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
  .acct-figure { font-size: 20px; font-weight: 600; font-variant-numeric: tabular-nums; }
  .acct-compact .acct-figure { font-size: 14px; font-weight: 500; }
  /* --text-muted, not --text-dim, and no opacity on top of it.
     Measured on the default theme with the repo's own contrast helper:
     --text-dim on --surface-panel is 2.54:1, and .acct-muted multiplied that
     down to about 2.1 — against a WCAG AA floor of 4.5 for text this size.
     Reported as "ตรงนี้ดูยากมากครับ", which is what 2.1:1 looks like.
     The opacity is the half worth naming: a colour token can be measured and
     held to a number, and opacity composites against whatever happens to be
     behind it, so no token can promise a ratio through one. Two shades of
     unreadable are not worth keeping apart — both lines are --text-muted now
     (4.56:1 here), which is one fewer shade and all of it legible. */
  .acct-note, .acct-plain { font-size: 12px; color: var(--text-muted); }
  .acct-plain { display: flex; align-items: center; gap: 6px; }
  .acct-danger .acct-figure, .acct-danger .acct-note { color: var(--status-danger); }
  .acct-quota { display: flex; align-items: center; gap: 8px; }
  .acct-window { font-size: 12px; color: var(--text-dim); min-width: 74px; }
  .acct-compact .acct-window { min-width: 0; }
  .acct-bar { flex: 1; height: 6px; border-radius: 3px; background: var(--surface-sunken); overflow: hidden; }
  .acct-fill { display: block; height: 100%; }
  .acct-fill.ok { background: var(--status-success); }
  .acct-fill.warn { background: var(--status-warn); }
  .acct-pct { font-size: 12px; color: var(--text-dim); font-variant-numeric: tabular-nums; white-space: nowrap; }
  .acct-stamp { font-size: 11px; color: var(--text-dim); opacity: .75; }
</style>
