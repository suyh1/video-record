import * as Dialog from '@radix-ui/react-dialog'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { CalendarDays, LoaderCircle, Trash2, X } from 'lucide-react'
import { useRef, useState } from 'react'

import { deleteWatchEvent } from '../../api/client'
import type { WatchEvent } from '../../api/types'

type WatchHistoryProps = {
  mediaID: string
  events: WatchEvent[]
}

export function WatchHistory({ mediaID, events }: WatchHistoryProps) {
  const queryClient = useQueryClient()
  const confirmButton = useRef<HTMLButtonElement>(null)
  const [target, setTarget] = useState<WatchEvent | null>(null)
  const mutation = useMutation({
    mutationFn: (eventID: string) => deleteWatchEvent(mediaID, eventID),
    onSuccess: (_, eventID) => {
      queryClient.setQueryData<WatchEvent[]>(
        ['watch-events', mediaID],
        (current) => current?.filter((event) => event.id !== eventID) ?? [],
      )
      void queryClient.invalidateQueries({ queryKey: ['record', mediaID] })
      void queryClient.invalidateQueries({ queryKey: ['episode-progress', mediaID] })
      setTarget(null)
    },
  })

  if (!events.length) return <p className="quiet-empty">还没有观看事件</p>

  return (
    <>
      <ol className="watch-history">
        {events.map((event) => {
          const date = formatDate(event.watchedAt)
          return (
            <li key={event.id}>
              <span className="history-icon"><CalendarDays aria-hidden="true" size={16} /></span>
              <div>
                <strong>{date}</strong>
                {event.viewingMethod ? <span>{event.viewingMethod}</span> : null}
              </div>
              <button
                className="watch-event-delete"
                type="button"
                aria-label={`删除 ${date}的观看事件`}
                onClick={() => setTarget(event)}
              >
                <Trash2 aria-hidden="true" size={17} />
              </button>
            </li>
          )
        })}
      </ol>
      <Dialog.Root open={Boolean(target)} onOpenChange={(open) => { if (!open && !mutation.isPending) setTarget(null) }}>
        <Dialog.Portal>
          <Dialog.Overlay className="dialog-backdrop" />
          <Dialog.Content
            className="member-dialog watch-event-dialog"
            onOpenAutoFocus={(event) => { event.preventDefault(); confirmButton.current?.focus() }}
          >
            <Dialog.Title>删除观看事件</Dialog.Title>
            <Dialog.Description>
              删除后会重新计算首次和最近观看日期。评分、笔记和标签不会被删除。
            </Dialog.Description>
            {target ? <p>{formatDate(target.watchedAt)}</p> : null}
            {mutation.isError ? <p className="watch-event-delete-error" role="alert">删除失败，观看事件仍然保留。</p> : null}
            <div className="dialog-actions">
              <Dialog.Close asChild><button type="button" disabled={mutation.isPending}><X aria-hidden="true" size={16} />取消</button></Dialog.Close>
              <button
                ref={confirmButton}
                type="button"
                disabled={!target || mutation.isPending}
                onClick={() => { if (target) mutation.mutate(target.id) }}
              >
                {mutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <Trash2 aria-hidden="true" size={16} />}
                {mutation.isPending ? '正在删除' : '确认删除观看事件'}
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </>
  )
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC',
  }).format(new Date(value))
}
