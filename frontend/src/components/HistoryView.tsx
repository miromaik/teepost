import React, { useMemo, useState } from 'react';
import { api, DisplayPreferences, formatDateTime, PLATFORMS, Post } from '../api';
import { PlatformIcon } from './PlatformIcons';

interface HistoryViewProps {
  posts: Post[];
  loading: boolean;
  preferences: DisplayPreferences;
  onRefresh: () => void;
  onDuplicatePost: (post: Post) => void;
  onPostUpdated: (post: Post) => void;
  onPostDeleted: (id: string) => void;
}

export const HistoryView: React.FC<HistoryViewProps> = ({
  posts,
  loading,
  preferences,
  onRefresh,
  onDuplicatePost,
  onPostUpdated,
  onPostDeleted,
}) => {
  const [retryingKey, setRetryingKey] = useState<string | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const sentPosts = useMemo(
    () => posts.filter((p) => p.status === 'sent' || p.status === 'failed' || !p.status),
    [posts]
  );

  const handleRetry = async (postId: string, accountId: string) => {
    const key = `${postId}:${accountId}`;
    setRetryingKey(key);
    setError(null);
    try {
      const updated = await api.retryPostAccount(postId, accountId);
      onPostUpdated(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Retry failed');
    } finally {
      setRetryingKey(null);
    }
  };

  const handleDelete = async (id: string) => {
    setDeletingId(id);
    setError(null);
    try {
      await api.deletePost(id);
      onPostDeleted(id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete post');
    } finally {
      setDeletingId(null);
    }
  };

  const formatTimestamp = (iso: string) => formatDateTime(iso, preferences, true);

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>Sent</h1>
        <button type="button" className="btn-tonal btn-sm" onClick={onRefresh} disabled={loading}>
          {loading ? 'Refreshing...' : 'Refresh'}
        </button>
      </div>

      {error && <div className="alert-banner alert-error">{error}</div>}

      {!loading && sentPosts.length === 0 ? (
        <div className="surface-card empty-state">
          <h3>No posts sent yet</h3>
        </div>
      ) : (
        <div className="history-list">
          {sentPosts.map((post) => (
            <article key={post.id} className="surface-card history-card">
              <div className="history-card-top">
                <div className="queue-time-group">
                  <span className="pastel-pill mint">{formatTimestamp(post.createdAt)}</span>
                  {post.firstComment && <span className="pastel-pill sky">+ Comment</span>}
                </div>
                <div className="history-actions">
                  <button
                    type="button"
                    className="btn-tonal btn-xs"
                    onClick={() => onDuplicatePost(post)}
                  >
                    Duplicate
                  </button>
                  <button
                    type="button"
                    className="btn-ghost btn-xs danger-text"
                    onClick={() => handleDelete(post.id)}
                    disabled={deletingId === post.id}
                  >
                    ×
                  </button>
                </div>
              </div>

              <div className="history-content">
                <p className="history-text">
                  {post.masterText || <span className="muted-text">(Media post)</span>}
                </p>

                {post.media && post.media.length > 0 && (
                  <div className="history-thumbs">
                    {post.media.map((m) => (
                      <a
                        key={m.id}
                        href={m.url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="history-thumb"
                      >
                        <img src={m.url} alt={m.altText || m.filename} />
                      </a>
                    ))}
                  </div>
                )}
              </div>

              <div className="history-results">
                {post.results.map((res) => {
                  const meta = PLATFORMS[res.platform];
                  const isSuccess = res.status === 'success';
                  const key = `${post.id}:${res.accountId}`;
                  return (
                    <div key={res.id || key} className={`history-result-chip ${isSuccess ? 'ok' : 'fail'}`}>
                      <span className="chip-icon" style={{ color: meta.color }}>
                        <PlatformIcon platform={res.platform} size={14} />
                      </span>
                      <span className="chip-account">{res.accountName}</span>
                      {isSuccess ? (
                        <>
                          <span className="chip-status ok">Sent</span>
                          {res.externalUrl && (
                            <a
                              href={res.externalUrl}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="chip-link"
                            >
                              ↗
                            </a>
                          )}
                        </>
                      ) : (
                        <>
                          <span className="chip-status fail" title={res.errorMessage}>
                            Failed
                          </span>
                          <button
                            type="button"
                            className="chip-retry-btn"
                            onClick={() => handleRetry(post.id, res.accountId)}
                            disabled={retryingKey === key}
                          >
                            Retry
                          </button>
                        </>
                      )}
                    </div>
                  );
                })}
              </div>

              {post.results.some((r) => r.status === 'error') && (
                <div className="history-errors-box">
                  {post.results
                    .filter((r) => r.status === 'error')
                    .map((r) => (
                      <div key={r.accountId} className="history-error-line">
                        <strong>{r.accountName}:</strong> {r.errorMessage}
                      </div>
                    ))}
                </div>
              )}
            </article>
          ))}
        </div>
      )}
    </div>
  );
};
