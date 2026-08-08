<script lang="ts">
  // One filterable list, two ways in (ARCHITECTURE.md §36):
  //   mode="prompts" — the "/" button, or typing "/" in an empty composer.
  //     Picking a row writes "/name " into the message. Nothing acts on the app.
  //   mode="all"     — the "+" button. Every row acts on the app instead.
  // Split by what Enter does, not by what the row is about: a list where Enter
  // sometimes types and sometimes executes is a list nobody trusts.
  //
  // Rows carry their current value on the right, so the palette doubles as the
  // status readout for settings that are otherwise scattered across three
  // places — and it is the only surface that tells the user a keyboard
  // shortcut exists at all.
  import { onMount } from 'svelte'
  import { t } from './i18n.svelte'
  import { ListPromptPresets, ToolCounts, SupportedThinkLevels, PickAttachmentImage } from '../../wailsjs/go/main/App'
  import { cockpit, newSession, switchApprovalMode, attachImageFromPath } from './stores/cockpit.svelte'

  let {
    mode = 'all',
    oninsert,
    onclose,
    onopenmodel,
    onswitchthink,
  }: {
    mode?: 'all' | 'prompts'
    oninsert: (text: string) => void
    onclose: () => void
    onopenmodel: () => void
    onswitchthink: (level: string) => void
  } = $props()

  type Row = {
    id: string
    group: string
    label: string
    hint?: string   // current value, or the keyboard shortcut
    badge?: string
    run?: () => void | Promise<void>  // no run = a read-only status row
  }

  let presets = $state<{ name: string; description: string; builtin: boolean }[]>([])
  // All four fields the Go ToolCounts carries. `workbench` was missing, so the
  // line rendered "NaN tools" for the frame between opening the palette and
  // ToolCounts() resolving — builtin + undefined.
  let counts = $state({ builtin: 0, workbench: 0, skill: 0, mcp: 0 })
  let thinkLevels = $state<string[]>([])
  let query = $state('')
  let cursor = $state(0)
  let inputEl = $state<HTMLInputElement | null>(null)

  onMount(async () => {
    inputEl?.focus()
    presets = await ListPromptPresets()
    if (mode === 'prompts') return
    counts = await ToolCounts()
    try {
      thinkLevels = await SupportedThinkLevels()
    } catch { /* backend not ready — the row just won't cycle */ }
  })

  const approvalLabels: Record<string, string> = {
    'ask': t('chat.approvalAsk'),
    'unsafe-only': t('chat.approvalUnsafeOnly'),
    'full-access': t('chat.approvalFullAccess'),
  }

  // Cycling beats a submenu here: the value is right there on the row, so one
  // click shows the next one. Three modes and a handful of think levels are
  // short enough that stepping through is faster than opening anything.
  function cycle<T>(list: T[], current: T): T {
    const i = list.indexOf(current)
    return list[(i + 1) % list.length] ?? current
  }

  const rows = $derived.by((): Row[] => {
    const out: Row[] = presets.map((p) => ({
      id: 'preset:' + p.name,
      group: t('palette.groupPrompts'),
      label: '/' + p.name,
      hint: p.description,
      badge: p.builtin ? t('settings.promptBuiltin') : undefined,
      run: () => oninsert('/' + p.name + ' '),
    }))
    if (mode === 'prompts') return out

    out.push(
      {
        id: 'attach', group: t('palette.groupContext'), label: t('chat.attachImage'),
        run: async () => {
          const path = await PickAttachmentImage()
          if (path) await attachImageFromPath(path)
        },
      },
      { id: 'new', group: t('palette.groupContext'), label: t('palette.newChat'), hint: 'Ctrl+N', run: () => newSession() },
      // Both halves, not just the name: the composer chip now carries the
      // provider as a mark and the model name not at all, so this row is where
      // "which brain, exactly" is read as text rather than hovered for.
      {
        id: 'model', group: t('palette.groupModel'), label: t('palette.switchModel'),
        hint: cockpit.model.modelName
          ? cockpit.model.provider + ' · ' + cockpit.model.modelName
          : cockpit.model.provider,
        run: onopenmodel,
      },
      {
        id: 'approval', group: t('palette.groupModel'), label: t('palette.approval'),
        hint: approvalLabels[cockpit.model.approval] ?? cockpit.model.approval,
        run: () => switchApprovalMode(cycle(['ask', 'unsafe-only', 'full-access'], cockpit.model.approval)),
      },
    )
    if (thinkLevels.length > 1) {
      out.push({
        id: 'think', group: t('palette.groupModel'), label: t('chat.thinkLevel'),
        hint: cockpit.model.thinkLevel,
        run: () => onswitchthink(cycle(thinkLevels, cockpit.model.thinkLevel)),
      })
    }
    out.push(
      {
        id: 'tools', group: t('palette.groupTools'),
        // builtin + workbench are both tools Aetox ships; the split matters in
        // Settings, not in a one-line readout.
        label: t('palette.toolsLine', {
          tools: String(counts.builtin + counts.workbench),
          skill: String(counts.skill),
          mcp: String(counts.mcp),
        }),
        hint: t('palette.toolsHint'),
      },
      { id: 'sc-settings', group: t('palette.groupKeys'), label: t('palette.openSettings'), hint: 'Ctrl+,' },
      { id: 'sc-panels', group: t('palette.groupKeys'), label: t('palette.togglePanels'), hint: 'Ctrl+Alt+S / B' },
      { id: 'sc-wb', group: t('palette.groupKeys'), label: t('palette.workbenchTabs'), hint: 'Ctrl+T · Ctrl+Shift+G' },
    )
    return out
  })

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase()
    if (!q) return rows
    return rows.filter((r) => (r.label + ' ' + (r.hint ?? '') + ' ' + r.group).toLowerCase().includes(q))
  })

  // Keep the cursor on a real row while filtering narrows the list.
  $effect(() => {
    if (cursor > filtered.length - 1) cursor = Math.max(0, filtered.length - 1)
  })

  async function pick(row: Row | undefined) {
    if (!row) return
    if (!row.run) return // status rows are not buttons
    await row.run()
    onclose()
  }

  function onKeydown(e: KeyboardEvent) {
    // stopPropagation, not just preventDefault: Escape also closes Settings at
    // the window level, and one press must dismiss one layer — the palette
    // opened over Settings, so it is the layer that closes.
    if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); onclose(); return }
    if (e.key === 'ArrowDown') { e.preventDefault(); cursor = Math.min(cursor + 1, filtered.length - 1) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); cursor = Math.max(cursor - 1, 0) }
    else if (e.key === 'Enter') { e.preventDefault(); void pick(filtered[cursor]) }
  }
</script>

<div class="palette" role="dialog" aria-label={t('palette.title')}>
  <input
    class="pal-search"
    bind:this={inputEl}
    bind:value={query}
    placeholder={mode === 'prompts' ? t('palette.searchPrompts') : t('palette.search')}
    onkeydown={onKeydown}
  />
  <div class="pal-list">
    {#if filtered.length === 0}
      <div class="pal-empty">{t('palette.noMatch')}</div>
    {/if}
    {#each filtered as row, i (row.id)}
      {#if i === 0 || filtered[i - 1].group !== row.group}
        <div class="pal-group">{row.group}</div>
      {/if}
      <button
        type="button"
        class="pal-row"
        class:on={i === cursor}
        class:static={!row.run}
        onmouseenter={() => (cursor = i)}
        onclick={() => pick(row)}
      >
        <span class="pal-label">{row.label}</span>
        {#if row.badge}<span class="pal-badge">{row.badge}</span>{/if}
        {#if row.hint}<span class="pal-hint">{row.hint}</span>{/if}
      </button>
    {/each}
  </div>
</div>
