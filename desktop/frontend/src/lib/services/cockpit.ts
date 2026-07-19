// The data-source seam. A CockpitSource supplies cockpit state; the store hydrates
// from whichever one is wired. Today: MockSource. Later: a WailsSource that calls
// the Go core bindings — same interface, no component changes.

import type { CockpitState } from '../types'

export interface CockpitSource {
  load(): CockpitState | Promise<CockpitState>
}

// When the Go core exposes real project/git state, add:
//
//   import { GetCockpitState } from '../../wailsjs/go/main/App'
//   export class WailsSource implements CockpitSource {
//     async load(): Promise<CockpitState> { return await GetCockpitState() }
//   }
