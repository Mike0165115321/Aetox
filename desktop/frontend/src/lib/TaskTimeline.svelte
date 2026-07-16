<script lang="ts">
  import type { TimelineStep, StepStatus } from './types'

  let { steps, elapsed }: { steps: TimelineStep[]; elapsed: string } = $props()

  const nodeGlyph: Record<StepStatus, string> = { done: '✓', active: '➜', wait: '○' }
</script>

<div class="timeline">
  <div class="tl-head">
    <span class="eyebrow">Task Timeline</span>
    <span class="elapsed">⏱ {elapsed}</span>
  </div>
  <div class="tl-body">
    {#each steps as step}
      <div class="step" class:is-wait={step.status === 'wait'}>
        <div class="node {step.status}">{nodeGlyph[step.status]}</div>
        <div class="st-main">
          <div class="st-top"><span class="ts">{step.time}</span><b>{step.title}</b></div>
          <div class="st-sub">{step.detail}</div>

          {#if step.change}
            <div class="change-card">
              <div class="cc-h">Change Summary</div>
              <ul>
                {#each step.change.items as item}<li>{item}</li>{/each}
              </ul>
              <div class="cc-f">
                <span>{step.change.footer}</span>
                <span class="badge-edit">{step.change.badge}</span>
              </div>
            </div>
          {/if}
        </div>
      </div>
    {/each}
  </div>
</div>
