import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { delay, http, HttpResponse } from 'msw'
import { MemoryRouter } from 'react-router-dom'
import { expect, it } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { CandidateReviewPage } from './CandidateReviewPage'

it('reviews evidence and supports confirm, rematch, ignore, and retained custom input', async () => {
  const actions: string[] = []
  let customAttempts = 0
  server.use(
    http.get('*/api/v1/sync/candidates', () => HttpResponse.json(candidateFixtures())),
    http.post('*/api/v1/sync/candidates/c-confirm/confirm', ({ request }) => {
      assertProtectedWrite(request)
      actions.push('confirm')
      return HttpResponse.json({ ...candidateFixtures()[0], status: 'confirmed' })
    }),
    http.post('*/api/v1/sync/candidates/c-rematch/rematch', async ({ request }) => {
      assertProtectedWrite(request)
      expect(await request.json()).toEqual({ mediaId: 'media-b', episodeId: '' })
      actions.push('rematch')
      return HttpResponse.json({ ...candidateFixtures()[1], status: 'confirmed', mediaId: 'media-b' })
    }),
    http.post('*/api/v1/sync/candidates/c-ignore/ignore', ({ request }) => {
      assertProtectedWrite(request)
      actions.push('ignore')
      return HttpResponse.json({ ...candidateFixtures()[2], status: 'ignored' })
    }),
    http.post('*/api/v1/sync/candidates/c-custom/custom', async ({ request }) => {
      assertProtectedWrite(request)
      customAttempts++
      const body = await request.json()
      expect(body).toEqual({ title: '保留的自定义剧名', mediaType: 'tv', year: '2026' })
      if (customAttempts === 1) return HttpResponse.json({ code: 'internal_error' }, { status: 500 })
      actions.push('custom')
      return HttpResponse.json({ ...candidateFixtures()[3], status: 'confirmed', mediaId: 'custom-media' })
    }),
  )
  sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
  const user = userEvent.setup()
  renderWithQueryClient(<MemoryRouter><CandidateReviewPage /></MemoryRouter>)

  expect(await screen.findByRole('heading', { name: '同步候选' })).toBeVisible()
  expect(await screen.findByText('标题和年份找到 1 个可能匹配，需要人工确认')).toBeVisible()
  expect(screen.getByText('冲突')).toBeVisible()

  const confirmCandidate = screen.getByRole('article', { name: 'Localized Film' })
  await user.click(within(confirmCandidate).getByRole('button', { name: '确认此匹配' }))

  const rematchCandidate = screen.getByRole('article', { name: 'Ambiguous Film' })
  await user.click(within(rematchCandidate).getByRole('radio', {
    name: /候选 2[\s\S]*Ambiguous Film[\s\S]*Second Original[\s\S]*2024/,
  }))
  await user.click(within(rematchCandidate).getByRole('button', { name: '使用所选匹配' }))

  const ignoredCandidate = screen.getByRole('article', { name: 'Ignore Film' })
  await user.click(within(ignoredCandidate).getByRole('button', { name: '忽略此事件' }))

  const customCandidate = screen.getByRole('article', { name: 'New Series' })
  await user.click(within(customCandidate).getByRole('button', { name: '创建自定义条目' }))
  const titleInput = within(customCandidate).getByRole('textbox', { name: '自定义标题' })
  expect(titleInput).toHaveFocus()
  await user.clear(titleInput)
  await user.type(titleInput, '保留的自定义剧名')
  await user.click(within(customCandidate).getByRole('button', { name: '保存并导入' }))
  expect(await within(customCandidate).findByRole('alert')).toHaveTextContent('保存失败')
  expect(titleInput).toHaveValue('保留的自定义剧名')
  await user.click(within(customCandidate).getByRole('button', { name: '保存并导入' }))

  await waitFor(() => expect(actions).toEqual(['confirm', 'rematch', 'ignore', 'custom']))
})

it('shows loading, empty, and recoverable error states', async () => {
  let requests = 0
  server.use(http.get('*/api/v1/sync/candidates', async () => {
    requests++
    if (requests === 1) {
      await delay(30)
      return HttpResponse.json({ code: 'internal_error' }, { status: 500 })
    }
    return HttpResponse.json([])
  }))
  const user = userEvent.setup()
  renderWithQueryClient(<MemoryRouter><CandidateReviewPage /></MemoryRouter>)

  expect(screen.getByLabelText('正在加载同步候选')).toBeVisible()
  expect(await screen.findByRole('alert')).toHaveTextContent('无法读取同步候选')
  await user.click(screen.getByRole('button', { name: '重试' }))

  expect(await screen.findByText('没有需要核对的同步记录')).toBeVisible()
  expect(screen.getByRole('link', { name: '返回设置' })).toHaveAttribute('href', '/settings')
})

function assertProtectedWrite(request: Request) {
  expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
  expect(request.headers.get('Idempotency-Key')).toBeTruthy()
}

function candidateFixtures() {
  return [
    candidate('c-confirm', 'possible', 'Localized Film', {
      mediaId: 'media-confirm',
      evidence: [{ code: 'title_year_match', text: '标题和年份找到 1 个可能匹配，需要人工确认' }],
      options: [option('media-confirm', 'Localized Film', '2024')],
    }),
    candidate('c-rematch', 'conflict', 'Ambiguous Film', {
      evidence: [{ code: 'external_id_conflict', text: '外部 ID 指向 2 个不同条目' }],
      options: [
        option('media-a', 'Ambiguous Film', '2024', 'First Original'),
        option('media-b', 'Ambiguous Film', '2024', 'Second Original'),
      ],
    }),
    candidate('c-ignore', 'unmatched', 'Ignore Film', {
      evidence: [{ code: 'no_match', text: '未找到可用的外部 ID、标题和年份匹配' }],
    }),
    candidate('c-custom', 'unmatched', 'New Series', {
      event: { mediaType: 'episode', year: 2026, seasonNumber: 1, episodeNumber: 2 },
      evidence: [{ code: 'no_match', text: '未找到可用的外部 ID、标题和年份匹配' }],
    }),
  ]
}

function candidate(
  id: string,
  status: string,
  title: string,
  overrides: Record<string, unknown> = {},
) {
  const eventOverride = (overrides.event ?? {}) as Record<string, unknown>
  return {
    id,
    accountId: 'account-1',
    externalEventId: `event-${id}`,
    status,
    evidence: [],
    options: [],
    createdAt: '2026-07-13T08:31:00Z',
    updatedAt: '2026-07-13T08:31:00Z',
    ...overrides,
    event: {
      playedAt: '2026-07-13T08:30:00Z',
      durationSeconds: 7200,
      positionSeconds: 7200,
      providerItemId: `provider-${id}`,
      mediaType: 'movie',
      title,
      year: 2024,
      ...eventOverride,
    },
  }
}

function option(mediaId: string, title: string, year: string, originalTitle = title) {
  return { mediaId, mediaType: 'movie', title, originalTitle, year }
}
