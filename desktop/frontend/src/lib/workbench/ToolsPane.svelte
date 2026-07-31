<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { ListTools, ListSkills } from '../../../wailsjs/go/main/App'
  import { EventsOn } from '../../../wailsjs/runtime/runtime'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'
  import type { IconName } from '../icons'

  type SkillRow = { name: string; description: string; source: string }
  let skills = $state<SkillRow[]>([])

  // This panel is what the user plugged in, so it takes the MCP tools out of
  // the tool list and shows the skills beside them — two different kinds of
  // thing, listed as two, rather than merged under one word as they used to be.
  const groups: { key: string; label: string; icon: IconName }[] = $derived([
    { key: 'mcp', label: t('toolsPane.mcpTools'), icon: 'plug' },
    { key: 'skill', label: t('toolsPane.externalSkills'), icon: 'package' },
  ])

  async function load() {
    const [tools, docs] = await Promise.all([ListTools(), ListSkills()])
    skills = [...tools.filter((s) => s.source === 'mcp'), ...docs]
  }
  onMount(load)
  // MCP servers connect in the background after startup; the backend emits
  // this once their tools are registered, so the panel fills in on its own.
  const off = EventsOn('skills:updated', load)
  onDestroy(() => off())
</script>

<div class="insp-scroll">
  <div style="padding:8px">
    <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px">
      <span class="muted tp-sub">{t('toolsPane.addedTools', { count: skills.length })}</span>
      <button class="ctrl" onclick={load}><Icon name="rotateCw" size={13} /> {t('toolsPane.refresh')}</button>
    </div>

    {#each groups as g}
      {@const items = skills.filter((s) => s.source === g.key)}
      {#if items.length > 0}
        <div class="eyebrow" style="margin:12px 0 4px"><Icon name={g.icon} size={13} /> {g.label} ({items.length})</div>
        {#each items as s}
          <div style="padding:6px 8px; border-radius:6px">
            <div style="font-weight:600">{s.name}</div>
            {#if s.description}<div class="muted tp-sub">{s.description}</div>{/if}
          </div>
        {/each}
      {/if}
    {/each}

    {#if skills.length === 0}
      <div class="empty">{t('toolsPane.noTools')}</div>
    {/if}
  </div>
</div>
