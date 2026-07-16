// The data-source seam. A CockpitSource supplies cockpit state; the store hydrates
// from whichever one is wired. Today: MockSource. Later: a WailsSource that calls
// the Go core bindings — same interface, no component changes.

import type { CockpitState } from '../types'
import { mockState } from '../mockData'

export interface CockpitSource {
  load(): CockpitState | Promise<CockpitState>
}

export class MockSource implements CockpitSource {
  load(): CockpitState {
    // structuredClone so the store owns its copy and edits never mutate the sample.
    return structuredClone(mockState)
  }
}

// When the Go core is bound, add:
//
//   import { GetCockpitState } from '../../wailsjs/go/main/App'
//   export class WailsSource implements CockpitSource {
//     async load(): Promise<CockpitState> { return await GetCockpitState() }
//   }
