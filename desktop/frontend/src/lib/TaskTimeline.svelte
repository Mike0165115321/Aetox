<script lang="ts">
  import type { TimelineStep, StepStatus } from './types'
  import { t } from './i18n.svelte'
  import Icon from './Icon.svelte'
  import type { IconName } from './icons'

  let { steps, elapsed }: { steps: TimelineStep[]; elapsed: string } = $props()

  const nodeIcon: Record<StepStatus, IconName> = { done: 'check', active: 'arrowRight', wait: 'circle' }
</script>

<div class="timeline">
  <div class="tl-head">
    <span class="eyebrow">{t('taskTimeline.title')}</span>
    <span class="elapsed"><Icon name="timer" size={12} /> {elapsed}</span>
  </div>
  <div class="tl-body">
    {#each steps as step}
      <div class="step" class:is-wait={step.status === 'wait'}>
        <div class="node {step.status}"><Icon name={nodeIcon[step.status]} size={12} /></div>
        <div class="st-main">
          <div class="st-top"><span class="ts">{step.time}</span><b>{step.title}</b></div>
          <div class="st-sub">{step.detail}</div>

          {#if step.change}
            <div class="change-card">
              <div class="cc-h">{t('taskTimeline.changeSummary')}</div>
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
