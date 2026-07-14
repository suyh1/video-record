import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, LoaderCircle, Users } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

import { APIError, getRecordSharing, updateRecordSharing } from '../../api/client'

type RecordSharingEditorProps = {
  mediaID: string
  version: number
  onVersionChange: (version: number) => void
}

export function RecordSharingEditor({ mediaID, version, onVersionChange }: RecordSharingEditorProps) {
  const queryClient = useQueryClient()
  const reviewRef = useRef<HTMLTextAreaElement>(null)
  const query = useQuery({
    queryKey: ['record-sharing', mediaID],
    queryFn: ({ signal }) => getRecordSharing(mediaID, signal),
  })
  const [shareRating, setShareRating] = useState(false)
  const [shareReview, setShareReview] = useState(false)
  const [sharedReview, setSharedReview] = useState('')
  const [saved, setSaved] = useState(false)
  const [validationError, setValidationError] = useState('')
  const mutation = useMutation({
    mutationFn: () => updateRecordSharing(mediaID, {
      shareRating,
      shareReview,
      sharedReview: shareReview ? sharedReview.trim() : '',
      expectedVersion: Math.max(version, query.data?.version ?? version),
    }),
    onSuccess: (sharing) => {
      queryClient.setQueryData(['record-sharing', mediaID], sharing)
      onVersionChange(sharing.version)
      setSaved(true)
    },
    onError: () => setSaved(false),
  })

  useEffect(() => {
    if (!query.data) return
    setShareRating(query.data.shareRating)
    setShareReview(query.data.shareReview)
    setSharedReview(query.data.sharedReview ?? '')
  }, [query.data])

  if (query.isPending) return <div className="record-sharing-skeleton skeleton" aria-label="正在加载家庭公开设置" />
  if (query.isError) {
    if (query.error instanceof APIError && query.error.status === 404) {
      return <p className="quiet-empty">保存个人记录后可选择向家庭公开评分或短评。</p>
    }
    return <p className="record-sharing-error" role="alert">无法读取家庭公开设置</p>
  }

  const submit = (event: React.FormEvent) => {
    event.preventDefault()
    setSaved(false)
    if (shareReview && !sharedReview.trim()) {
      setValidationError('请输入要向家庭公开的短评')
      reviewRef.current?.focus()
      return
    }
    setValidationError('')
    mutation.mutate()
  }

  return (
    <form className="record-sharing-editor" onSubmit={submit}>
      <div className="record-sharing-heading">
        <Users aria-hidden="true" size={17} />
        <div><strong>家庭公开</strong><span>默认保持私有，只公开你主动选择的内容</span></div>
      </div>
      <label><input type="checkbox" checked={shareRating} onChange={(event) => setShareRating(event.target.checked)} />向家庭公开评分</label>
      <label><input type="checkbox" checked={shareReview} onChange={(event) => setShareReview(event.target.checked)} />向家庭公开短评</label>
      {shareReview ? (
        <label className="form-field">
          <span>家庭短评</span>
          <textarea
            ref={reviewRef}
            aria-label="家庭短评"
            rows={3}
            maxLength={500}
            value={sharedReview}
            onChange={(event) => setSharedReview(event.target.value)}
          />
        </label>
      ) : null}
      {validationError ? <p role="alert">{validationError}</p> : null}
      {mutation.isError ? <p role="alert">保存失败，公开选择和短评仍保留在此处。</p> : null}
      {saved ? <p role="status">家庭公开设置已保存</p> : null}
      <button type="submit" disabled={mutation.isPending}>
        {mutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <Check aria-hidden="true" size={16} />}
        {mutation.isPending ? '正在保存' : '保存家庭公开设置'}
      </button>
    </form>
  )
}
