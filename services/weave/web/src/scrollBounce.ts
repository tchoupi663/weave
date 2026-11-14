// A page must not look frozen at the end of a scroll. Every scroll container
// acknowledges its end instead of stopping dead: the incident stage, which
// owns its wheel, glows at the edge it ran out of, and the other containers
// take the browser's own overscroll.
//
// The signal only appears once scrolling has settled at the edge, so it never
// flickers mid-scroll and never animates at rest. Presentation lives in
// styles.css (`data-rubber`, `data-rubber-soft`).
const SELECTOR = [
  '#root',
  '.inspector',
  '.incident-stage',
  '.incident-list',
  '.incident-compose',
].join(', ')

// Containers that own their wheel listener and cannot transform without
// breaking layout get the edge glow instead of a pull.
const SOFT = new Set(['incident-stage', 'incident-compose'])

/** Idle time before a container is considered to have stopped at its edge. */
const EDGE_SETTLE_MS = 180
/** How long the signal stays up after the last gesture. */
const SETTLE_MS = 900
const GLOW_SPAN = 40

type Edge = 'top' | 'bottom' | 'left' | 'right'

export function installScrollBounce(stop: () => boolean): () => void {
  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)')
  const timers = new Map<HTMLElement, number>()
  const pending = new Map<HTMLElement, number>()
  const signalled = new Set<HTMLElement>()

  const restore = (element: HTMLElement) => {
    signalled.delete(element)
    element.removeAttribute('data-rubber')
    element.removeAttribute('data-rubber-soft')
  }

  const signal = (element: HTMLElement, edge: Edge, force: number) => {
    const soft = SOFT.has(element.className.split(' ')[0])
    const amount = Math.min(1, Math.abs(force) / GLOW_SPAN).toFixed(2)
    signalled.add(element)
    element.dataset.edge = edge
    element.style.setProperty('--rubber-amount', amount)
    element.setAttribute(soft ? 'data-rubber-soft' : 'data-rubber', '')
  }

  const schedule = (element: HTMLElement) => {
    const previous = timers.get(element)
    if (previous !== undefined) window.clearTimeout(previous)
    timers.set(element, window.setTimeout(() => {
      timers.delete(element)
      restore(element)
    }, SETTLE_MS))
  }

  const onWheel = (event: WheelEvent) => {
    if (event.defaultPrevented || reduced.matches || stop()) return
    const target = event.target
    const element = target instanceof Element ? target.closest<HTMLElement>(SELECTOR) : null
    if (!element) return

    const verticalRoom = element.scrollHeight - element.clientHeight
    const horizontalRoom = element.scrollWidth - element.clientWidth
    const atTop = element.scrollTop <= 0
    const atBottom = element.scrollTop + element.clientHeight >= element.scrollHeight - 1

    if (verticalRoom > 1 && event.deltaY) {
      const atTopEdge = atTop && event.deltaY < 0
      const atBottomEdge = atBottom && event.deltaY > 0
      if (!atTopEdge && !atBottomEdge) return
      const edge: Edge = atTopEdge ? 'top' : 'bottom'
      const force = Math.abs(event.deltaY)
      window.clearTimeout(pending.get(element))
      pending.set(element, window.setTimeout(() => {
        pending.delete(element)
        signal(element, edge, force)
        schedule(element)
      }, EDGE_SETTLE_MS))
      return
    }

    if (horizontalRoom > 1 && Math.abs(event.deltaX) > Math.abs(event.deltaY)) {
      const atStart = element.scrollLeft <= 0
      const atEnd = element.scrollLeft + element.clientWidth >= element.scrollWidth - 1
      if ((atStart && event.deltaX < 0) || (atEnd && event.deltaX > 0)) {
        // Release the native overscroll so the rail does not also jump.
        event.preventDefault()
        signal(element, atStart ? 'left' : 'right', Math.abs(event.deltaX))
        schedule(element)
      }
      return
    }

    // A container that fits on screen has no native scroll, so any wheel past
    // it is the whole page's end.
    if (verticalRoom <= 1 && event.deltaY && !SOFT.has(element.className.split(' ')[0])) {
      signal(element, event.deltaY < 0 ? 'top' : 'bottom', Math.abs(event.deltaY))
      schedule(element)
    }
  }

  const onScroll = (event: Event) => {
    const target = event.target
    const element = target instanceof Element ? target.closest<HTMLElement>(SELECTOR) : null
    if (!element) return
    const atEdge = element.scrollTop <= 0
      || element.scrollTop + element.clientHeight >= element.scrollHeight - 1
    if (!atEdge) {
      restore(element)
      return
    }
    if (signalled.has(element)) schedule(element)
  }

  const onKey = (event: KeyboardEvent) => {
    if (reduced.matches) return
    const target = event.target as HTMLElement | null
    if (target && target.closest('input, textarea, select, [contenteditable="true"]')) return
    if (!['PageDown', 'PageUp', 'Home', 'End', 'ArrowUp', 'ArrowDown'].includes(event.key)) return
    const element = target?.closest<HTMLElement>(SELECTOR)
    if (!element) return
    window.setTimeout(() => {
      const atTop = element.scrollTop <= 0
      const atBottom = element.scrollTop + element.clientHeight >= element.scrollHeight - 1
      if (!atTop && !atBottom) return
      signal(element, atTop ? 'top' : 'bottom', 18)
      schedule(element)
    }, 0)
  }

  window.addEventListener('wheel', onWheel, { passive: false })
  window.addEventListener('scroll', onScroll, { passive: true, capture: true })
  window.addEventListener('keydown', onKey)

  const onMedia = () => {
    for (const element of signalled) restore(element)
  }
  reduced.addEventListener?.('change', onMedia)

  return () => {
    window.removeEventListener('wheel', onWheel)
    window.removeEventListener('scroll', onScroll, true)
    window.removeEventListener('keydown', onKey)
    reduced.removeEventListener?.('change', onMedia)
    for (const id of timers.values()) window.clearTimeout(id)
    for (const id of pending.values()) window.clearTimeout(id)
    timers.clear()
    pending.clear()
  }
}
