import React, { useMemo, useState } from 'react';
import { Account, api, PLATFORMS, Post } from '../api';
import { PlatformIcon } from './PlatformIcons';

interface DraftsIdeasViewProps {
  posts: Post[];
  accounts: Account[];
  onEditInComposer: (post: Post) => void;
  onPostUpdated: (post: Post) => void;
  onPostDeleted: (id: string) => void;
}

export const DraftsIdeasView: React.FC<DraftsIdeasViewProps> = ({
  posts,
  accounts,
  onEditInComposer,
  onPostUpdated,
  onPostDeleted,
}) => {
  const [filterType, setFilterType] = useState<'all' | 'draft' | 'idea'>('all');
  const [ideaText, setIdeaText] = useState('');
  const [savingIdea, setSavingIdea] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const items = useMemo(
    () =>
      posts
        .filter((p) => p.status === 'draft' || p.status === 'idea')
        .sort((a, b) => (b.updatedAt || b.createdAt).localeCompare(a.updatedAt || a.createdAt)),
    [posts]
  );

  const filteredItems = useMemo(() => {
    return items.filter((p) => (filterType === 'all' ? true : p.status === filterType));
  }, [items, filterType]);

  const accountMap = useMemo(() => {
    const map = new Map<string, Account>();
    for (const a of accounts) map.set(a.id, a);
    return map;
  }, [accounts]);

  const handleQuickIdea = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!ideaText.trim()) return;
    setSavingIdea(true);
    setError(null);
    try {
      const saved = await api.savePost({
        mode: 'idea',
        accountIds: [],
        masterText: ideaText.trim(),
        customizePerNetwork: false,
        customTexts: {},
        media: [],
        tags: [],
      });
      onPostUpdated(saved);
      setIdeaText('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save idea');
    } finally {
      setSavingIdea(false);
    }
  };

  const handleAddToQueue = async (post: Post) => {
    const targets =
      post.targetAccountIds?.length > 0
        ? post.targetAccountIds
        : accounts.filter((a) => a.enabled).map((a) => a.id);

    if (targets.length === 0) {
      setError('Enable at least one channel in Settings first.');
      return;
    }

    setBusyId(post.id);
    setError(null);
    try {
      const queued = await api.savePost({
        id: post.id,
        mode: 'queue',
        accountIds: targets,
        masterText: post.masterText,
        firstComment: post.firstComment,
        customizePerNetwork: post.customizePerNetwork,
        customTexts: post.customTexts,
        media: post.media,
        tags: post.tags,
      });
      onPostUpdated(queued);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add to queue');
    } finally {
      setBusyId(null);
    }
  };

  const handleDelete = async (id: string) => {
    setBusyId(id);
    setError(null);
    try {
      await api.deletePost(id);
      onPostDeleted(id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed');
    } finally {
      setBusyId(null);
    }
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>Drafts & Ideas</h1>
        <div className="segmented-control">
          <button
            type="button"
            className={`seg-btn ${filterType === 'all' ? 'active' : ''}`}
            onClick={() => setFilterType('all')}
          >
            All ({items.length})
          </button>
          <button
            type="button"
            className={`seg-btn ${filterType === 'draft' ? 'active' : ''}`}
            onClick={() => setFilterType('draft')}
          >
            Drafts
          </button>
          <button
            type="button"
            className={`seg-btn ${filterType === 'idea' ? 'active' : ''}`}
            onClick={() => setFilterType('idea')}
          >
            Ideas
          </button>
        </div>
      </div>

      <form onSubmit={handleQuickIdea} className="surface-card quick-idea-card">
        <div className="quick-idea-inline">
          <input
            type="text"
            className="text-input"
            placeholder="Quick idea note..."
            value={ideaText}
            onChange={(e) => setIdeaText(e.target.value)}
          />
          <button
            type="submit"
            className="btn-primary btn-sm"
            disabled={savingIdea || !ideaText.trim()}
          >
            + Save Idea
          </button>
        </div>
      </form>

      {error && <div className="alert-banner alert-error">{error}</div>}

      {filteredItems.length === 0 ? (
        <div className="surface-card empty-state">
          <h3>Nothing here yet</h3>
          <p className="muted-text">Save a draft in Compose or jot down an idea above.</p>
        </div>
      ) : (
        <div className="drafts-grid">
          {filteredItems.map((post) => (
            <article key={post.id} className="surface-card draft-card">
              <div className="draft-card-top">
                <span className={`pastel-pill ${post.status === 'idea' ? 'peach' : 'sky'}`}>
                  {post.status === 'idea' ? 'Idea' : 'Draft'}
                </span>
                {post.firstComment && <span className="pastel-pill lavender">+ Comment</span>}
              </div>

              <p className="history-text">
                {post.masterText || <span className="muted-text">(Media draft)</span>}
              </p>

              {post.media?.length > 0 && (
                <div className="history-thumbs">
                  {post.media.map((m) => (
                    <div key={m.id} className="history-thumb">
                      <img src={m.url} alt={m.altText || m.filename} />
                    </div>
                  ))}
                </div>
              )}

              {post.targetAccountIds?.length > 0 && (
                <div className="history-results">
                  {post.targetAccountIds.map((accId) => {
                    const acc = accountMap.get(accId);
                    if (!acc) return null;
                    return (
                      <span key={acc.id} className="history-result-chip">
                        <span style={{ color: PLATFORMS[acc.platform].color }}>
                          <PlatformIcon platform={acc.platform} size={13} />
                        </span>
                        <span>{acc.name}</span>
                      </span>
                    );
                  })}
                </div>
              )}

              <div className="draft-card-actions">
                <button
                  type="button"
                  className="btn-primary btn-xs"
                  onClick={() => onEditInComposer(post)}
                >
                  Open
                </button>
                <button
                  type="button"
                  className="btn-tonal btn-xs"
                  onClick={() => handleAddToQueue(post)}
                  disabled={busyId === post.id}
                >
                  Queue
                </button>
                <button
                  type="button"
                  className="btn-ghost btn-xs danger-text"
                  onClick={() => handleDelete(post.id)}
                  disabled={busyId === post.id}
                >
                  ×
                </button>
              </div>
            </article>
          ))}
        </div>
      )}
    </div>
  );
};
