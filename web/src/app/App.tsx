import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  BarChart3,
  CalendarDays,
  Clapperboard,
  Home,
  LibraryBig,
  Plus,
  Search,
  Settings,
  type LucideIcon,
} from 'lucide-react'
import { type FormEvent, useRef, useState } from 'react'
import { BrowserRouter, NavLink, Route, Routes, useNavigate } from 'react-router-dom'

import { createMediaFromTMDB } from '../api/client'
import type { MediaSearchResult } from '../api/types'
import { CalendarPage } from '../features/calendar/CalendarPage'
import { MemberSettings } from '../features/household/MemberSettings'
import { LibraryPage } from '../features/library/LibraryPage'
import { MediaDetailsPage } from '../features/media/MediaDetailsPage'
import { SearchDialog } from '../features/search/SearchDialog'
import { TmdbAttribution } from '../features/settings/TmdbStatus'
import { StatsPage } from '../features/stats/StatsPage'

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

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
})

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <ApplicationShell />
      </BrowserRouter>
    </QueryClientProvider>
  )
}

function ApplicationShell() {
  const searchInput = useRef<HTMLInputElement>(null)
  const [searchOpen, setSearchOpen] = useState(false)
  const navigate = useNavigate()

  const focusSearch = () => {
    setSearchOpen(true)
  }

  const submitSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setSearchOpen(true)
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

      <aside className="sidebar">
        <Brand />
        <PrimaryNavigation className="sidebar-navigation" />
      </aside>

      <div className="app-column">
        <header className="topbar">
          <div className="mobile-brand" aria-hidden="true">
            <Clapperboard size={22} strokeWidth={1.8} />
          </div>
          <form className="global-search" role="search" onSubmit={submitSearch}>
            <Search aria-hidden="true" size={18} strokeWidth={1.8} />
            <input
              ref={searchInput}
              type="search"
              aria-label="搜索影视"
              placeholder="搜索电影或剧集"
              readOnly
              onFocus={() => setSearchOpen(true)}
              onClick={() => setSearchOpen(true)}
            />
          </form>
          <button className="record-button" type="button" onClick={() => setSearchOpen(true)}>
            <Plus aria-hidden="true" size={18} strokeWidth={2} />
            <span>记录</span>
          </button>
        </header>

        <main id="main-content" className="main-content" tabIndex={-1}>
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/library" element={<LibraryPage onSearch={() => setSearchOpen(true)} />} />
            <Route path="/media/:mediaId" element={<MediaDetailsPage />} />
            <Route path="/calendar" element={<CalendarPage />} />
            <Route path="/stats" element={<StatsPage />} />
            <Route path="/settings" element={<SettingsPage />} />
          </Routes>
        </main>
      </div>

      <MobileNavigation onSearch={focusSearch} />
      <SearchDialog open={searchOpen} onClose={() => setSearchOpen(false)} onSelect={selectSearchResult} />
    </div>
  )
}

function Brand() {
  return (
    <div className="brand" aria-label="video-record">
      <span className="brand-mark" aria-hidden="true">
        <Clapperboard size={22} strokeWidth={1.8} />
      </span>
      <span className="brand-name">video-record</span>
    </div>
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

function MobileNavigation({ onSearch }: { onSearch: () => void }) {
  const leadingItems = navigationItems.slice(0, 2)
  const trailingItems = navigationItems.slice(2)

  return (
    <nav className="mobile-navigation" aria-label="移动导航">
      {leadingItems.map((item) => (
        <MobileNavigationLink key={item.path} item={item} />
      ))}
      <button className="mobile-nav-link search-trigger" type="button" onClick={onSearch}>
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

function HomePage() {
  return (
    <div className="page home-page">
      <header className="page-heading">
        <p className="page-kicker">私人影库</p>
        <h1>首页</h1>
      </header>

      <section className="content-section" aria-labelledby="continue-heading">
        <div className="section-heading">
          <div>
            <h2 id="continue-heading">继续观看</h2>
            <p>0 部剧集</p>
          </div>
        </div>
        <div className="empty-state">
          <Clapperboard aria-hidden="true" size={24} strokeWidth={1.6} />
          <p>还没有正在观看的剧集</p>
          <NavLink className="text-link" to="/library">
            前往影库
          </NavLink>
        </div>
      </section>

      <section className="content-section" aria-labelledby="recent-heading">
        <div className="section-heading">
          <div>
            <h2 id="recent-heading">最近记录</h2>
            <p>按观看时间排列</p>
          </div>
        </div>
        <div className="timeline-empty">
          <span aria-hidden="true" />
          <p>第一条观影记录会显示在这里</p>
        </div>
      </section>
    </div>
  )
}

function PlaceholderPage({ title }: { title: string }) {
  return (
    <div className="page">
      <header className="page-heading">
        <p className="page-kicker">video-record</p>
        <h1>{title}</h1>
      </header>
    </div>
  )
}

function SettingsPage() {
  return (
    <div className="page">
      <header className="page-heading">
        <p className="page-kicker">video-record</p>
        <h1>设置</h1>
      </header>
      <section className="integration-status" aria-labelledby="metadata-heading">
        <h2 id="metadata-heading">元数据</h2>
        <TmdbAttribution />
      </section>
      <MemberSettings />
    </div>
  )
}
