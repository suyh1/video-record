import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import {
  BarChart3,
  CalendarDays,
  Home,
  LibraryBig,
  Plus,
  Search,
  Settings,
  type LucideIcon,
} from 'lucide-react'
import { type FormEvent, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { BrowserRouter, Link, NavLink, Route, Routes, useLocation, useNavigate, useNavigationType } from 'react-router-dom'

import { getCurrentUser, getSetupStatus, getSyncStatus } from '../api/client'
import type { MediaSearchResult } from '../api/types'
import { BrandMark } from './BrandMark'
import { NotFoundPage } from './NotFoundPage'
import { CalendarPage } from '../features/calendar/CalendarPage'
import { AuthGate } from '../features/auth/AuthGate'
import { HomePage } from '../features/home/HomePage'
import type { HomeHeroBackdropState } from '../features/home/HomeHero'
import { HouseholdRecentEvents } from '../features/household/HouseholdRecentEvents'
import { MemberSettings } from '../features/household/MemberSettings'
import { LibraryPage } from '../features/library/LibraryPage'
import { MediaDetailsPage } from '../features/media/MediaDetailsPage'
import { TMDBPreviewPage } from '../features/media/TMDBPreviewPage'
import { SearchDialog } from '../features/search/SearchDialog'
import { TmdbStatus } from '../features/settings/TmdbStatus'
import { AccountSettings } from '../features/settings/AccountSettings'
import { DataTransfer } from '../features/settings/DataTransfer'
import { BackupRestore } from '../features/settings/BackupRestore'
import { IntegrationAccounts } from '../features/settings/IntegrationAccounts'
import { StatsPage } from '../features/stats/StatsPage'
import { CandidateReviewPage } from '../features/sync/CandidateReviewPage'
import { SyncStatus } from '../features/sync/SyncStatus'

type NavigationItem = {
  label: string
  path: string
  icon: LucideIcon
}

const navigationItems: NavigationItem[] = [
  { label: '首页', path: '/', icon: Home },
  { label: '影库', path: '/library', icon: LibraryBig },
  { label: '日历', path: '/calendar', icon: CalendarDays },
  { label: '统计', path: '/stats', icon: BarChart3 },
  { label: '设置', path: '/settings', icon: Settings },
]

export function App() {
  const [queryClient] = useState(() => new QueryClient({
    defaultOptions: {
      queries: {
        refetchOnWindowFocus: false,
        staleTime: 30_000,
      },
    },
  }))
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AuthGate>
          <ApplicationShell />
        </AuthGate>
      </BrowserRouter>
    </QueryClientProvider>
  )
}

