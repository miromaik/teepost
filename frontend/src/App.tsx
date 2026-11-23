import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Account,
  api,
  AuthStatus,
  DisplayPreferences,
  HashtagGroup,
  MediaItem,
  PlatformId,
  Post,
  ScheduleSlotsResponse,
} from './api';
import { AccountsSettingsView } from './components/AccountsSettingsView';
import { CalendarView } from './components/CalendarView';
import { ComposerWorkspace } from './components/ComposerWorkspace';
import { DraftsIdeasView } from './components/DraftsIdeasView';
import { HistoryView } from './components/HistoryView';
import { QueueView } from './components/QueueView';
import { AppTab, SidebarNav } from './components/SidebarNav';

function getInitialTheme(): 'light' | 'dark' {
  const saved = localStorage.getItem('teepost-theme');
  if (saved === 'light' || saved === 'dark') return saved;
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export const App: React.FC = () => {
  const [theme, setTheme] = useState<'light' | 'dark'>(getInitialTheme);
  const [authStatus, setAuthStatus] = useState<AuthStatus | null>(null);
  const [loginPassword, setLoginPassword] = useState('');
  const [loginError, setLoginError] = useState<string | null>(null);
  const [loginLoading, setLoginLoading] = useState(false);

  const [activeTab, setActiveTab] = useState<AppTab>('compose');
  const [preselectPlatform, setPreselectPlatform] = useState<PlatformId | null>(null);

  const [accounts, setAccounts] = useState<Account[]>([]);
  const [posts, setPosts] = useState<Post[]>([]);
  const [scheduleInfo, setScheduleInfo] = useState<ScheduleSlotsResponse | null>(null);
  const [hashtagGroups, setHashtagGroups] = useState<HashtagGroup[]>([]);
  const [preferences, setPreferences] = useState<DisplayPreferences>({
    timezone: 'Local',
    timeFormat: '24h',
  });
  const [loadingData, setLoadingData] = useState(false);

  const [editingPostId, setEditingPostId] = useState<string | null>(null);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [masterText, setMasterText] = useState('');
  const [firstComment, setFirstComment] = useState('');
  const [customizePerNetwork, setCustomizePerNetwork] = useState(false);
  const [customTexts, setCustomTexts] = useState<Record<string, string>>({});
  const [media, setMedia] = useState<MediaItem[]>([]);
  const [tags, setTags] = useState<string[]>([]);

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem('teepost-theme', theme);
  }, [theme]);

  const loadWorkspaceData = useCallback(async () => {
    setLoadingData(true);
    try {
      const [fetchedAccounts, fetchedPosts, fetchedSchedule, fetchedGroups, fetchedPrefs] =
        await Promise.all([
          api.listAccounts(),
          api.listPosts(),
          api.getScheduleSlots(),
          api.listHashtagGroups(),
          api.getPreferences(),
        ]);
      setAccounts(fetchedAccounts);
      setPosts(fetchedPosts);
      setScheduleInfo(fetchedSchedule);
      setHashtagGroups(fetchedGroups);
      setPreferences(fetchedPrefs);
      setSelectedIds((prev) => {
        if (prev.length > 0) {
          return prev.filter((id) =>
            fetchedAccounts.some((a) => a.id === id && a.enabled)
          );
        }
        return fetchedAccounts.filter((a) => a.enabled).map((a) => a.id);
      });
    } finally {
      setLoadingData(false);
    }
  }, []);

  const checkAuthAndLoad = useCallback(async () => {
    try {
      const status = await api.getAuthStatus();
      setAuthStatus(status);
      if (status.authenticated || !status.authRequired) {
        await loadWorkspaceData();
      }
    } catch {
      setAuthStatus({ authRequired: false, authenticated: true });
    }
  }, [loadWorkspaceData]);

  useEffect(() => {
    void checkAuthAndLoad();
  }, [checkAuthAndLoad]);

  useEffect(() => {
    if (!authStatus?.authenticated && authStatus?.authRequired) return;
    if (activeTab !== 'queue' && activeTab !== 'calendar') return;
    const timer = window.setInterval(() => {
      void api.listPosts().then(setPosts).catch(() => {});
    }, 15000);
    return () => window.clearInterval(timer);
  }, [activeTab, authStatus]);

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoginError(null);
    setLoginLoading(true);
    try {
      await api.login(loginPassword);
      setLoginPassword('');
      await checkAuthAndLoad();
    } catch (err) {
      setLoginError(err instanceof Error ? err.message : 'Sign in failed');
    } finally {
      setLoginLoading(false);
    }
  };

  const handleLogout = async () => {
    await api.logout();
    setAuthStatus({ authRequired: true, authenticated: false });
  };

  const handlePostSavedOrPublished = (post: Post) => {
    setPosts((prev) => {
      const exists = prev.some((p) => p.id === post.id);
      if (exists) return prev.map((p) => (p.id === post.id ? post : p));
      return [post, ...prev];
    });
    void api.getScheduleSlots().then(setScheduleInfo).catch(() => {});
  };

  const handleOpenInComposer = (post: Post, asDuplicate = false) => {
    setEditingPostId(asDuplicate ? null : post.id);
    setMasterText(post.masterText);
    setFirstComment(post.firstComment ?? '');
    setCustomizePerNetwork(post.customizePerNetwork);
    setCustomTexts({ ...(post.customTexts ?? {}) });
    setMedia(post.media ? [...post.media] : []);
    setTags(post.tags ? [...post.tags] : []);

    const sourceIds =
      post.targetAccountIds?.length > 0
        ? post.targetAccountIds
        : post.results?.map((r) => r.accountId) ?? [];
    const validIds = sourceIds.filter((id) =>
      accounts.some((a) => a.id === id && a.enabled)
    );
    if (validIds.length > 0) setSelectedIds(validIds);
    setActiveTab('compose');
  };

  const counts = useMemo(() => {
    let queue = 0;
    let drafts = 0;
    let sent = 0;
    for (const p of posts) {
      if (p.status === 'scheduled' || p.status === 'publishing') queue++;
      else if (p.status === 'draft' || p.status === 'idea') drafts++;
      else sent++;
    }
    return { queue, drafts, sent };
  }, [posts]);

  if (!authStatus) {
    return (
      <div className="boot-screen">
        <div className="spinner" />
      </div>
    );
  }

  if (authStatus.authRequired && !authStatus.authenticated) {
    return (
      <div className="login-page">
        <div className="login-card surface-card">
          <div className="login-brand">
            <div className="brand-badge" aria-hidden="true">
              <svg width="24" height="24" viewBox="0 0 32 32" fill="none">
                <path d="M25 7C14 7 7 13.5 7 25C18.5 25 25 18 25 7Z" fill="currentColor" />
                <path d="M10 22L20 12" stroke="var(--primary)" strokeWidth="2.8" strokeLinecap="round" />
              </svg>
            </div>
            <h1>Teepost</h1>
          </div>
          <form onSubmit={handleLogin} className="login-form">
            <input
              type="password"
              className="text-input"
              placeholder="Password"
              value={loginPassword}
              onChange={(e) => setLoginPassword(e.target.value)}
              autoFocus
              required
            />
            {loginError && <div className="alert-banner alert-error">{loginError}</div>}
            <button type="submit" className="btn-primary btn-full" disabled={loginLoading}>
              {loginLoading ? 'Signing in...' : 'Sign In'}
            </button>
          </form>
        </div>
      </div>
    );
  }

  return (
    <div className="app-shell">
      <SidebarNav
        activeTab={activeTab}
        onSelectTab={setActiveTab}
        theme={theme}
        onToggleTheme={() => setTheme((t) => (t === 'dark' ? 'light' : 'dark'))}
        authRequired={authStatus.authRequired}
        onLogout={handleLogout}
        accountsCount={accounts.length}
        queueCount={counts.queue}
        queuePaused={Boolean(scheduleInfo?.paused)}
        draftsCount={counts.drafts}
        sentCount={counts.sent}
        nextSlot={scheduleInfo?.nextSlot}
        preferences={preferences}
      />

      <main className="app-main">
        {activeTab === 'compose' && (
          <ComposerWorkspace
            accounts={accounts}
            selectedIds={selectedIds}
            onChangeSelectedIds={setSelectedIds}
            editingPostId={editingPostId}
            onCancelEditingPost={() => setEditingPostId(null)}
            masterText={masterText}
            onChangeMasterText={setMasterText}
            firstComment={firstComment}
            onChangeFirstComment={setFirstComment}
            customizePerNetwork={customizePerNetwork}
            onChangeCustomizePerNetwork={setCustomizePerNetwork}
            customTexts={customTexts}
            onChangeCustomTexts={setCustomTexts}
            media={media}
            onChangeMedia={setMedia}
            tags={tags}
            onChangeTags={setTags}
            hashtagGroups={hashtagGroups}
            onHashtagGroupsChanged={setHashtagGroups}
            scheduleInfo={scheduleInfo}
            onPostSavedOrPublished={handlePostSavedOrPublished}
            onGoToAccounts={(p) => {
              if (p) setPreselectPlatform(p);
              setActiveTab('accounts');
            }}
            onNavigateTab={(t) => setActiveTab(t)}
          />
        )}

        {activeTab === 'queue' && (
          <QueueView
            posts={posts}
            accounts={accounts}
            scheduleInfo={scheduleInfo}
            preferences={preferences}
            onScheduleUpdated={setScheduleInfo}
            onPostsReplaced={setPosts}
            onEditInComposer={(post) => handleOpenInComposer(post, false)}
            onPostUpdated={handlePostSavedOrPublished}
            onPostDeleted={(id) =>
              setPosts((prev) => prev.filter((p) => p.id !== id))
            }
          />
        )}

        {activeTab === 'calendar' && (
          <CalendarView
            posts={posts}
            accounts={accounts}
            preferences={preferences}
            onEditInComposer={(post) =>
              handleOpenInComposer(
                post,
                post.status === 'sent' || post.status === 'failed'
              )
            }
          />
        )}

        {activeTab === 'drafts' && (
          <DraftsIdeasView
            posts={posts}
            accounts={accounts}
            onEditInComposer={(post) => handleOpenInComposer(post, false)}
            onPostUpdated={handlePostSavedOrPublished}
            onPostDeleted={(id) =>
              setPosts((prev) => prev.filter((p) => p.id !== id))
            }
          />
        )}

        {activeTab === 'history' && (
          <HistoryView
            posts={posts}
            loading={loadingData}
            preferences={preferences}
            onRefresh={() => void loadWorkspaceData()}
            onDuplicatePost={(post) => handleOpenInComposer(post, true)}
            onPostUpdated={handlePostSavedOrPublished}
            onPostDeleted={(id) =>
              setPosts((prev) => prev.filter((p) => p.id !== id))
            }
          />
        )}

        {activeTab === 'accounts' && (
          <AccountsSettingsView
            accounts={accounts}
            onAccountsChanged={loadWorkspaceData}
            authRequired={authStatus.authRequired}
            onAuthChanged={(nextAuthRequired) =>
              setAuthStatus({ authRequired: nextAuthRequired, authenticated: true })
            }
            preferences={preferences}
            onPreferencesChanged={(nextPrefs) => {
              setPreferences(nextPrefs);
              void api.getScheduleSlots().then(setScheduleInfo).catch(() => {});
            }}
            initialPlatformModal={preselectPlatform}
            onClearInitialModal={() => setPreselectPlatform(null)}
          />
        )}
      </main>
    </div>
  );
};
