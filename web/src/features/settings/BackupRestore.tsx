import * as Dialog from '@radix-ui/react-dialog'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Archive, Download, HardDriveUpload, LoaderCircle, RotateCcw, X } from 'lucide-react'
import { useRef, useState } from 'react'

import { createBackup, getBackups, getCurrentUser, restoreBackup } from '../../api/client'
import type { RestoreResult } from '../../api/types'

export function BackupRestore() {
  const queryClient = useQueryClient()
  const [file, setFile] = useState<File | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [message, setMessage] = useState('')
  const [restoreResult, setRestoreResult] = useState<RestoreResult | null>(null)
  const confirmButton = useRef<HTMLButtonElement>(null)
  const currentUser = useQuery({ queryKey: ['current-user'], queryFn: ({ signal }) => getCurrentUser(signal) })
  const backups = useQuery({
    queryKey: ['backups'],
    queryFn: ({ signal }) => getBackups(signal),
    enabled: currentUser.data?.role === 'admin',
  })
  const createMutation = useMutation({
    mutationFn: createBackup,
    onSuccess: () => {
      setMessage('备份已创建')
      void queryClient.invalidateQueries({ queryKey: ['backups'] })
    },
    onError: () => setMessage('创建备份失败，请稍后重试。'),
  })
  const restoreMutation = useMutation({
    mutationFn: (selectedFile: File) => restoreBackup(selectedFile),
    onSuccess: (result) => {
      setRestoreResult(result)
      setMessage('系统备份已恢复')
      setConfirmOpen(false)
      void queryClient.invalidateQueries()
    },
    onError: () => {
      setMessage('恢复失败，当前数据库保持不变，已选择的文件仍保留。')
      setConfirmOpen(false)
    },
  })

  if (currentUser.isPending || (currentUser.data?.role === 'admin' && backups.isPending)) {
    return <div className="backup-restore-skeleton skeleton" aria-label="正在加载系统备份" />
  }
  if (currentUser.isError || backups.isError) {
    return <p className="backup-restore-error" role="alert">无法读取系统备份</p>
  }
  if (currentUser.data.role !== 'admin') return null

  return (
    <section className="backup-restore" aria-labelledby="backup-restore-heading">
      <div className="backup-restore-heading">
        <div>
          <h2 id="backup-restore-heading">备份与恢复</h2>
          <p>系统备份包含数据库快照和校验清单，不包含环境变量或加密密钥。</p>
        </div>
        <button type="button" disabled={createMutation.isPending} onClick={() => createMutation.mutate()}>
          {createMutation.isPending
            ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} />
            : <Archive aria-hidden="true" size={16} />}
          {createMutation.isPending ? '正在创建' : '创建系统备份'}
        </button>
      </div>

      {message ? (
        <p
          className={createMutation.isError || restoreMutation.isError ? 'backup-restore-error' : 'backup-restore-message'}
          role={createMutation.isError || restoreMutation.isError ? 'alert' : 'status'}
        >
          {message}
        </p>
      ) : null}
      {restoreResult?.warnings.includes('integrations_locked') ? (
        <p className="backup-restore-warning" role="alert">集成凭据已锁定。配置原加密密钥后才能重新连接媒体服务器。</p>
      ) : null}

      {backups.data?.length ? (
        <ul className="backup-list" aria-label="系统备份">
          {backups.data.map((backup) => (
            <li key={backup.filename}>
              <HardDriveUpload aria-hidden="true" size={18} />
              <div>
                <strong>{formatDate(backup.manifest.createdAt)}</strong>
                <span>{formatBytes(backup.bytes)} · 数据库架构 {backup.manifest.schemaVersion}</span>
              </div>
              <a href={`/api/v1/backups/${encodeURIComponent(backup.filename)}`} download aria-label={`下载 ${backup.filename}`}>
                <Download aria-hidden="true" size={16} />
              </a>
            </li>
          ))}
        </ul>
      ) : <p className="backup-empty">还没有系统备份。创建第一份快照后可在这里下载。</p>}

      <form className="restore-form" onSubmit={(event) => {
        event.preventDefault()
        if (file) setConfirmOpen(true)
      }}>
        <label>
          <span>选择系统备份</span>
          <input
            type="file"
            accept=".vrbackup,application/vnd.video-record.backup"
            onChange={(event) => {
              setFile(event.target.files?.[0] ?? null)
              setRestoreResult(null)
              setMessage('')
            }}
          />
        </label>
        <button type="submit" disabled={!file || restoreMutation.isPending}>
          <RotateCcw aria-hidden="true" size={16} />恢复此备份
        </button>
      </form>

      <Dialog.Root open={confirmOpen} onOpenChange={setConfirmOpen}>
        <Dialog.Portal>
          <Dialog.Overlay className="dialog-backdrop" />
          <Dialog.Content
            className="member-dialog backup-dialog"
            onOpenAutoFocus={(event) => { event.preventDefault(); confirmButton.current?.focus() }}
          >
            <Dialog.Title>恢复系统备份</Dialog.Title>
            <Dialog.Description>
              恢复会短暂进入维护模式，并在替换数据库前自动创建当前数据库快照。此操作会用所选备份覆盖现有数据。
            </Dialog.Description>
            <p className="backup-dialog-file">{file?.name}</p>
            <div className="dialog-actions">
              <Dialog.Close asChild><button type="button"><X aria-hidden="true" size={16} />取消</button></Dialog.Close>
              <button
                ref={confirmButton}
                type="button"
                disabled={!file || restoreMutation.isPending}
                onClick={() => { if (file) restoreMutation.mutate(file) }}
              >
                {restoreMutation.isPending
                  ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} />
                  : <RotateCcw aria-hidden="true" size={16} />}
                {restoreMutation.isPending ? '正在恢复' : '确认恢复'}
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </section>
  )
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${(value / (1024 * 1024)).toFixed(1)} MiB`
}