function ApplicationShell() {
  const [searchOpen, setSearchOpen] = useState(false)
  const [headerScrolled, setHeaderScrolled] = useState(false)
  const [homeHeroBackdropState, setHomeHeroBackdropState] = useState<HomeHeroBackdropState>('loading')
  const searchTrigger = useRef<HTMLElement | null>(null)
  const restoreFocusTimer = useRef<number | null>(null)
  const restoringSearchFocus = useRef(false)
  const location = useLocation()
  const navigate = useNavigate()
  const navigationType = useNavigationType()
  const immersiveHeader = location.pathname === '/'
    || /^\/(?:media\/[^/]+|tmdb\/(?:movie|tv)\/\d+)\/?$/.test(location.pathname)
  const whiteHomeHeader = location.pathname === '/' && homeHeroBackdropState !== 'ready'
  const imageHomeHeader = location.pathname === '/' && homeHeroBackdropState === 'ready'

  useLayoutEffect(() => {
    if (location.pathname !== '/') setHomeHeroBackdropState('loading')
  }, [location.pathname])

  useLayoutEffect(() => {
    if (navigationType !== 'POP') window.scrollTo({ behavior: 'auto', left: 0, top: 0 })
    setHeaderScrolled(immersiveHeader && window.scrollY > 32)
  }, [immersiveHeader, location.pathname, navigationType])

  useEffect(() => {
    if (!immersiveHeader) {
      setHeaderScrolled(false)
      return
    }
    const updateHeader = () => setHeaderScrolled(window.scrollY > 32)
    updateHeader()
    window.addEventListener('scroll', updateHeader, { passive: true })
    return () => window.removeEventListener('scroll', updateHeader)
  }, [immersiveHeader, location.pathname])

  useEffect(() => () => {
    if (restoreFocusTimer.current !== null) window.clearTimeout(restoreFocusTimer.current)
  }, [])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (searchOpen) return
      const target = event.target
      if (target instanceof HTMLElement) {
        const tag = target.tagName
        if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable) return
      }
      const isModK = (event.key === 'k' || event.key === 'K') && (event.metaKey || event.ctrlKey)
      const isSlash = event.key === '/' && !event.metaKey && !event.ctrlKey && !event.altKey
      if (!isModK && !isSlash) return
      event.preventDefault()
      focusSearch()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [searchOpen])

  const focusSearch = (trigger?: HTMLElement) => {
    if (restoringSearchFocus.current) return
    const activeElement = document.activeElement
    searchTrigger.current = trigger
      ?? (activeElement instanceof HTMLElement && activeElement !== document.body ? activeElement : null)
    setSearchOpen(true)
  }

  const closeSearch = () => {
    const trigger = searchTrigger.current
    setSearchOpen(false)
    if (restoreFocusTimer.current !== null) window.clearTimeout(restoreFocusTimer.current)
    restoreFocusTimer.current = window.setTimeout(() => {
      restoreFocusTimer.current = null
      if (!trigger?.isConnected) return
      restoringSearchFocus.current = true
      try {
        trigger.focus()
      } finally {
        restoringSearchFocus.current = false
      }
    })
  }

  const submitSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    focusSearch()
  }

  const selectSearchResult = (item: MediaSearchResult) => {
    if (item.source === 'local') {
      setSearchOpen(false)
      navigate(`/media/${item.id}`)
      return
    }
    if (!item.externalId) throw new Error('TMDB identity required')
    setSearchOpen(false)
    navigate(`/tmdb/${item.mediaType}/${item.externalId}`)
  }

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        跳到主要内容
      </a>

      <header
        aria-label="应用导航"
        className={`app-header ${immersiveHeader ? 'immersive-header' : 'solid-header'}${headerScrolled ? ' is-scrolled' : ''}${whiteHomeHeader ? ' home-white-header' : ''}${imageHomeHeader ? ' home-image-header' : ''}`}
      >
        <div className="app-header-inner">
          <Brand />
          <PrimaryNavigation className="app-primary-navigation" />
          <form className="global-search" role="search" onSubmit={submitSearch}>
            <Search aria-hidden="true" size={18} strokeWidth={1.8} />
            <input
              type="search"
              aria-label="搜索影视"
              placeholder="搜索电影或剧集（⌘K）"
              readOnly
              onFocus={(event) => focusSearch(event.currentTarget)}
              onClick={(event) => focusSearch(event.currentTarget)}
            />
          </form>
          <button className="record-button" type="button" onClick={(event) => focusSearch(event.currentTarget)}>
            <Plus aria-hidden="true" size={18} strokeWidth={2} />
            <span>记录</span>
          </button>
        </div>
      </header>

      <main id="main-content" className={`main-content${immersiveHeader ? ' immersive-content' : ''}`} tabIndex={-1}>
        <Routes>
          <Route
            path="/"
            element={(
              <HomePage
                onHeroBackdropStateChange={setHomeHeroBackdropState}
                onSearch={() => focusSearch()}
              />
            )}
          />
          <Route path="/library" element={<LibraryPage onSearch={() => focusSearch()} />} />
          <Route path="/media/:mediaId" element={<MediaDetailsPage />} />
          <Route path="/tmdb/:mediaType/:tmdbId" element={<TMDBPreviewPage />} />
          <Route path="/calendar" element={<CalendarPage />} />
          <Route path="/stats" element={<StatsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/settings/sync" element={<CandidateReviewPage />} />
          <Route path="*" element={<NotFoundPage onSearch={() => focusSearch()} />} />
        </Routes>
      </main>

      <MobileNavigation searchOpen={searchOpen} onSearch={focusSearch} />
      <SearchDialog open={searchOpen} onClose={closeSearch} onSelect={selectSearchResult} />
    </div>
  )
}

function Brand() {
  return (
    <Link className="brand" to="/" aria-label="video-record 首页">
      <span className="brand-mark" aria-hidden="true">
        <BrandMark size={22} />
      </span>
      <span className="brand-name">video-record</span>
    </Link>
  )
}

