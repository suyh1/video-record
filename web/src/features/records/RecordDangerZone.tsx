import * as Dialog from '@radix-ui/react-dialog'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Eraser, LoaderCircle, Trash2, X } from 'lucide-react'
import { useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { clearCurrentRoundFields, removeFromLibrary } from '../../api/client'
import type { CurrentRound } from '../../api/types'

type RecordDangerZoneProps = {
  mediaID: string
  round: CurrentRound
  onRoundChange: (round: CurrentRound) => void
}

type ConfirmKind = 'clear' | 'remove' | null

export function RecordDangerZone({ mediaID, round, onRoundChange }: RecordDangerZoneProps) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [confirm, setConfirm] = useState<ConfirmKind>(null)
  const [message, setMessage] = useState('')
  const confirmButton = useRef<HTMLButtonElement>(null)
  const seasonNumber = round.seasonNumber ?? undefined

  const clearMutation = useMutation({
    mutationFn: () => clearCurrentRoundFields(mediaID, seasonNumber, round.version),
    onSuccess: (next) => {
      onRoundChange(next)
      setConfirm(null)
      setMessage('已清空评分、笔记和观看方式')
      void queryClient.invalidateQueries({ queryKey: ['record', mediaID] })
    },
    onError: () => setMessage('清空失败，请稍后重试'),
  })

  const removeMutation = useMutation({
    mutationFn: () => removeFromLibrary(mediaID),
    onSuccess: () => {
      setConfirm(null)
      setMessage('已从影库移除')
      void queryClient.invalidateQueries({ queryKey: ['library'] })
      void queryClient.invalidateQueries({ queryKey: ['record', mediaID] })
      void queryClient.invalidateQueries({ queryKey: ['current-round', mediaID] })
      void navigate('/library')
    },
    onError: () => setMessage('移出影库失败，请稍后重试'),
  })

  const pending = clearMutation.isPending || removeMutation.isPending
  const hasOptional = round.rating !== null || Boolean(round.note || round.viewingMethod)

  return (
    <section className="record-danger-zone" aria-labelledby="record-danger-heading">
      <div className="record-danger-heading">
        <h3 id="record-danger-heading">危险操作</h3>
        <p>高风险操作需二次确认；不会删除媒体实体或他人记录。</p>
      </div>
      <div className="record-danger-actions">
        <button type="button" disabled={!hasOptional || pending} onClick={() => { setMessage(''); setConfirm('clear') }}>
          <Eraser aria-hidden="true" size={16} />
          清空可选字段
        </button>
        <button type="button" disabled={pending} onClick={() => { setMessage(''); setConfirm('remove') }}>
          <Trash2 aria-hidden="true" size={16} />
          移出影库
        </button>
      </div>
      {message ? <p role="status">{message}</p> : null}

      <Dialog.Root open={confirm !== null} onOpenChange={(open) => { if (!open) setConfirm(null) }}>
        <Dialog.Portal>
          <Dialog.Overlay className="dialog-backdrop" />
          <Dialog.Content
            className="member-dialog"
            onOpenAutoFocus={(event) => { event.preventDefault(); confirmButton.current?.focus() }}
          >
            <Dialog.Title>{confirm === 'remove' ? '移出影库' : '清空可选字段'}</Dialog.Title>
            <Dialog.Description>
              {confirm === 'remove'
                ? '将当前观看状态重置为未记录，本片会从影库列表消失。媒体条目、已归档多刷与其他成员记录不会删除。'
                : '将清空当前轮次的评分、私人笔记和观看方式，状态与完成时间保持不变。'}
            </Dialog.Description>
            <div className="dialog-actions">
              <Dialog.Close asChild>
                <button type="button"><X aria-hidden="true" size={16} />取消</button>
              </Dialog.Close>
              <button
                ref={confirmButton}
                type="button"
                disabled={pending}
                onClick={() => {
                  if (confirm === 'clear') clearMutation.mutate()
                  if (confirm === 'remove') removeMutation.mutate()
                }}
              >
                {pending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <Trash2 aria-hidden="true" size={16} />}
                {pending ? '处理中' : '确认'}
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </section>
  )
}
