import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { TMDBHighlight } from '../../api/types'
import { BackdropCarousel } from './BackdropCarousel'

type DeferredImage = {
  decode: ReturnType<typeof vi.fn>
  onerror: ((event: Event) => void) | null
  onload: ((event: Event) => void) | null
  rejectDecode: () => void
  resolveDecode: () => void
  src: string
}

const decodedImages: DeferredImage[] = []

function highlight(id: number, title: string): TMDBHighlight {
  return {
    id,
    mediaType: id % 2 === 0 ? 'tv' : 'movie',
    title,
    originalTitle: `${title} original`,
    year: '2026',
    overview: `${title} overview`,
    backdropURL: `/api/v1/public/tmdb/images/w1280/${id}.jpg?expires=42&signature=signed`,
  }
}

function installDecodedImageMock() {
  class TestImage {
    onerror: ((event: Event) => void) | null = null
    onload: ((event: Event) => void) | null = null
    src = ''
    private rejectPromise!: (reason: Error) => void
    private resolvePromise!: () => void
    private readonly decodePromise = new Promise<void>((resolve, reject) => {
      this.resolvePromise = resolve
      this.rejectPromise = reject
    })
    decode = vi.fn(() => this.decodePromise)

    constructor() {
      decodedImages.push(this)
    }

    resolveDecode = () => this.resolvePromise()
    rejectDecode = () => this.rejectPromise(new Error('decode failed'))
  }

  vi.stubGlobal('Image', TestImage)
}

async function resolveImage(index: number) {
  await act(async () => {
    decodedImages[index]!.resolveDecode()
    await Promise.resolve()
  })
}

async function rejectImage(index: number) {
  await act(async () => {
    decodedImages[index]!.rejectDecode()
    await Promise.resolve()
  })
}

function activeImage(container: HTMLElement) {
  return container.querySelector<HTMLImageElement>('.backdrop-carousel__image.is-active')
}

