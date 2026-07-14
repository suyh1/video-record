import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { FolderPlus, LoaderCircle } from 'lucide-react'
import { useEffect, useState } from 'react'

import { addCollectionItem, getCollections } from '../../api/client'
import type { Collection } from '../../api/types'

export function CollectionPicker({ mediaID }: { mediaID: string }) {
  const queryClient = useQueryClient()
  const [collectionID, setCollectionID] = useState('')
  const [message, setMessage] = useState('')
  const collections = useQuery({ queryKey: ['collections'], queryFn: ({ signal }) => getCollections(signal) })
  useEffect(() => {
    if (!collectionID && collections.data?.[0]) setCollectionID(collections.data[0].id)
  }, [collectionID, collections.data])
  const selected = collections.data?.find((collection) => collection.id === collectionID)
  const alreadyAdded = selected?.items.includes(mediaID) ?? false
  const mutation = useMutation({
    mutationFn: () => addCollectionItem(collectionID, mediaID),
    onSuccess: () => {
      queryClient.setQueryData<Collection[]>(['collections'], (current = []) => current.map((collection) => (
        collection.id === collectionID && !collection.items.includes(mediaID)
          ? { ...collection, items: [...collection.items, mediaID] }
          : collection
      )))
      setMessage(`已加入${selected?.name ?? '片单'}`)
    },
    onError: () => setMessage('加入片单失败，当前选择已保留。'),
  })

  if (collections.isPending) return <div className="skeleton collection-picker-skeleton" aria-label="正在加载个人片单" />
  if (collections.isError) return <p className="collection-picker-error" role="alert">无法读取个人片单</p>
  if (collections.data.length === 0) return <p className="collection-picker-empty">还没有个人片单，可先在影库中创建。</p>

  return (
    <div className="collection-picker">
      <label>
        <span>选择片单</span>
        <select value={collectionID} onChange={(event) => { setCollectionID(event.target.value); setMessage('') }}>
          {collections.data.map((collection) => <option key={collection.id} value={collection.id}>{collection.name}</option>)}
        </select>
      </label>
      <button type="button" disabled={!collectionID || alreadyAdded || mutation.isPending} onClick={() => mutation.mutate()}>
        {mutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <FolderPlus aria-hidden="true" size={16} />}
        {mutation.isPending ? '正在加入' : alreadyAdded ? '已在片单中' : '加入片单'}
      </button>
      {message ? <p className={mutation.isError ? 'collection-picker-error' : 'collection-picker-message'} role={mutation.isError ? 'alert' : 'status'}>{message}</p> : null}
    </div>
  )
}
