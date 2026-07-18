import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, LoaderCircle, Tags } from 'lucide-react'
import { useEffect, useState } from 'react'

import { getRecordTags, getUserTags, setRecordTags } from '../../api/client'

type RecordTagsEditorProps = {
  mediaID: string
  version: number
  onVersionChange: (version: number) => void
}

export function RecordTagsEditor({ mediaID, version, onVersionChange }: RecordTagsEditorProps) {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: ['record-tags', mediaID],
    queryFn: ({ signal }) => getRecordTags(mediaID, signal),
  })
  const suggestions = useQuery({
    queryKey: ['user-tags'],
    queryFn: ({ signal }) => getUserTags(signal),
  })
  const [draft, setDraft] = useState('')
  const [saved, setSaved] = useState(false)
  const mutation = useMutation({
    mutationFn: (tags: string[]) => setRecordTags(mediaID, version, tags),
    onSuccess: (nextVersion, tags) => {
      queryClient.setQueryData(['record-tags', mediaID], { tags })
      void queryClient.invalidateQueries({ queryKey: ['user-tags'] })
      onVersionChange(nextVersion)
      setSaved(true)
    },
    onError: () => setSaved(false),
  })

  useEffect(() => {
    if (query.data) setDraft(query.data.tags.join(', '))
  }, [query.data])

  if (query.isPending) return <div className="record-tags-skeleton skeleton" aria-label="正在加载私人标签" />
  if (query.isError) return <p className="record-tags-error" role="alert">无法读取私人标签</p>

  const currentTags = normalizeTags(draft)
  const submit = (event: React.FormEvent) => {
    event.preventDefault()
    setSaved(false)
    mutation.mutate(currentTags)
  }
  const appendTag = (name: string) => {
    if (currentTags.includes(name)) return
    setDraft([...currentTags, name].join('，'))
  }

  return (
    <form className="record-tags-editor" onSubmit={submit}>
      <label className="form-field">
        <span><Tags aria-hidden="true" size={16} />私人标签</span>
        <input
          aria-label="私人标签"
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="用逗号分隔，如：家庭、科幻"
        />
      </label>
      {suggestions.data?.tags.length ? (
        <div className="record-tag-suggestions" role="group" aria-label="已有标签">
          {suggestions.data.tags.map((name) => (
            <button key={name} type="button" onClick={() => appendTag(name)}>{name}</button>
          ))}
        </div>
      ) : null}
      <button type="submit" disabled={mutation.isPending}>
        {mutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <Check aria-hidden="true" size={16} />}
        {mutation.isPending ? '正在保存' : '保存标签'}
      </button>
      {mutation.isError ? <p role="alert">保存失败，当前标签仍保留在输入框中。</p> : null}
      {saved ? <p role="status">标签已保存</p> : null}
    </form>
  )
}

function normalizeTags(value: string) {
  return [...new Set(value.split(/[,，]/).map((tag) => tag.trim()).filter(Boolean))]
}
