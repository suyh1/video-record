import { useMutation } from '@tanstack/react-query'
import { Download, FileUp, LoaderCircle } from 'lucide-react'
import { useState } from 'react'

import { importData } from '../../api/client'
import type { ImportReport } from '../../api/types'

export function DataTransfer() {
  const [file, setFile] = useState<File | null>(null)
  const [report, setReport] = useState<ImportReport | null>(null)
  const mutation = useMutation({
    mutationFn: (selectedFile: File) => importData(selectedFile),
    onSuccess: (result) => setReport(result),
  })

  return (
    <section className="data-transfer" aria-labelledby="data-transfer-heading">
      <div className="data-transfer-heading">
        <h2 id="data-transfer-heading">数据迁移</h2>
        <div className="export-actions">
          <a href="/api/v1/data/export?format=json" download>
            <Download aria-hidden="true" size={16} />导出 JSON
          </a>
          <a href="/api/v1/data/export?format=csv" download>
            <Download aria-hidden="true" size={16} />导出 CSV
          </a>
        </div>
      </div>

      <form className="import-form" onSubmit={(event) => {
        event.preventDefault()
        if (file) mutation.mutate(file)
      }}>
        <label>
          <span>选择导入文件</span>
          <input
            type="file"
            accept=".json,.csv,application/json,text/csv"
            onChange={(event) => {
              setFile(event.target.files?.[0] ?? null)
              setReport(null)
            }}
          />
        </label>
        <button type="submit" disabled={!file || mutation.isPending}>
          {mutation.isPending
            ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} />
            : <FileUp aria-hidden="true" size={16} />}
          {mutation.isPending ? '正在导入' : '导入数据'}
        </button>
      </form>

      {mutation.isError ? <p className="data-transfer-error" role="alert">导入失败，已选择的文件仍保留。</p> : null}
      {report ? (
        <div className="import-report" role="status">
          <p>已导入 {report.importedRecords} 条记录，{report.importedCollections} 个片单</p>
          {report.failures.length > 0 ? (
            <ul>
              {report.failures.map((failure) => (
                <li key={`${failure.recordId}-${failure.code}`}>
                  {failure.recordId}：{failureLabel(failure.code)}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}
    </section>
  )
}

function failureLabel(code: string) {
  switch (code) {
    case 'external_identity_conflict': return '外部身份冲突'
    case 'collection_import_failed': return '片单导入失败'
    default: return '记录导入失败'
  }
}
