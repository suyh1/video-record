import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import { useState } from 'react'

import type { MediaSearchResult } from '../../api/types'
import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { CollectionManager } from './CollectionManager'

const mediaItems: MediaSearchResult[] = [
  { id: 'media-1', source: 'local', mediaType: 'movie', title: '花样年华', originalTitle: '', year: '2000', posterPath: null, status: 'completed' },
  { id: 'media-2', source: 'local', mediaType: 'movie', title: '一一', originalTitle: '', year: '2000', posterPath: null, status: 'completed' },
]

describe('CollectionManager', () => {
  it('keeps collection creation collapsed until requested and restores it after cancel', async () => {
    server.use(http.get('*/api/v1/collections', () => HttpResponse.json([])))
    const user = userEvent.setup()
    renderWithQueryClient(
      <CollectionManager mediaItems={mediaItems} selectedCollectionID="" onSelect={() => undefined} />,
    )

    expect(await screen.findByRole('button', { name: '创建片单' })).toHaveAttribute('title', '创建片单')
    expect(screen.queryByLabelText('片单名称')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '创建片单' }))
    expect(document.activeElement).toBe(screen.getByLabelText('片单名称'))
    await user.type(screen.getByLabelText('片单名称'), '稍后再建')
    await user.click(screen.getByRole('button', { name: '取消创建片单' }))

    expect(screen.queryByLabelText('片单名称')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '创建片单' })).toBeVisible()
  })

  it('creates, collapses, selects, and reorders a private collection', async () => {
    const collections = [{ id: 'collection-1', name: '周末电影', items: ['media-1', 'media-2'] }]
    let replaced: unknown
    server.use(
      http.get('*/api/v1/collections', () => HttpResponse.json(collections)),
      http.post('*/api/v1/collections', async ({ request }) => {
        expect(await request.json()).toEqual({ name: '家庭精选' })
        const created = { id: 'collection-2', name: '家庭精选', items: [] }
        collections.push(created)
        return HttpResponse.json(created, { status: 201 })
      }),
      http.put('*/api/v1/collections/:collectionID/items', async ({ request }) => {
        replaced = await request.json()
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const onSelect = vi.fn()
    const user = userEvent.setup()
    function Harness() {
      const [selectedCollectionID, setSelectedCollectionID] = useState('')
      return (
        <CollectionManager
          mediaItems={mediaItems}
          selectedCollectionID={selectedCollectionID}
          onSelect={(collectionID) => { onSelect(collectionID); setSelectedCollectionID(collectionID) }}
        />
      )
    }
    renderWithQueryClient(<Harness />)

    await user.click(await screen.findByRole('button', { name: '创建片单' }))
    await user.type(screen.getByLabelText('片单名称'), '家庭精选')
    await user.click(screen.getByRole('button', { name: '确认创建片单' }))
    expect(await screen.findByRole('button', { name: '家庭精选，0 部影视' })).toBeVisible()
    expect(screen.queryByLabelText('片单名称')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '家庭精选，0 部影视' }))
    expect(onSelect).toHaveBeenCalledWith('collection-2')

    await user.click(screen.getByRole('button', { name: '周末电影，2 部影视' }))
    expect(onSelect).toHaveBeenCalledWith('collection-1')

    const order = screen.getByRole('list', { name: '周末电影片单顺序' })
    await user.click(within(order).getByRole('button', { name: '上移 一一' }))
    await waitFor(() => expect(replaced).toEqual({ mediaIds: ['media-2', 'media-1'] }))
  })

  it('keeps the expanded form and name when collection creation fails', async () => {
    server.use(
      http.get('*/api/v1/collections', () => HttpResponse.json([])),
      http.post('*/api/v1/collections', () => HttpResponse.json({ code: 'conflict' }, { status: 409 })),
    )
    const user = userEvent.setup()
    renderWithQueryClient(
      <CollectionManager mediaItems={mediaItems} selectedCollectionID="" onSelect={() => undefined} />,
    )

    await user.click(await screen.findByRole('button', { name: '创建片单' }))
    await user.type(screen.getByLabelText('片单名称'), '重复片单')
    await user.click(screen.getByRole('button', { name: '确认创建片单' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('创建片单失败，名称已保留。')
    expect(screen.getByLabelText('片单名称')).toHaveValue('重复片单')
  })
})
