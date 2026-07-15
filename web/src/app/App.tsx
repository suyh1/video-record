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

import { createMediaFromTMDB, getSetupStatus } from '../api/client'
import type { MediaSearchResult } from '../api/types'
import { BrandMark } from './BrandMark'
import { NotFoundPage } from './NotFoundPage'
import { CalendarPage } from '../features/calendar/CalendarPage'
import { AuthGate } from '../features/auth/AuthGate'
import { HomePage } from '../features/home/HomePage'
import { MemberSettings } from '../features/household/MemberSettings'
import { LibraryPage } from '../features/library/LibraryPage'
import { MediaDetailsPage } from '../features/media/MediaDetailsPage'
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
  const searchTrigger = useRef<HTMLElement | null>(null)
  const restoreFocusTimer = useRef<number | null>(null)
  const restoringSearchFocus = useRef(false)
  const location = useLocation()
  const navigate = useNavigate()
  const navigationType = useNavigationType()
  const immersiveHeader = location.pathname === '/' || /^\/media\/[^/]+\/?$/.test(location.pathname)

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

  const selectSearchResult = async (item: MediaSearchResult) => {
    if (item.source === 'local') {
      setSearchOpen(false)
      navigate(`/media/${item.id}`)
      return
    }
    const media = await createMediaFromTMDB(item)
    setSearchOpen(false)
    navigate(`/media/${media.id}`)
  }

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        跳到主要内容
      </a>

      <header
        aria-label="应用导航"
        className={`app-header ${immersiveHeader ? 'immersive-header' : 'solid-header'}${headerScrolled ? ' is-scrolled' : ''}`}
      >
        <div className="app-header-inner">
          <Brand />
          <PrimaryNavigation className="app-primary-navigation" />
          <form className="global-search" role="search" onSubmit={submitSearch}>
            <Search aria-hidden="true" size={18} strokeWidth={1.8} />
            <input
              type="search"
              aria-label="搜索影视"
              placeholder="搜索电影或剧集"
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
          <Route path="/" element={<HomePage onSearch={() => focusSearch()} />} />
          <Route path="/library" element={<LibraryPage onSearch={() => focusSearch()} />} />
          <Route path="/media/:mediaId" element={<MediaDetailsPage />} />
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
  return (
    <nav className={className} aria-label="主导航">
      {navigationItems.map(({ label, path, icon: Icon }) => (
        <NavLink key={path} className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`} to={path} end={path === '/'} title={label}>
          <Icon aria-hidden="true" size={20} strokeWidth={1.8} />
          <span>{label}</span>
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
  return (
    <div className="page">
      <header className="page-heading">
        <p className="page-kicker">video-record</p>
        <h1>设置</h1>
      </header>
      <AccountSettings />
      <TmdbStatus configured={setup.data?.tmdbConfigured ?? false} />
      <IntegrationAccounts />
      <SyncStatus />
      <MemberSettings />
      <DataTransfer />
      <BackupRestore />
    </div>
  )
}
