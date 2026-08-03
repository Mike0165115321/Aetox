<script lang="ts">
  // A read-only look at a workbook the agent produced, so "what did I just
  // get?" is answered without leaving the window.
  //
  // Read-only is the whole design (ARCHITECTURE.md §79). Aetox has no business
  // being a worse Excel, and the button that hands the file to the real one
  // stays right there in the header — this is a glance, not a spreadsheet.
  import { OpenFileExternally } from '../../../wailsjs/go/main/App'
  import type { ooxml } from '../../../wailsjs/go/models'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'

  let { path, preview }: { path: string; preview: ooxml.WorkbookPreview } = $props()

  let active = $state(0)
  let failure = $state('')

  const sheet = $derived(preview.sheets?.[active])
  // The first row is the header only when it looks like one: every workbook
  // sheet_write produces has one, an arbitrary one may not, and drawing a data
  // row as a header is a small lie about what the file contains.
  const header = $derived(sheet?.rows?.[0] ?? [])
  const body = $derived(sheet?.rows?.slice(1) ?? [])
  const columns = $derived(Math.max(header.length, ...body.map((r) => r.length), 0))

  // A cell that is entirely digits, separators and a sign is a number, and
  // numbers read wrong ranged left. Deliberately a display decision only — the
  // string came from Go already formatted the way Excel shows it.
  const numeric = (v: string) => v !== '' && /^[-+]?[\d,]*\.?\d+%?$/.test(v.trim())

  async function openExternally() {
    failure = ''
    try {
      await OpenFileExternally(path)
    } catch (err) {
      failure = t('workbench.openFileError', { err: String(err) })
    }
  }
</script>

<div class="sheet-pane">
  <div class="sheet-head">
    {#if (preview.sheets?.length ?? 0) > 1}
      <div class="sheet-tabs">
        {#each preview.sheets as s, i}
          <button type="button" class="sheet-tab" class:active={active === i} onclick={() => (active = i)}>{s.name}</button>
        {/each}
      </div>
    {:else}
      <span class="sheet-name">{sheet?.name ?? ''}</span>
    {/if}
    <button type="button" class="ctrl" onclick={openExternally}>
      <Icon name="folderOpen" size={13} /> {t('workbench.openExternally')}
    </button>
  </div>

  {#if failure}
    <div class="sheet-note err">{failure}</div>
  {/if}

  <div class="sheet-scroll">
    <table class="sheet-grid">
      <thead>
        <tr>
          <th class="rownum"></th>
          {#each Array(columns) as _, c}
            <th class:num={numeric(header[c] ?? '')}>{header[c] ?? ''}</th>
          {/each}
        </tr>
      </thead>
      <tbody>
        {#each body as row, r}
          <tr>
            <td class="rownum">{r + 2}</td>
            {#each Array(columns) as _, c}
              <td class:num={numeric(row[c] ?? '')}>{row[c] ?? ''}</td>
            {/each}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>

  {#if sheet?.truncated}
    <!-- Saying so matters more than showing more: a prefix displayed as if it
         were the whole sheet is the one failure that misleads. -->
    <div class="sheet-note">{t('workbench.sheetTruncated', { shown: String(body.length + 1), total: String(sheet.totalRows) })}</div>
  {/if}
</div>

<style>
  .sheet-pane { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .sheet-head { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-bottom: 1px solid var(--border); flex: none; }
  .sheet-head .ctrl { margin-left: auto; flex: none; }
  .sheet-name { font-size: var(--fs-sm); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sheet-tabs { display: flex; gap: 2px; overflow-x: auto; }
  .sheet-tab { border: none; background: none; color: var(--text-muted); font-size: var(--fs-sm); padding: 3px 8px; border-radius: var(--r-sm); cursor: pointer; white-space: nowrap; }
  .sheet-tab:hover { background: var(--surface-row-hover); color: var(--text-primary); }
  .sheet-tab.active { background: var(--surface-raised); color: var(--text-primary); }

  .sheet-scroll { flex: 1; min-height: 0; overflow: auto; }
  .sheet-grid { border-collapse: collapse; font-size: var(--fs-sm); }
  .sheet-grid th, .sheet-grid td { border: 1px solid var(--border); padding: 4px 8px; text-align: left; white-space: nowrap; max-width: 340px; overflow: hidden; text-overflow: ellipsis; }
  .sheet-grid th { background: var(--surface-raised); color: var(--text-primary); font-weight: 600; position: sticky; top: 0; z-index: 1; }
  .sheet-grid td { color: var(--text-primary); }
  .sheet-grid .num { text-align: right; font-variant-numeric: tabular-nums; }
  /* The row numbers are the spreadsheet's own, so the preview and Excel agree
     when the user goes looking for a row. */
  .sheet-grid .rownum { position: sticky; left: 0; background: var(--surface-raised); color: var(--text-dim); text-align: right; font-variant-numeric: tabular-nums; z-index: 2; user-select: none; }
  .sheet-grid thead .rownum { z-index: 3; }

  .sheet-note { flex: none; padding: 6px 10px; font-size: var(--fs-xs); color: var(--text-muted); border-top: 1px solid var(--border); }
  .sheet-note.err { color: var(--status-danger); border-top: none; border-bottom: 1px solid var(--border); }
</style>
