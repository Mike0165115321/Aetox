<script lang="ts">
  // Shown in place of the editor when a file tab holds something this app
  // cannot render — today that is the .xlsx `sheet_write` produces, tomorrow
  // the .pptx and .docx on the same container (ARCHITECTURE.md §75).
  //
  // Deliberately not a preview. Aetox's promise is that the work happens on the
  // user's machine, and the program that opens a spreadsheet is already on it;
  // reproducing a worse Excel in here would mean writing an OOXML reader, which
  // OFFICE-EXPORT-PLAN.md §8 rules out.
  import { OpenFileExternally } from '../../../wailsjs/go/main/App'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'

  // `name` carries the label when there is no path to derive one from: a file
  // dropped on the desk that could not be brought in at all still names itself.
  let { path, reason, name }: { path: string; reason: string; name?: string } = $props()

  let failure = $state('')

  async function open() {
    failure = ''
    try {
      await OpenFileExternally(path)
    } catch (err) {
      failure = String(err)
    }
  }
</script>

<div class="pane-empty extfile">
  <span class="ic"><Icon name="fileText" size={28} /></span>
  <p class="name">{path ? path.split('/').pop() ?? path : name ?? ''}</p>
  <p class="why">{path ? t('workbench.cannotPreview') : t('workbench.dropFailed')}</p>
  {#if path}
    <!-- No path means the file never made it into the project, so there is
         nothing here for the OS to open — the reason line is the whole answer. -->
    <button type="button" class="proj-add" onclick={open}>
      <span class="ic"><Icon name="folderOpen" size={14} /></span> {t('workbench.openExternally')}
    </button>
  {/if}
  {#if failure}
    <p class="why err">{t('workbench.openFileError', { err: failure })}</p>
  {:else}
    <p class="why dim">{reason}</p>
  {/if}
</div>

<style>
  .extfile .name { font-size: var(--fs-md); color: var(--text-primary); word-break: break-all; }
  .extfile .why { font-size: var(--fs-sm); max-width: 34ch; }
  .extfile .why.dim { color: var(--text-dim); font-size: var(--fs-xs); }
  .extfile .why.err { color: var(--danger, #e5484d); }
  .extfile .ic { opacity: 0.5; }
</style>
