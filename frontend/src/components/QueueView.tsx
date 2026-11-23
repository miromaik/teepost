import React, { useMemo, useState } from 'react';
import {
  Account,
  api,
  DisplayPreferences,
  formatDateTime,
  PLATFORMS,
  Post,
  PostingSlot,
  ScheduleSlotsResponse,
} from '../api';
import { PlatformIcon } from './PlatformIcons';

interface QueueViewProps {
  posts: Post[];
  accounts: Account[];
  scheduleInfo: ScheduleSlotsResponse | null;
  preferences: DisplayPreferences;
  onScheduleUpdated: (next: ScheduleSlotsResponse) => void;
  onPostsReplaced: (posts: Post[]) => void;
  onEditInComposer: (post: Post) => void;
  onPostUpdated: (post: Post) => void;
  onPostDeleted: (id: string) => void;
}

const WEEKDAYS = [
  { value: 1, label: 'Monday', short: 'Mon' },
  { value: 2, label: 'Tuesday', short: 'Tue' },
  { value: 3, label: 'Wednesday', short: 'Wed' },
  { value: 4, label: 'Thursday', short: 'Thu' },
  { value: 5, label: 'Friday', short: 'Fri' },
  { value: 6, label: 'Saturday', short: 'Sat' },
  { value: 0, label: 'Sunday', short: 'Sun' },
];

