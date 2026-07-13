import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { MediaDetailsPage } from './MediaDetailsPage'

describe('MediaDetailsPage', () => {
  it('shows personal record and history before external metadata', async () => {
    server.use(
      http.get('*/api/v1/media/media-1', () =>
        HttpResponse.json({
          id: 'media-1',
          mediaType: 'movie',
          title: '花样年华',
          originalTitle: 'In the Mood for Love',
          releaseDate: '2000-09-29',
          overview: '两位邻居在克制与靠近之间建立起一段关系。',
          posterPath: null,
          backdropPath: '',
        }),
      ),
      http.get('*/api/v1/records/media-1', () =>
        HttpResponse.json({
          mediaId: 'media-1', status: 'completed', rating: 9.4, note: '雨夜与走廊。',
          watchedAt: '2026-07-12T12:00:00Z', viewingMethod: '家庭投影', version: 3,
        }),
      ),
      http.get('*/api/v1/records/media-1/events', () =>
        HttpResponse.json([
          {
            id: 'event-1', mediaId: 'media-1', watchedAt: '2026-07-12T20:30:00Z',
            viewingMethod: '家庭投影', source: 'manual', completion: 100,
          },
        ]),
      ),
      http.get('*/api/v1/household/participants', () =>
        HttpResponse.json([
          { id: 'member-1', username: 'family', role: 'member', active: true },
        ]),
      ),
    )
    renderWithQueryClient(
      <MemoryRouter initialEntries={['/media/media-1']}>
        <Routes>
          <Route path="/media/:mediaId" element={<MediaDetailsPage />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByRole('heading', { name: '花样年华', level: 1 })).toBeVisible()
    expect(screen.getByLabelText('花样年华 暂无海报')).toBeVisible()
    expect(screen.getByText('9.4 / 10')).toBeVisible()
    expect(screen.getByText('雨夜与走廊。')).toBeVisible()
    expect(screen.getByText('2026年7月12日')).toBeVisible()
    expect(await screen.findByRole('checkbox', { name: 'family' })).toBeVisible()
    const personalRecord = screen.getByRole('heading', { name: '个人记录' })
    const overview = screen.getByRole('heading', { name: '简介' })
    expect(personalRecord.compareDocumentPosition(overview) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})
