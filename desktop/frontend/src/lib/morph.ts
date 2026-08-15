// Update a rendered answer in place instead of rebuilding it.
//
// A streaming reply re-renders its whole markdown on every token, and the
// obvious way to show that — `{@html html}` — assigns innerHTML, which DESTROYS
// every element and builds new ones. For prose nobody notices. For anything
// with a life of its own it is the difference between working and not:
//
//   - a CSS animation restarts from zero with the element that carries it, so
//     the running beam twitched in place instead of travelling, and a drawing
//     the model animated itself never got past its first frame;
//   - a text selection inside the reply is dropped mid-drag;
//   - an <img> or a <details> the user opened is rebuilt from scratch.
//
// So the new markup is reconciled against the old one node by node: same kind
// of node in the same place is kept and updated, anything else is replaced, and
// the tail is trimmed or extended. Streaming markdown is very nearly
// append-only, so in the common frame nothing above the last block moves at all.
//
// Not a general virtual DOM and deliberately not: there are no keys and no
// move detection, because the input is one document growing at its end rather
// than a list being reordered. What it must guarantee is narrower — an element
// that is still the same element in the next frame is the same element in the
// DOM — and that is enough for the three failures above.

/** Reconcile host's children against html, keeping every node it can. */
export function morphInto(host: Element, html: string): void {
  const next = host.ownerDocument.createElement('div')
  next.innerHTML = html
  patchChildren(host, next)
}

function patchChildren(oldParent: Node, newParent: Node): void {
  let oldNode = oldParent.firstChild
  let newNode = newParent.firstChild

  while (oldNode !== null && newNode !== null) {
    const nextOld = oldNode.nextSibling
    const nextNew = newNode.nextSibling
    if (comparable(oldNode, newNode)) {
      patchNode(oldNode, newNode)
    } else {
      oldParent.replaceChild(newNode, oldNode)
    }
    oldNode = nextOld
    newNode = nextNew
  }

  // Whatever is left over on one side: the answer grew, or a re-parse turned
  // three nodes into two.
  while (newNode !== null) {
    const nextNew = newNode.nextSibling
    oldParent.appendChild(newNode)
    newNode = nextNew
  }
  while (oldNode !== null) {
    const nextOld = oldNode.nextSibling
    oldParent.removeChild(oldNode)
    oldNode = nextOld
  }
}

// Same kind of node, same tag. Not the same class or the same id — those are
// attributes, and syncing them is the entire point: the `live` class moves from
// one block to the next as the answer grows, and moving it must not rebuild
// either block.
function comparable(a: Node, b: Node): boolean {
  if (a.nodeType !== b.nodeType) return false
  if (a.nodeType !== Node.ELEMENT_NODE) return true
  return (a as Element).tagName === (b as Element).tagName
}

function patchNode(oldNode: Node, newNode: Node): void {
  if (oldNode.nodeType !== Node.ELEMENT_NODE) {
    if (oldNode.nodeValue !== newNode.nodeValue) oldNode.nodeValue = newNode.nodeValue
    return
  }
  const oldEl = oldNode as Element
  const newEl = newNode as Element
  syncAttributes(oldEl, newEl)

  // A <style> is text to the DOM but a stylesheet to the engine, and rewriting
  // it restarts every animation its rules name. Only touched when it actually
  // differs — which, once a drawing's scope stopped changing per frame
  // (markdown.ts confineDrawing), is once.
  if (oldEl.tagName === 'STYLE') {
    if (oldEl.textContent !== newEl.textContent) oldEl.textContent = newEl.textContent
    return
  }
  patchChildren(oldEl, newEl)
}

function syncAttributes(oldEl: Element, newEl: Element): void {
  for (const attr of Array.from(newEl.attributes)) {
    if (oldEl.getAttribute(attr.name) !== attr.value) oldEl.setAttribute(attr.name, attr.value)
  }
  for (const attr of Array.from(oldEl.attributes)) {
    if (!newEl.hasAttribute(attr.name)) oldEl.removeAttribute(attr.name)
  }
}

/** Svelte action: `use:streamedHtml={html}` where `{@html html}` would go. */
export function streamedHtml(node: HTMLElement, html: string) {
  morphInto(node, html)
  return {
    update: (next: string) => morphInto(node, next),
  }
}
