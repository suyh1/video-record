import { expect, test } from '@playwright/test'

import { admin, baseURL, login } from './support'

test('exports data and rehearses a real backup restore', async ({ page }, testInfo) => {
  test.slow()
  await login(page)
  await page.goto('/settings')
  const membersBeforeResponse = await page.context().request.get(`${baseURL}/api/v1/household/members`)
  expect(membersBeforeResponse.ok()).toBeTruthy()
  const membersBefore = await membersBeforeResponse.json() as Array<{ username: string }>
  const usernamesBefore = membersBefore.map((member) => member.username).sort()

  const exportDownload = page.waitForEvent('download')
  await page.getByRole('link', { name: '导出 JSON' }).click()
  await expect((await exportDownload).suggestedFilename()).toBe('video-record-export.json')

  await page.getByRole('button', { name: '创建系统备份' }).click()
  await expect(page.getByRole('region', { name: '备份与恢复' }).getByRole('status')).toContainText('备份已创建')
  const downloadLink = page.getByRole('link', { name: /^下载 / }).first()
  const backupDownload = page.waitForEvent('download')
  await downloadLink.click()
  const backup = await backupDownload
  const backupPath = testInfo.outputPath('rehearsal.vrbackup')
  await backup.saveAs(backupPath)

  await page.getByRole('button', { name: '添加成员' }).click()
  await page.getByLabel('用户名').fill('restore-probe')
  await page.getByLabel('初始密码').fill(admin.password)
  await page.getByRole('button', { name: '创建成员' }).click()
  await expect(page.getByText('restore-probe')).toBeVisible()

  await page.getByLabel('选择系统备份').setInputFiles(backupPath)
  await page.getByRole('button', { name: '恢复此备份' }).click()
  const confirm = page.getByRole('button', { name: '确认恢复' })
  await expect(confirm).toBeFocused()
  await confirm.click()
  await expect(page.getByRole('region', { name: '备份与恢复' }).getByRole('status')).toContainText('系统备份已恢复')
  await expect(page.getByText('restore-probe')).toHaveCount(0)
  const membersAfterResponse = await page.context().request.get(`${baseURL}/api/v1/household/members`)
  expect(membersAfterResponse.ok()).toBeTruthy()
  const membersAfter = await membersAfterResponse.json() as Array<{ username: string }>
  expect(membersAfter.map((member) => member.username).sort()).toEqual(usernamesBefore)
  await expect(page.getByText(`${membersBefore.length} 名成员`)).toBeVisible()
})
