<script lang="ts">
  import { onMount } from 'svelte'
  import { ListSkills } from '../../../wailsjs/go/main/App'

  type SkillRow = { name: string; description: string; source: string }
  let skills = $state<SkillRow[]>([])

  // The backend (ListSkills) decides what belongs here — MCP tools and
  // discovered skills, never embedded built-ins. This just renders the groups.
  const groups = [
    { key: 'mcp', label: 'MCP tools', icon: '🔌' },
    { key: 'external', label: 'สกิลภายนอก', icon: '📦' },
  ]

  async function load() {
    skills = await ListSkills()
  }
  onMount(load)
</script>

<div class="insp-scroll">
  <div style="padding:8px">
    <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px">
      <span class="muted" style="font-size:12px">เครื่องมือที่เพิ่มเข้ามา ({skills.length})</span>
      <button class="ctrl" onclick={load}>↻ รีเฟรช</button>
    </div>

    {#each groups as g}
      {@const items = skills.filter((s) => s.source === g.key)}
      {#if items.length > 0}
        <div class="eyebrow" style="margin:12px 0 4px">{g.icon} {g.label} ({items.length})</div>
        {#each items as s}
          <div style="padding:6px 8px; border-radius:6px">
            <div style="font-weight:600">{s.name}</div>
            {#if s.description}<div class="muted" style="font-size:12px">{s.description}</div>{/if}
          </div>
        {/each}
      {/if}
    {/each}

    {#if skills.length === 0}
      <div class="empty">ยังไม่มี MCP server หรือสกิลภายนอก — เพิ่ม MCP server ได้ที่ Settings → MCP servers</div>
    {/if}
  </div>
</div>
