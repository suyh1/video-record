import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { CollectionPicker } from './CollectionPicker'

describe('CollectionPicker', () => {
  it('adds the current media to the selected private collection', async () => {
    let payload: unknown
    server.use(
      http.get('*/api/v1/collections', () => HttpResponse.json([
        { id: 'collection-1', name: '周末电影', items: [] },
      ])),
      http.post('*/api/v1/collections/:collectionID/items', async ({ request }) => {
        payload = await request.json()
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const user = userEvent.setup()
    renderWithQueryClient(<CollectionPicker mediaID="media-1" />)

    await user.selectOptions(await screen.findByLabelText('选择片单'), 'collection-1')
    await user.click(screen.getByRole('button', { name: '加入片单' }))

    expect(payload).toEqual({ mediaId: 'media-1' })
    expect(await screen.findByRole('status')).toHaveTextContent('已加入周末电影')
  })
})
