// What jsdom is missing that the app actually uses.
//
// Only genuine environment gaps belong here — things a real browser has and
// jsdom does not. Anything about how *our* code behaves belongs in a test, not
// in a shim that every test silently inherits.

// The Web Animations API. jsdom does not implement `element.animate`, and
// Svelte 5 runs every `transition:` through it — so the first transition to
// appear in a component under test threw `element.animate is not a function`
// from inside Svelte, on a timer, outside any test's call stack.
//
// That shape is worth naming: vitest reported it as an *unhandled error*, not
// as a failing test. Every assertion still passed, the summary still read
// "56 passed (56)", and the run still exited 1 — so a green-looking local run
// piped through `tail` hid it completely, and CI was the first thing to say so
// out loud (2026-08-16).
//
// The stub reports a finished animation and calls `onfinish` as soon as Svelte
// attaches one, which is what makes an outro complete and the element leave the
// document — the same end state a real browser reaches, minus the frames in
// between. Tests that care about a transition's *timing* would need a real
// clock; none do, and if one ever does it should say so itself rather than
// change this for everyone.
// ResizeObserver. jsdom does not implement it, and every real browser has since
// 2020 — including the WebView2 the app actually ships in. toolGlide uses one to
// re-measure the live row when the timeline's width changes, and without this
// every test that renders a tool timeline died on `ResizeObserver is not
// defined` (2026-08-25).
//
// It never fires, which is correct rather than lazy: jsdom lays nothing out, so
// there is no resize to observe and a callback would be reporting a measurement
// that does not exist. What the observer is FOR is covered by the MutationObserver
// beside it, which jsdom does have.
if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver
}

if (typeof Element !== 'undefined' && !Element.prototype.animate) {
  Element.prototype.animate = function (): Animation {
    let finishHandler: ((this: Animation, ev: AnimationPlaybackEvent) => unknown) | null = null
    const animation = {
      currentTime: 0,
      startTime: 0,
      playbackRate: 1,
      playState: 'finished' as AnimationPlayState,
      effect: null,
      timeline: null,
      id: '',
      pending: false,
      // Both resolve to the animation itself, which is what the real API does
      // and what the type says. Getters rather than fields because the value is
      // the object being defined, and a field would have to name it before the
      // const exists.
      get ready(): Promise<Animation> {
        return Promise.resolve(animation as Animation)
      },
      get finished(): Promise<Animation> {
        return Promise.resolve(animation as Animation)
      },
      oncancel: null,
      onremove: null,
      get onfinish() {
        return finishHandler
      },
      set onfinish(handler) {
        finishHandler = handler
        if (handler) queueMicrotask(() => handler.call(animation as Animation, {} as AnimationPlaybackEvent))
      },
      cancel() {},
      play() {},
      pause() {},
      reverse() {},
      persist() {},
      // A readonly state, not a method — the animation has not been replaced by
      // a later one. Part of the Animation interface in the lib.dom the current
      // TypeScript ships, and absent here the casts below stop being casts and
      // become errors. Completed rather than widened through `unknown`, which
      // would turn every future addition into silence instead of a message.
      replaceState: 'active' as AnimationReplaceState,
      commitStyles() {},
      updatePlaybackRate() {},
      finish() {
        finishHandler?.call(animation as Animation, {} as AnimationPlaybackEvent)
      },
      addEventListener() {},
      removeEventListener() {},
      dispatchEvent() {
        return true
      },
    }
    return animation as unknown as Animation
  } as Element['animate']
}