describe('BackdropCarousel', () => {
  const originalMatchMedia = Object.getOwnPropertyDescriptor(window, 'matchMedia')
  const originalVisibilityState = Object.getOwnPropertyDescriptor(document, 'visibilityState')
  let mediaMatches = false
  let visibilityState: DocumentVisibilityState = 'visible'
  let mediaChangeListener: ((event: MediaQueryListEvent) => void) | null = null
  let addMediaListener: ReturnType<typeof vi.fn>
  let removeMediaListener: ReturnType<typeof vi.fn>

  beforeEach(() => {
    vi.useFakeTimers()
    decodedImages.length = 0
    installDecodedImageMock()
    mediaMatches = false
    visibilityState = 'visible'
    mediaChangeListener = null
    addMediaListener = vi.fn((_type: string, listener: (event: MediaQueryListEvent) => void) => {
      mediaChangeListener = listener
    })
    removeMediaListener = vi.fn((_type: string, listener: (event: MediaQueryListEvent) => void) => {
      if (mediaChangeListener === listener) mediaChangeListener = null
    })
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn(() => ({
        addEventListener: addMediaListener,
        matches: mediaMatches,
        media: '(prefers-reduced-motion: reduce)',
        onchange: null,
        removeEventListener: removeMediaListener,
      })),
    })
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => visibilityState,
    })
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
    vi.unstubAllGlobals()
    if (originalMatchMedia) Object.defineProperty(window, 'matchMedia', originalMatchMedia)
    else delete (window as { matchMedia?: typeof window.matchMedia }).matchMedia
    if (originalVisibilityState) Object.defineProperty(document, 'visibilityState', originalVisibilityState)
  })

  it('does not display a backdrop until its preload image has decoded', async () => {
    const item = highlight(1, '降临')
    const { container } = render(<BackdropCarousel items={[item]} intervalMs={7_000} />)

    expect(activeImage(container)).toBeNull()
    expect(decodedImages).toHaveLength(1)

    await resolveImage(0)

    expect(activeImage(container)).toHaveAttribute('src', item.backdropURL)
    expect(activeImage(container)).toHaveAttribute('alt', '')
    expect(activeImage(container)).toHaveAttribute('aria-hidden', 'true')
  })

  it('skips a failed first image and activates the next decoded image', async () => {
    const items = [highlight(1, '失败图片'), highlight(2, '可用图片')]
    const onActiveItemChange = vi.fn()
    const { container } = render(
      <BackdropCarousel items={items} intervalMs={7_000} onActiveItemChange={onActiveItemChange} />,
    )

    await rejectImage(0)
    expect(decodedImages).toHaveLength(2)
    expect(activeImage(container)).toBeNull()

    await resolveImage(1)

    expect(activeImage(container)).toHaveAttribute('src', items[1]!.backdropURL)
    expect(onActiveItemChange).toHaveBeenLastCalledWith(items[1])
  })

  it('removes a rendered image that later errors and activates the next ready image', async () => {
    const items = [highlight(1, '第一张'), highlight(2, '第二张')]
    const { container } = render(<BackdropCarousel items={items} intervalMs={7_000} />)
    await resolveImage(0)
    await resolveImage(1)

    fireEvent.error(activeImage(container)!)

    expect(activeImage(container)).toHaveAttribute('src', items[1]!.backdropURL)
    expect(container.querySelector(`img[src="${items[0]!.backdropURL}"]`)).toBeNull()
  })

  it('renders a stable empty state and reports null when every image fails', async () => {
    const onActiveItemChange = vi.fn()
    const { container } = render(
      <BackdropCarousel
        items={[highlight(1, '失败一'), highlight(2, '失败二')]}
        intervalMs={7_000}
        onActiveItemChange={onActiveItemChange}
      />,
    )

    await rejectImage(0)
    await rejectImage(1)

    expect(container.firstElementChild).toHaveClass('backdrop-carousel', 'is-empty')
    expect(container.querySelector('img')).toBeNull()
    expect(onActiveItemChange).toHaveBeenLastCalledWith(null)
  })

  it('advances after the configured interval once two images are ready', async () => {
    const items = [highlight(1, '第一张'), highlight(2, '第二张')]
    const { container } = render(<BackdropCarousel items={items} intervalMs={7_000} />)
    await resolveImage(0)
    await resolveImage(1)

    expect(activeImage(container)).toHaveAttribute('src', items[0]!.backdropURL)

    act(() => vi.advanceTimersByTime(6_999))
    expect(activeImage(container)).toHaveAttribute('src', items[0]!.backdropURL)

    act(() => vi.advanceTimersByTime(1))
    expect(activeImage(container)).toHaveAttribute('src', items[1]!.backdropURL)
  })

  it('stops autoplay after manual navigation', async () => {
    const items = [highlight(1, '第一张'), highlight(2, '第二张')]
    const { container } = render(<BackdropCarousel items={items} intervalMs={7_000} showControls />)
    await resolveImage(0)
    await resolveImage(1)

    fireEvent.click(screen.getByRole('button', { name: '下一张背景' }))
    expect(activeImage(container)).toHaveAttribute('src', items[1]!.backdropURL)
    expect(screen.getByRole('button', { name: '继续轮播' })).toHaveAttribute('aria-pressed', 'true')

    act(() => vi.advanceTimersByTime(21_000))
    expect(activeImage(container)).toHaveAttribute('src', items[1]!.backdropURL)
  })

  it('stops while hidden and restarts a full interval after becoming visible', async () => {
    const items = [highlight(1, '第一张'), highlight(2, '第二张')]
    const { container } = render(<BackdropCarousel items={items} intervalMs={7_000} />)
    await resolveImage(0)
    await resolveImage(1)

    act(() => vi.advanceTimersByTime(3_500))
    visibilityState = 'hidden'
    act(() => document.dispatchEvent(new Event('visibilitychange')))
    act(() => vi.advanceTimersByTime(10_000))
    expect(activeImage(container)).toHaveAttribute('src', items[0]!.backdropURL)

    visibilityState = 'visible'
    act(() => document.dispatchEvent(new Event('visibilitychange')))
    act(() => vi.advanceTimersByTime(6_999))
    expect(activeImage(container)).toHaveAttribute('src', items[0]!.backdropURL)
    act(() => vi.advanceTimersByTime(1))
    expect(activeImage(container)).toHaveAttribute('src', items[1]!.backdropURL)
  })

  it('does not autoplay when reduced motion is preferred', async () => {
    mediaMatches = true
    const items = [highlight(1, '第一张'), highlight(2, '第二张')]
    const { container } = render(<BackdropCarousel items={items} intervalMs={7_000} />)
    await resolveImage(0)
    await resolveImage(1)

    act(() => vi.advanceTimersByTime(21_000))

    expect(activeImage(container)).toHaveAttribute('src', items[0]!.backdropURL)
  })

  it('renders stable, labelled previous, next, and pause controls', async () => {
    render(<BackdropCarousel items={[highlight(1, '一'), highlight(2, '二')]} intervalMs={7_000} showControls />)
    await resolveImage(0)
    await resolveImage(1)

    const controls = screen.getAllByRole('button')
    expect(controls).toHaveLength(3)
    expect(screen.getByRole('button', { name: '上一张背景' })).toBeEnabled()
    expect(screen.getByRole('button', { name: '下一张背景' })).toBeEnabled()
    const pause = screen.getByRole('button', { name: '暂停轮播' })
    expect(pause).toHaveAttribute('aria-pressed', 'false')

    fireEvent.click(pause)
    const resume = screen.getByRole('button', { name: '继续轮播' })
    expect(resume).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getAllByRole('button')).toHaveLength(3)

    fireEvent.click(resume)
    expect(screen.getByRole('button', { name: '暂停轮播' })).toHaveAttribute('aria-pressed', 'false')
  })

  it('uses load and error events when Image.decode is unavailable', async () => {
    const fallbackImages: Array<{
      onerror: ((event: Event) => void) | null
      onload: ((event: Event) => void) | null
      src: string
    }> = []
    class FallbackImage {
      onerror: ((event: Event) => void) | null = null
      onload: ((event: Event) => void) | null = null
      src = ''
      constructor() {
        fallbackImages.push(this)
      }
    }
    vi.stubGlobal('Image', FallbackImage)
    const item = highlight(1, '回退图片')
    const { container } = render(<BackdropCarousel items={[item]} intervalMs={7_000} />)

    expect(activeImage(container)).toBeNull()
    await act(async () => fallbackImages[0]!.onload?.(new Event('load')))

    expect(activeImage(container)).toHaveAttribute('src', item.backdropURL)
  })

  it('ignores stale decode results when items change', async () => {
    const firstCallback = vi.fn()
    const secondCallback = vi.fn()
    const { container, rerender } = render(
      <BackdropCarousel items={[highlight(1, '旧图片')]} intervalMs={7_000} onActiveItemChange={firstCallback} />,
    )
    const oldImage = decodedImages[0]!

    rerender(
      <BackdropCarousel items={[highlight(9, '新图片')]} intervalMs={7_000} onActiveItemChange={secondCallback} />,
    )
    expect(decodedImages).toHaveLength(2)

    await act(async () => {
      oldImage.resolveDecode()
      await Promise.resolve()
    })
    expect(activeImage(container)).toBeNull()
    expect(firstCallback).not.toHaveBeenCalled()

    await resolveImage(1)
    expect(activeImage(container)).toHaveAttribute('src', highlight(9, '新图片').backdropURL)
    expect(secondCallback).toHaveBeenLastCalledWith(highlight(9, '新图片'))
  })

  it('cleans up timers, listeners, and pending decode updates on unmount', async () => {
    const onActiveItemChange = vi.fn()
    const removeVisibilityListener = vi.spyOn(document, 'removeEventListener')
    const { unmount } = render(
      <BackdropCarousel
        items={[highlight(1, '一'), highlight(2, '二')]}
        intervalMs={7_000}
        onActiveItemChange={onActiveItemChange}
      />,
    )
    await resolveImage(0)
    await resolveImage(1)
    expect(vi.getTimerCount()).toBe(1)

    unmount()

    expect(vi.getTimerCount()).toBe(0)
    expect(removeVisibilityListener).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
    expect(removeMediaListener).toHaveBeenCalledWith('change', expect.any(Function))

    const callsAtUnmount = onActiveItemChange.mock.calls.length
    await act(async () => {
      decodedImages.forEach((image) => image.resolveDecode())
      await Promise.resolve()
    })
    expect(onActiveItemChange).toHaveBeenCalledTimes(callsAtUnmount)
  })
})
