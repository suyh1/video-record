import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, Check, Folder, LoaderCircle, Plus, RefreshCw, X } from 'lucide-react'
import { type FormEvent, useState } from 'react'

import { createCollection, getCollections, replaceCollectionItems } from '../../api/client'
import type { Collection, MediaSearchResult } from '../../api/types'

type CollectionManagerProps = {
  mediaItems: MediaSearchResult[]
  selectedCollectionID: string
  onSelect: (collectionID: string) => void
}

export function CollectionManager({ mediaItems, selectedCollectionID, onSelect }: CollectionManagerProps) {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [message, setMessage] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const collections = useQuery({ queryKey: ['collections'], queryFn: ({ signal }) => getCollections(signal) })
  const createMutation = useMutation({
    mutationFn: () => createCollection(name),
    onSuccess: (created) => {
      queryClient.setQueryData<Collection[]>(['collections'], (current = []) => [...current, created])
      setName('')
      setCreateOpen(false)
      setMessage(`已创建${created.name}`)
    },
    onError: () => setMessage('创建片单失败，名称已保留。'),
  })
  const replaceMutation = useMutation({
    mutationFn: ({ collectionID, mediaIDs }: { collectionID: string; mediaIDs: string[] }) => (
      replaceCollectionItems(collectionID, mediaIDs)
    ),
    onSuccess: (_, variables) => {
      queryClient.setQueryData<Collection[]>(['collections'], (current = []) => current.map((collection) => (
        collection.id === variables.collectionID ? { ...collection, items: variables.mediaIDs } : collection
      )))
      setMessage('片单顺序已保存')
    },
    onError: () => setMessage('片单更新失败，请稍后重试。'),
  })

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!name.trim() || createMutation.isPending) return
    setMessage('')
    createMutation.mutate()
  }
  const selected = collections.data?.find((collection) => collection.id === selectedCollectionID)
  const mediaByID = new Map(mediaItems.map((item) => [item.id, item]))
  const replace = (mediaIDs: string[]) => {
    if (selected) replaceMutation.mutate({ collectionID: selected.id, mediaIDs })
  }
  const closeCreate = () => {
    setCreateOpen(false)
    setName('')
    setMessage('')
    createMutation.reset()
  }

  return (
    <section className="collection-manager" aria-labelledby="collections-heading">
      <div className="collection-manager-heading">
        <div>
          <h2 id="collections-heading">个人片单</h2>
          <p>片单只对当前账户可见</p>
        </div>
      </div>

      {collections.isPending ? <div className="skeleton collection-manager-skeleton" aria-label="正在加载个人片单" /> : null}
      {collections.isError ? (
        <div className="collection-manager-error" role="alert">
          <span>无法读取个人片单</span>
          <button type="button" onClick={() => void collections.refetch()}><RefreshCw aria-hidden="true" size={16} />重试</button>
        </div>
      ) : null}
      <div className="collection-strip">
        {collections.data ? (
          <div className="collection-tabs" role="group" aria-label="个人片单筛选">
            <button type="button" aria-pressed={!selectedCollectionID} onClick={() => onSelect('')}>全部记录</button>
            {collections.data.map((collection) => (
              <button
                key={collection.id}
                type="button"
                aria-pressed={selectedCollectionID === collection.id}
                aria-label={`${collection.name}，${collection.items.length} 部影视`}
                onClick={() => onSelect(collection.id)}
              >
                <Folder aria-hidden="true" size={16} />
                <span>{collection.name}</span>
                <small>{collection.items.length}</small>
              </button>
            ))}
          </div>
        ) : null}
        {!createOpen ? (
          <button
            className="icon-button collection-create-trigger"
            type="button"
            aria-label="创建片单"
            title="创建片单"
            aria-expanded="false"
            onClick={() => {
              setCreateOpen(true)
              setMessage('')
              createMutation.reset()
            }}
          >
            <Plus aria-hidden="true" size={18} />
          </button>
        ) : null}
      </div>

      {createOpen ? (
        <form className="collection-create-form" onSubmit={submit}>
          <label>
            <span>片单名称</span>
            <input autoFocus value={name} maxLength={100} onChange={(event) => setName(event.target.value)} />
          </label>
          <button type="submit" disabled={!name.trim() || createMutation.isPending}>
            {createMutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <Check aria-hidden="true" size={16} />}
            {createMutation.isPending ? '正在创建' : '确认创建片单'}
          </button>
          <button type="button" aria-label="取消创建片单" title="取消创建片单" onClick={closeCreate}>
            <X aria-hidden="true" size={18} />
          </button>
        </form>
      ) : null}

      {selected && selected.items.length > 0 ? (
        <ol className="collection-order" aria-label={`${selected.name}片单顺序`}>
          {selected.items.map((mediaID, index) => {
            const item = mediaByID.get(mediaID)
            if (!item) return null
            return (
              <li key={mediaID}>
                <span>{index + 1}</span>
                <strong>{item.title}</strong>
                <button
                  type="button"
                  aria-label={`上移 ${item.title}`}
                  title="上移"
                  disabled={index === 0 || replaceMutation.isPending}
                  onClick={() => replace(moveItem(selected.items, index, index - 1))}
                ><ArrowUp aria-hidden="true" size={16} /></button>
                <button
                  type="button"
                  aria-label={`下移 ${item.title}`}
                  title="下移"
                  disabled={index === selected.items.length - 1 || replaceMutation.isPending}
                  onClick={() => replace(moveItem(selected.items, index, index + 1))}
                ><ArrowDown aria-hidden="true" size={16} /></button>
                <button
                  type="button"
                  aria-label={`从片单移除 ${item.title}`}
                  title="从片单移除"
                  disabled={replaceMutation.isPending}
                  onClick={() => replace(selected.items.filter((id) => id !== mediaID))}
                ><X aria-hidden="true" size={16} /></button>
              </li>
            )
          })}
        </ol>
      ) : null}
      {selected && selected.items.length === 0 ? <p className="collection-manager-empty">这个片单还没有影视</p> : null}
      {message ? <p className={createMutation.isError || replaceMutation.isError ? 'collection-manager-error-text' : 'collection-manager-message'} role={createMutation.isError || replaceMutation.isError ? 'alert' : 'status'}>{message}</p> : null}
    </section>
  )
}

function moveItem(items: string[], from: number, to: number) {
  const reordered = [...items]
  const [item] = reordered.splice(from, 1)
  if (item) reordered.splice(to, 0, item)
  return reordered
}
