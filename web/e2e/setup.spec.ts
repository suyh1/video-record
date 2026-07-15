import { expect, test } from '@playwright/test'

import { admin, baseURL, expectNoBlockingA11yViolations, seedLibrary } from './support'

test('initializes the closed installation and supports a fresh login', async ({ page }) => {
  await page.goto('/')

  await expect(page.getByRole('heading', { name: '开始使用 video-record' })).toBeVisible()
  await expect(page.getByText('数据存储已就绪')).toBeVisible()
  await expect(page.getByText(/TMDB/)).toBeVisible()
  await expectNoBlockingA11yViolations(page)

  await page.keyboard.press('Tab')
  await expect(page.getByLabel('管理员用户名')).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(page.getByLabel('管理员密码', { exact: true })).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(page.getByRole('button', { name: '显示管理员密码' })).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(page.getByLabel('确认密码', { exact: true })).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(page.getByRole('button', { name: '显示确认密码' })).toBeFocused()

  let releaseInitialization!: () => void
  let markInitializationStarted!: () => void
  const initializationStarted = new Promise<void>((resolve) => {
    markInitializationStarted = resolve
  })
  const initializationHeld = new Promise<void>((resolve) => {
    releaseInitialization = resolve
  })
  await page.route('**/api/v1/setup/admin', async (route) => {
    markInitializationStarted()
    await initializationHeld
    await route.continue()
  }, { times: 1 })

  await page.getByLabel('管理员用户名').fill(admin.username)
  await page.getByLabel('管理员密码', { exact: true }).fill(admin.password)
  await page.getByLabel('确认密码', { exact: true }).fill(admin.password)
  const setupSubmit = page.getByRole('button', { name: '创建管理员' })
  const initialSubmitHeight = await setupSubmit.evaluate((element) => element.getBoundingClientRect().height)
  await setupSubmit.click()
  await initializationStarted
  const pendingSubmit = page.getByRole('button', { name: '正在创建' })
  await expect(pendingSubmit).toBeDisabled()
  await expect(pendingSubmit).toHaveAttribute('aria-busy', 'true')
  await expect.poll(() => pendingSubmit.evaluate((element) => element.getBoundingClientRect().height)).toBe(initialSubmitHeight)
  releaseInitialization()

  await expect(page.getByRole('navigation', { name: '主导航' })).toBeVisible()
  const csrfToken = await page.evaluate(() => window.sessionStorage.getItem('video-record.csrf-token'))
  expect(csrfToken).toBeTruthy()
  await seedLibrary(page, csrfToken ?? '')
  const seededLibrary = await page.context().request.get(`${baseURL}/api/v1/library`)
  expect(seededLibrary.ok()).toBeTruthy()
  await expect(seededLibrary.json()).resolves.toMatchObject({
    items: [{ id: 'e2e-movie' }, { id: 'e2e-series' }],
  })

  await page.context().clearCookies()
  await page.evaluate(() => window.sessionStorage.clear())
  await page.reload()
  await expect(page.getByRole('heading', { name: '登录 video-record' })).toBeVisible()
  await expectNoBlockingA11yViolations(page)
  await page.keyboard.press('Tab')
  await expect(page.getByLabel('用户名')).toBeFocused()

  await page.getByLabel('用户名').fill(admin.username)
  await page.getByLabel('密码', { exact: true }).fill('wrong-synthetic-password')
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page.getByRole('alert')).toContainText('用户名或密码不正确')

  await page.getByLabel('密码', { exact: true }).fill(admin.password)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page.getByRole('navigation', { name: '主导航' })).toBeVisible()

  const status = await page.context().request.get(`${baseURL}/api/v1/setup/status`)
  await expect(status.json()).resolves.toMatchObject({ initialized: true })
})
