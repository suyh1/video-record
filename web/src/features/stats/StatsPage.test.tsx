import { screen, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { expect, it } from 'vitest'

import type { StatsSummary } from '../../api/types'
import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { StatsPage } from './StatsPage'

const summary: StatsSummary = {
  totalWatches: 12,
  uniqueMedia: 9,
  totalMinutes: 750,
  repeatWatches: 3,
  monthly: [{ label: '2026-06', value: 4 }, { label: '2026-07', value: 8 }],
  yearly: [{ label: '2025', value: 7 }, { label: '2026', value: 12 }],
  genres: [{ label: '剧情', value: 7 }, { label: '科幻', value: 5 }],
  ratings: [{ label: '8.0-8.9', value: 4 }, { label: '9.0-10.0', value: 2 }],
  tags: [{ label: '家庭', value: 3 }],
  viewingMethods: [{ label: '影院', value: 5 }, { label: '家庭电视', value: 7 }],
}

it('renders private viewing totals and a textual table for every chart', async () => {
  server.use(http.get('*/api/v1/stats', () => HttpResponse.json(summary)))
  renderWithQueryClient(<StatsPage />)

  expect(await screen.findByRole('heading', { name: '统计' })).toBeVisible()
  expect(screen.getByText('12 次观看')).toBeVisible()
  expect(screen.getByText('12 小时 30 分')).toBeVisible()
  expect(screen.getByText('3 次重看')).toBeVisible()

  const expectedTables = ['月度观看数据', '年度观看数据', '类型分布数据', '评分分布数据', '标签分布数据', '观看方式数据']
  for (const name of expectedTables) {
    const table = screen.getByRole('table', { name })
    expect(table).toBeVisible()
    expect(within(table).getAllByRole('row').length).toBeGreaterThan(1)
  }
  expect(screen.getByRole('img', { name: '类型分布图' })).toHaveTextContent('剧情')
  expect(screen.getByRole('img', { name: '观看方式图' })).toHaveTextContent('家庭电视')
})