export const QueueView: React.FC<QueueViewProps> = ({
  posts,
  accounts,
  scheduleInfo,
  preferences,
  onScheduleUpdated,
  onPostsReplaced,
  onEditInComposer,
  onPostUpdated,
  onPostDeleted,
}) => {
  const [busyId, setBusyId] = useState<string | null>(null);
  const [queueBusy, setQueueBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [newWeekday, setNewWeekday] = useState<number>(1);
  const [newTime, setNewTime] = useState<string>('09:00');
  const [savingSlots, setSavingSlots] = useState(false);

  const scheduledPosts = useMemo(
    () =>
      posts
        .filter((p) => p.status === 'scheduled' || p.status === 'publishing')
        .sort((a, b) => (a.scheduledAt || '').localeCompare(b.scheduledAt || '')),
    [posts]
  );

  const accountMap = useMemo(() => {
    const map = new Map<string, Account>();
    for (const acc of accounts) map.set(acc.id, acc);
    return map;
  }, [accounts]);

  const formatScheduledTime = (iso: string) => {
    if (!iso) return 'Unscheduled';
    return formatDateTime(iso, preferences, true);
  };

  const handleTogglePause = async () => {
    if (!scheduleInfo) return;
    setQueueBusy(true);
    setError(null);
    try {
      const res = await api.setQueuePaused(!scheduleInfo.paused);
      onScheduleUpdated({ ...scheduleInfo, paused: res.paused });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to toggle queue pause');
    } finally {
      setQueueBusy(false);
    }
  };

  const handleShuffle = async () => {
    setQueueBusy(true);
    setError(null);
    try {
      const nextPosts = await api.shuffleQueue();
      onPostsReplaced(nextPosts);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Shuffle failed');
    } finally {
      setQueueBusy(false);
    }
  };

  const handleReorder = async (postId: string, direction: 'up' | 'down') => {
    setBusyId(postId);
    setError(null);
    try {
      const nextPosts = await api.reorderQueue(postId, direction);
      onPostsReplaced(nextPosts);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Reorder failed');
    } finally {
      setBusyId(null);
    }
  };

  const handlePublishNow = async (post: Post) => {
    setBusyId(post.id);
    setError(null);
    try {
      const updated = await api.publishSavedPostNow(post.id);
      onPostUpdated(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Publish failed');
    } finally {
      setBusyId(null);
    }
  };

  const handleMoveToDrafts = async (post: Post) => {
    setBusyId(post.id);
    setError(null);
    try {
      const updated = await api.savePost({
        id: post.id,
        mode: 'draft',
        accountIds: post.targetAccountIds,
        masterText: post.masterText,
        firstComment: post.firstComment,
        customizePerNetwork: post.customizePerNetwork,
        customTexts: post.customTexts,
        media: post.media,
        tags: post.tags,
      });
      onPostUpdated(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to move post');
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

  const handleAddSlot = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!scheduleInfo) return;
    setSavingSlots(true);
    setError(null);
    try {
      const nextSlots: PostingSlot[] = [
        ...scheduleInfo.slots,
        { id: '', weekday: newWeekday, timeHhmm: newTime },
      ];
      const updated = await api.updateScheduleSlots(nextSlots);
      onScheduleUpdated(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update schedule');
    } finally {
      setSavingSlots(false);
    }
  };

  const handleRemoveSlot = async (slotId: string) => {
    if (!scheduleInfo) return;
    setSavingSlots(true);
    setError(null);
    try {
      const nextSlots = scheduleInfo.slots.filter((s) => s.id !== slotId);
      const updated = await api.updateScheduleSlots(nextSlots);
      onScheduleUpdated(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to remove slot');
    } finally {
      setSavingSlots(false);
    }
  };

  const isPaused = Boolean(scheduleInfo?.paused);

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>Queue</h1>
        <div className="queue-header-actions">
          <button
            type="button"
            className="btn-tonal btn-sm"
            onClick={handleShuffle}
            disabled={queueBusy || scheduledPosts.length < 2}
          >
            Shuffle
          </button>
          <button
            type="button"
            className={`btn-sm ${isPaused ? 'btn-primary' : 'btn-tonal'}`}
            onClick={handleTogglePause}
            disabled={queueBusy}
          >
            {isPaused ? 'Resume Queue' : 'Pause Queue'}
          </button>
        </div>
      </div>

      {isPaused && (
        <div className="alert-banner alert-warning">
          Queue is paused. Scheduled posts will wait until you resume.
        </div>
      )}

      {error && <div className="alert-banner alert-error">{error}</div>}

      <div className="queue-layout">
        <section className="queue-posts-col">
          {scheduledPosts.length === 0 ? (
            <div className="surface-card empty-state">
              <h3>Queue is empty</h3>
              <p className="muted-text">Add posts from Compose using Queue or Schedule.</p>
            </div>
          ) : (
            <div className="history-list">
              {scheduledPosts.map((post, idx) => (
                <article key={post.id} className="surface-card history-card">
                  <div className="history-card-top">
                    <div className="queue-time-group">
                      <span className="pastel-pill lavender">
                        {post.status === 'publishing'
                          ? 'Publishing...'
                          : formatScheduledTime(post.scheduledAt)}
                      </span>
                      {post.firstComment && (
                        <span className="pastel-pill sky">+ Comment</span>
                      )}
                    </div>

                    <div className="history-actions">
                      <button
                        type="button"
                        className="btn-ghost btn-xs"
                        onClick={() => handleReorder(post.id, 'up')}
                        disabled={busyId === post.id || idx === 0}
                        title="Move earlier"
                      >
                        ↑
                      </button>
                      <button
                        type="button"
                        className="btn-ghost btn-xs"
                        onClick={() => handleReorder(post.id, 'down')}
                        disabled={busyId === post.id || idx === scheduledPosts.length - 1}
                        title="Move later"
                      >
                        ↓
                      </button>
                      <button
                        type="button"
                        className="btn-primary btn-xs"
                        onClick={() => handlePublishNow(post)}
                        disabled={busyId === post.id}
                      >
                        Post Now
                      </button>
                      <button
                        type="button"
                        className="btn-tonal btn-xs"
                        onClick={() => onEditInComposer(post)}
                        disabled={busyId === post.id}
                      >
                        Edit
                      </button>
                      <button
                        type="button"
                        className="btn-ghost btn-xs"
                        onClick={() => handleMoveToDrafts(post)}
                        disabled={busyId === post.id}
                      >
                        Draft
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
                  </div>

                  <p className="history-text">
                    {post.masterText || <span className="muted-text">(Media post)</span>}
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

                  <div className="history-results">
                    {(post.targetAccountIds ?? []).map((accId) => {
                      const acc = accountMap.get(accId);
                      if (!acc) return null;
                      const meta = PLATFORMS[acc.platform];
                      return (
                        <div key={acc.id} className="history-result-chip">
                          <span className="chip-icon" style={{ color: meta.color }}>
                            <PlatformIcon platform={acc.platform} size={13} />
                          </span>
                          <span className="chip-account">{acc.name}</span>
                        </div>
                      );
                    })}
                  </div>
                </article>
              ))}
            </div>
          )}
        </section>

        <aside className="surface-card schedule-config-card">
          <div className="schedule-config-head">
            <h3>Posting Schedule</h3>
            <span className="muted-text">{scheduleInfo?.timezone || 'Local'}</span>
          </div>

          <form onSubmit={handleAddSlot} className="add-slot-form">
            <select
              className="text-input"
              value={newWeekday}
              onChange={(e) => setNewWeekday(Number(e.target.value))}
            >
              {WEEKDAYS.map((wd) => (
                <option key={wd.value} value={wd.value}>
                  {wd.label}
                </option>
              ))}
            </select>
            <input
              type="time"
              className="text-input"
              value={newTime}
              onChange={(e) => setNewTime(e.target.value)}
              required
            />
            <button type="submit" className="btn-tonal btn-sm" disabled={savingSlots}>
              +
            </button>
          </form>

          <div className="slots-days-list">
            {WEEKDAYS.map((wd) => {
              const daySlots = (scheduleInfo?.slots ?? []).filter(
                (s) => s.weekday === wd.value
              );
              return (
                <div key={wd.value} className="slot-day-row">
                  <span className="slot-day-label">{wd.short}</span>
                  <div className="slot-chips">
                    {daySlots.length === 0 ? (
                      <span className="muted-text slot-empty">-</span>
                    ) : (
                      daySlots.map((slot) => (
                        <span key={slot.id} className="slot-chip">
                          {slot.timeHhmm}
                          <button
                            type="button"
                            className="slot-chip-del"
                            onClick={() => handleRemoveSlot(slot.id)}
                          >
                            ×
                          </button>
                        </span>
                      ))
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </aside>
      </div>
    </div>
  );
};