function PrimaryNavigation({ className }: { className: string }) {
  const sync = useQuery({
    queryKey: ['sync-status'],
    queryFn: ({ signal }) => getSyncStatus(signal),
    staleTime: 60_000,
  })
  const pending = sync.data?.pendingTotal ?? 0
  return (
    <nav className={className} aria-label="主导航">
      {navigationItems.map(({ label, path, icon: Icon }) => (
        <NavLink key={path} className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`} to={path} end={path === '/'} title={label}>
          <Icon aria-hidden="true" size={20} strokeWidth={1.8} />
          <span>{label}</span>
          {path === '/settings' && pending > 0 ? (
            <span className="nav-badge" aria-label={`${pending} 条同步待核对`}>{pending > 99 ? '99+' : pending}</span>
          ) : null}
        </NavLink>
      ))}
    </nav>
  )
}

function MobileNavigation({ onSearch, searchOpen }: {
  onSearch: (trigger: HTMLButtonElement) => void
  searchOpen: boolean
}) {
  const leadingItems = navigationItems.slice(0, 2)
  const trailingItems = navigationItems.slice(2)

  return (
    <nav className="mobile-navigation" aria-label="移动导航">
      {leadingItems.map((item) => (
        <MobileNavigationLink key={item.path} item={item} />
      ))}
      <button
        aria-expanded={searchOpen}
        aria-haspopup="dialog"
        className="mobile-nav-link search-trigger"
        type="button"
        onClick={(event) => onSearch(event.currentTarget)}
      >
        <Search aria-hidden="true" size={20} strokeWidth={1.8} />
        <span>搜索</span>
      </button>
      {trailingItems.map((item) => (
        <MobileNavigationLink key={item.path} item={item} />
      ))}
    </nav>
  )
}

function MobileNavigationLink({ item }: { item: NavigationItem }) {
  const Icon = item.icon
  return (
    <NavLink className={({ isActive }) => `mobile-nav-link${isActive ? ' active' : ''}`} to={item.path} end={item.path === '/'}>
      <Icon aria-hidden="true" size={20} strokeWidth={1.8} />
      <span>{item.label}</span>
    </NavLink>
  )
}

function SettingsPage() {
  const setup = useQuery({ queryKey: ['setup-status'], queryFn: ({ signal }) => getSetupStatus(signal) })
  const currentUser = useQuery({ queryKey: ['current-user'], queryFn: ({ signal }) => getCurrentUser(signal) })
  return (
    <div className="page settings-page">
      <header className="page-heading">
        <p className="page-kicker">video-record</p>
        <h1>设置</h1>
      </header>
      <nav className="settings-section-navigation" aria-label="设置章节">
        <a href="#settings-account">账户</a>
        <a href="#settings-connections">TMDB 与媒体服务器</a>
        <a href="#settings-household">家庭成员</a>
        <a href="#settings-data">数据导入导出</a>
        <a href="#settings-backup">备份与恢复</a>
      </nav>
      <div className="settings-section-group" id="settings-account">
        <AccountSettings />
      </div>
      <div className="settings-section-group" id="settings-connections">
        <TmdbStatus configured={setup.data?.tmdbConfigured ?? false} />
        <IntegrationAccounts />
        <SyncStatus />
      </div>
      <div className="settings-section-group" id="settings-household">
        {currentUser.data?.role === 'member' ? (
          <SettingsPermissionNotice title="家庭成员">仅管理员可管理家庭成员。</SettingsPermissionNotice>
        ) : <MemberSettings />}
        <HouseholdRecentEvents />
      </div>
      <div className="settings-section-group" id="settings-data">
        <DataTransfer />
      </div>
      <div className="settings-section-group" id="settings-backup">
        {currentUser.data?.role === 'member' ? (
          <SettingsPermissionNotice title="备份与恢复">仅管理员可创建和恢复系统备份。</SettingsPermissionNotice>
        ) : <BackupRestore />}
      </div>
    </div>
  )
}

function SettingsPermissionNotice({ title, children }: { title: string; children: string }) {
  return (
    <section className="settings-permission-notice">
      <h2>{title}</h2>
      <p>{children}</p>
    </section>
  )
}
