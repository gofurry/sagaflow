import { useEffect, useRef, useState, type CSSProperties, type RefObject } from 'react'

interface Props {
  scrollerRef: RefObject<HTMLElement | null>
  refreshKey?: string
  minScrollableDistance?: number
}

interface ScrollState {
  top: number
  max: number
  progress: number
  desktop: boolean
}

export function PageScrollOrb({ scrollerRef, refreshKey, minScrollableDistance = 320 }: Props) {
  const frame = useRef<number | null>(null)
  const refreshTimer = useRef<number | null>(null)
  const [state, setState] = useState<ScrollState>({ top: 0, max: 0, progress: 0, desktop: window.innerWidth >= 768 })

  useEffect(() => {
    const scroller = scrollerRef.current
    if (!scroller) return

    const update = () => {
      const max = Math.max(scroller.scrollHeight - scroller.clientHeight, 0)
      const top = scroller.scrollTop
      setState({
        top,
        max,
        progress: max > 0 ? Math.min(100, Math.max(0, top / max * 100)) : 0,
        desktop: window.innerWidth >= 768,
      })
    }
    const scheduleUpdate = () => {
      if (frame.current !== null) return
      frame.current = window.requestAnimationFrame(() => {
        frame.current = null
        update()
      })
    }
    const scheduleRefresh = () => {
      if (refreshTimer.current !== null) window.clearTimeout(refreshTimer.current)
      refreshTimer.current = window.setTimeout(update, 120)
    }
    const resizeObserver = new ResizeObserver(scheduleRefresh)
    const observeContent = () => {
      resizeObserver.disconnect()
      resizeObserver.observe(scroller)
      Array.from(scroller.children).forEach((child) => resizeObserver.observe(child))
    }
    const mutationObserver = new MutationObserver(() => {
      observeContent()
      scheduleRefresh()
    })

    observeContent()
    mutationObserver.observe(scroller, { childList: true, subtree: true, characterData: true })
    scroller.addEventListener('scroll', scheduleUpdate, { passive: true })
    window.addEventListener('resize', scheduleUpdate)
    update()
    window.requestAnimationFrame(update)

    return () => {
      if (frame.current !== null) window.cancelAnimationFrame(frame.current)
      if (refreshTimer.current !== null) window.clearTimeout(refreshTimer.current)
      resizeObserver.disconnect()
      mutationObserver.disconnect()
      scroller.removeEventListener('scroll', scheduleUpdate)
      window.removeEventListener('resize', scheduleUpdate)
    }
  }, [refreshKey, scrollerRef])

  if (!state.desktop || state.max <= minScrollableDistance) return null

  const progress = Math.round(state.progress)
  const style = { '--scroll-progress': `${progress}%` } as CSSProperties
  return <button
    aria-label={`页面滚动进度 ${progress}%`}
    className={`page-scroll-orb${state.top > 72 ? ' visible' : ''}`}
    onClick={() => scrollerRef.current?.scrollTo({ top: Math.max(0, state.top - state.max * .25), behavior: 'smooth' })}
    style={style}
    title="向上滚动 25%"
    type="button"
  >
    <span>{progress}%</span>
  </button>
}
