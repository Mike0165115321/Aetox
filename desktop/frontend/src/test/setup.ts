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

// IntersectionObserver, same reason and one deliberate difference: this one
// FIRES, once, reporting the element as on screen.
//
// BrowserPane asks it a question the other observer cannot answer — does this
// pane have a box, counting every ancestor — and hides the native browser
// window when the answer is no. A stub that never calls back would leave that
// answer at its initial value in every test, which happens to be the value the
// tests want and would therefore prove nothing. Firing true once makes the
// default path real; a test that cares about the false case drives its own
// observer, which browserOrphan.test.ts does.
if (typeof globalThis.IntersectionObserver === 'undefined') {
  globalThis.IntersectionObserver = class {
    constructor(private cb: IntersectionObserverCallback) {}
    observe(el: Element) {
      this.cb(
        [{ target: el, isIntersecting: true, intersectionRatio: 1 } as IntersectionObserverEntry],
        this as unknown as IntersectionObserver,
      )
    }
    unobserve() {}
    disconnect() {}
    takeRecords() { return [] }
  } as unknown as typeof IntersectionObserver
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

// The native bridge object. The app's frontend always runs inside its own
// webview, and WebView2 marks that webview with window.chrome.webview - which
// is what hostWebview.ts reads to decide whether this frontend may drive the
// native surfaces (browser windows, the PTY) or is a wails-dev spectator that
// must stand down (§191). jsdom has neither chrome nor webkit, so without this
// every component test would render as the stood-down spectator and every
// existing assertion about BrowserSetBounds / TerminalResize would be asserting
// against a frontend the app never ships. The spectator test deletes it to BE
// the outsider.
if (!(window as any).chrome?.webview) {
  ;(window as any).chrome = { ...(window as any).chrome, webview: {} }
}
