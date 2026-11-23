import React from 'react';
import { DisplayPreferences, formatDateTime } from '../api';

export type AppTab =
  | 'compose'
  | 'queue'
  | 'calendar'
  | 'drafts'
  | 'history'
  | 'accounts';

interface SidebarNavProps {
  activeTab: AppTab;
  onSelectTab: (tab: AppTab) => void;
  theme: 'light' | 'dark';
  onToggleTheme: () => void;
  authRequired: boolean;
  onLogout: () => void;
  accountsCount: number;
  queueCount: number;
  queuePaused: boolean;
  draftsCount: number;
  sentCount: number;
  nextSlot?: string;
  preferences?: DisplayPreferences | null;
}

export const SidebarNav: React.FC<SidebarNavProps> = ({
  activeTab,
  onSelectTab,
  theme,
  onToggleTheme,
  authRequired,
  onLogout,
  accountsCount,
  queueCount,
  queuePaused,
  draftsCount,
  sentCount,
  nextSlot,
  preferences,
}) => {
  const formatNextSlot = (iso?: string) => {
    if (!iso) return 'No slots';
    return formatDateTime(iso, preferences, true);
  };

  return (
    <aside className="sidebar-nav">
      <div className="sidebar-top">
        <div
          className="brand-group sidebar-brand"
          onClick={() => onSelectTab('compose')}
          role="button"
          tabIndex={0}
        >
          <div className="brand-badge" aria-hidden="true">
            <svg width="24" height="24" viewBox="0 0 32 32" fill="none">
              <path d="M25 7C14 7 7 13.5 7 25C18.5 25 25 18 25 7Z" fill="currentColor" />
              <path d="M10 22L20 12" stroke="var(--primary)" strokeWidth="2.8" strokeLinecap="round" />
            </svg>
          </div>
          <div>
            <span className="brand-title">Teepost</span>
          </div>
        </div>

        <button
          type="button"
          className="btn-primary sidebar-cta"
          onClick={() => onSelectTab('compose')}
        >
          + New Post
        </button>

        <nav className="sidebar-links" aria-label="Primary Navigation">
          <button
            type="button"
            className={`sidebar-link ${activeTab === 'compose' ? 'active' : ''}`}
            onClick={() => onSelectTab('compose')}
          >
            <span className="sidebar-link-main">
              <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M12 20h9" />
                <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
              </svg>
              Compose
            </span>
          </button>

          <button
            type="button"
            className={`sidebar-link ${activeTab === 'queue' ? 'active' : ''}`}
            onClick={() => onSelectTab('queue')}
          >
            <span className="sidebar-link-main">
              <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <line x1="8" y1="6" x2="21" y2="6" />
                <line x1="8" y1="12" x2="21" y2="12" />
                <line x1="8" y1="18" x2="21" y2="18" />
                <line x1="3" y1="6" x2="3.01" y2="6" />
                <line x1="3" y1="12" x2="3.01" y2="12" />
                <line x1="3" y1="18" x2="3.01" y2="18" />
              </svg>
              Queue
            </span>
            {queuePaused ? (
              <span className="pastel-pill peach">Paused</span>
            ) : (
              queueCount > 0 && <span className="nav-count">{queueCount}</span>
            )}
          </button>

          <button
            type="button"
            className={`sidebar-link ${activeTab === 'calendar' ? 'active' : ''}`}
            onClick={() => onSelectTab('calendar')}
          >
            <span className="sidebar-link-main">
              <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
                <line x1="16" y1="2" x2="16" y2="6" />
                <line x1="8" y1="2" x2="8" y2="6" />
                <line x1="3" y1="10" x2="21" y2="10" />
              </svg>
              Calendar
            </span>
          </button>

          <button
            type="button"
            className={`sidebar-link ${activeTab === 'drafts' ? 'active' : ''}`}
            onClick={() => onSelectTab('drafts')}
          >
            <span className="sidebar-link-main">
              <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
                <polyline points="14 2 14 8 20 8" />
              </svg>
              Drafts
            </span>
            {draftsCount > 0 && <span className="nav-count">{draftsCount}</span>}
          </button>

          <button
            type="button"
            className={`sidebar-link ${activeTab === 'history' ? 'active' : ''}`}
            onClick={() => onSelectTab('history')}
          >
            <span className="sidebar-link-main">
              <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <circle cx="12" cy="12" r="10" />
                <polyline points="12 6 12 12 16 14" />
              </svg>
              Sent
            </span>
            {sentCount > 0 && <span className="nav-count">{sentCount}</span>}
          </button>

          <button
            type="button"
            className={`sidebar-link ${activeTab === 'accounts' ? 'active' : ''}`}
            onClick={() => onSelectTab('accounts')}
          >
            <span className="sidebar-link-main">
              <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <circle cx="12" cy="12" r="3" />
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
              </svg>
              Settings
            </span>
            <span className="nav-count">{accountsCount}</span>
          </button>
        </nav>
      </div>

      <div className="sidebar-footer">
        {nextSlot && (
          <div className="sidebar-slot-box">
            <span className="sidebar-slot-label">
              {queuePaused ? 'Queue paused' : 'Next slot'}
            </span>
            <span className="sidebar-slot-val">{formatNextSlot(nextSlot)}</span>
          </div>
        )}

        <div className="sidebar-footer-actions">
          <button type="button" className="sidebar-theme-btn" onClick={onToggleTheme}>
            {theme === 'dark' ? 'Light' : 'Dark'}
          </button>
          {authRequired && (
            <button type="button" className="btn-ghost btn-xs" onClick={onLogout}>
              Sign out
            </button>
          )}
        </div>
      </div>
    </aside>
  );
};
