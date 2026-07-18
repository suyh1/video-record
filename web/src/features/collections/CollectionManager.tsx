import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, Check, Folder, LoaderCircle, Plus, RefreshCw, X } from 'lucide-react'
import { type FormEvent, useId, useRef, useState } from 'react'

import { createCollection, deleteCollection, getCollections, renameCollection, replaceCollectionItems } from '../../api/client'
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
  const createFormID = useId()
  const createTriggerRef = useRef<HTMLButtonElement>(null)
  const collections = useQuery({ queryKey: ['collections'], queryFn: ({ signal }) => getCollections(signal) })
  const createMutation = useMutation({
    mutationFn: () => createCollection(name),
    onSuccess: (created) => {
      queryClient.setQueryData<Collection[]>(['collections'], (current = []) => [...current, created])
      setName('')
      setCreateOpen(false)
      setMessage(`已创建${created.name}`)
      createTriggerRef.current?.focus()
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
  const renameMutation = useMutation({
    mutationFn: ({ collectionID, nextName }: { collectionID: string; nextName: string }) => (
      renameCollection(collectionID, nextName)
    ),
    onSuccess: (updated) => {
      queryClient.setQueryData<Collection[]>(['collections'], (current = []) => current.map((collection) => (
        collection.id === updated.id ? { ...collection, name: updated.name } : collection
      )))
      setMessage(`已重命名为${updated.name}`)
    },
    onError: () => setMessage('重命名失败，请检查名称是否重复。'),
  })
  const deleteMutation = useMutation({
    mutationFn: (collectionID: string) => deleteCollection(collectionID),
    onSuccess: (_, collectionID) => {
      queryClient.setQueryData<Collection[]>(['collections'], (current = []) => (
        current.filter((collection) => collection.id !== collectionID)
      ))
      if (selectedCollectionID === collectionID) onSelect('')
      setMessage('片单已删除')
    },
    onError: () => setMessage('删除片单失败，请稍后重试。'),
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
    createTriggerRef.current?.focus()
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
        <button
          ref={createTriggerRef}
          className="icon-button collection-create-trigger"
          type="button"
          aria-label="创建片单"
          title="创建片单"
          aria-expanded={createOpen}
          aria-controls={createFormID}
          onClick={() => {
            if (createOpen) {
              closeCreate()
            } else {
              setCreateOpen(true)
              setMessage('')
              createMutation.reset()
            }
          }}
        >
          <Plus aria-hidden="true" size={18} />
        </button>
      </div>

      {createOpen ? (
        <form id={createFormID} className="collection-create-form" onSubmit={submit}>
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
      {selected ? (
        <div className="collection-manage-actions">
          <button
            type="button"
            disabled={renameMutation.isPending || deleteMutation.isPending}
            onClick={() => {
              const nextName = window.prompt('新的片单名称', selected.name)?.trim()
              if (!nextName || nextName === selected.name) return
              renameMutation.mutate({ collectionID: selected.id, nextName })
            }}
          >
            重命名
          </button>
          <button
            type="button"
            disabled={renameMutation.isPending || deleteMutation.isPending}
            onClick={() => {
              if (!window.confirm(`确定删除片单「${selected.name}」？影视记录不会被删除。`)) return
              deleteMutation.mutate(selected.id)
            }}
          >
            删除片单
          </button>
        </div>
      ) : null}
      {selected && selected.items.length === 0 ? <p className="collection-manager-empty">这个片单还没有影视</p> : null}
      {message ? <p className={createMutation.isError || replaceMutation.isError || renameMutation.isError || deleteMutation.isError ? 'collection-manager-error-text' : 'collection-manager-message'} role={createMutation.isError || replaceMutation.isError || renameMutation.isError || deleteMutation.isError ? 'alert' : 'status'}>{message}</p> : null}
    </section>
  )
}

function moveItem(items: string[], from: number, to: number) {
  const reordered = [...items]
  const [item] = reordered.splice(from, 1)
  if (item) reordered.splice(to, 0, item)
  return reordered
}
