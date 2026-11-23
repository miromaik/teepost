import React, { useMemo, useState } from 'react';
import { Account, DisplayPreferences, formatDateTime, PLATFORMS, Post } from '../api';
import { PlatformIcon } from './PlatformIcons';

interface CalendarViewProps {
  posts: Post[];
  accounts: Account[];
  preferences: DisplayPreferences;
  onEditInComposer: (post: Post) => void;
}

type CalendarMode = 'month' | 'week';

function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

function sameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

function getMondayStart(d: Date): Date {
  const copy = startOfDay(d);
  const day = copy.getDay();
  const diff = day === 0 ? -6 : 1 - day;
  copy.setDate(copy.getDate() + diff);
  return copy;
}

export const CalendarView: React.FC<CalendarViewProps> = ({
  posts,
  accounts,
  preferences,
  onEditInComposer,
}) => {
  const [mode, setMode] = useState<CalendarMode>('month');
  const [cursor, setCursor] = useState<Date>(() => new Date());
  const [channelFilter, setChannelFilter] = useState<string>('all');

  const accountMap = useMemo(() => {
    const map = new Map<string, Account>();
    for (const acc of accounts) map.set(acc.id, acc);
    return map;
  }, [accounts]);

  const calendarPosts = useMemo(() => {
    return posts
      .filter((p) => p.status === 'scheduled' || p.status === 'sent' || p.status === 'failed')
      .filter((p) => {
        if (channelFilter === 'all') return true;
        const targets = p.targetAccountIds ?? [];
        const resultTargets = p.results?.map((r) => r.accountId) ?? [];
        return targets.includes(channelFilter) || resultTargets.includes(channelFilter);
      })
      .map((p) => {
        const ts = p.status === 'scheduled' && p.scheduledAt ? p.scheduledAt : p.createdAt;
        return { post: p, date: new Date(ts) };
      })
      .filter((item) => !Number.isNaN(item.date.getTime()));
  }, [posts, channelFilter]);

  const days = useMemo(() => {
    if (mode === 'week') {
      const start = getMondayStart(cursor);
      return Array.from({ length: 7 }, (_, i) => {
        const d = new Date(start);
        d.setDate(start.getDate() + i);
        return d;
      });
    }

    const firstOfMonth = new Date(cursor.getFullYear(), cursor.getMonth(), 1);
    const gridStart = getMondayStart(firstOfMonth);
    return Array.from({ length: 35 }, (_, i) => {
      const d = new Date(gridStart);
      d.setDate(gridStart.getDate() + i);
      return d;
    });
  }, [mode, cursor]);

  const handleShift = (dir: -1 | 1) => {
    const next = new Date(cursor);
    if (mode === 'week') {
      next.setDate(next.getDate() + dir * 7);
    } else {
      next.setMonth(next.getMonth() + dir);
    }
    setCursor(next);
  };

  const titleLabel = useMemo(() => {
    return new Intl.DateTimeFormat(undefined, {
      month: 'long',
      year: 'numeric',
    }).format(cursor);
  }, [cursor]);

  const today = new Date();

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>Calendar</h1>

        <div className="calendar-controls">
          <select
            className="text-input calendar-filter-select"
            value={channelFilter}
            onChange={(e) => setChannelFilter(e.target.value)}
            aria-label="Filter by channel"
          >
            <option value="all">All channels</option>
            {accounts.map((acc) => (
              <option key={acc.id} value={acc.id}>
                {acc.name} ({PLATFORMS[acc.platform].shortLabel})
              </option>
            ))}
          </select>

          <div className="segmented-control">
            <button
              type="button"
              className={`seg-btn ${mode === 'month' ? 'active' : ''}`}
              onClick={() => setMode('month')}
            >
              Month
            </button>
            <button
              type="button"
              className={`seg-btn ${mode === 'week' ? 'active' : ''}`}
              onClick={() => setMode('week')}
            >
              Week
            </button>
          </div>
        </div>
      </div>

      <div className="surface-card calendar-shell">
        <div className="calendar-toolbar">
          <div className="calendar-nav-btns">
            <button type="button" className="btn-tonal btn-xs" onClick={() => handleShift(-1)}>
              ←
            </button>
            <button type="button" className="btn-ghost btn-xs" onClick={() => setCursor(new Date())}>
              Today
            </button>
            <button type="button" className="btn-tonal btn-xs" onClick={() => handleShift(1)}>
              →
            </button>
          </div>

          <h2 className="calendar-month-title">{titleLabel}</h2>

          <div className="calendar-legend">
            <span className="pastel-pill lavender">Scheduled</span>
            <span className="pastel-pill mint">Sent</span>
          </div>
        </div>

        <div className="calendar-weekdays">
          {['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'].map((d) => (
            <div key={d} className="calendar-weekday-cell">
              {d}
            </div>
          ))}
        </div>

        <div className={`calendar-grid ${mode}`}>
          {days.map((day) => {
            const isToday = sameDay(day, today);
            const inCurrentMonth = day.getMonth() === cursor.getMonth();
            const dayPosts = calendarPosts
              .filter((item) => sameDay(item.date, day))
              .sort((a, b) => a.date.getTime() - b.date.getTime());

            return (
              <div
                key={day.toISOString()}
                className={`calendar-cell ${isToday ? 'today' : ''} ${
                  !inCurrentMonth && mode === 'month' ? 'other-month' : ''
                }`}
              >
                <div className="calendar-cell-day">
                  <span>{day.getDate()}</span>
                </div>

                <div className="calendar-events">
                  {dayPosts.map(({ post, date }) => {
                    const timeStr = formatDateTime(date, preferences, false);
                    const channels = (
                      post.targetAccountIds?.length
                        ? post.targetAccountIds
                        : post.results?.map((r) => r.accountId) ?? []
                    )
                      .map((id) => accountMap.get(id))
                      .filter((x): x is Account => Boolean(x));

                    return (
                      <button
                        key={post.id}
                        type="button"
                        className={`calendar-event-card ${post.status}`}
                        onClick={() => onEditInComposer(post)}
                        title={post.masterText}
                      >
                        <div className="calendar-event-top">
                          <span className="calendar-event-time">{timeStr}</span>
                          <span className="calendar-event-icons">
                            {channels.slice(0, 3).map((acc) => (
                              <span key={acc.id} style={{ color: PLATFORMS[acc.platform].color }}>
                                <PlatformIcon platform={acc.platform} size={11} />
                              </span>
                            ))}
                          </span>
                        </div>
                        <div className="calendar-event-text">
                          {post.masterText || '(Media)'}
                        </div>
                      </button>
                    );
                  })}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
};
