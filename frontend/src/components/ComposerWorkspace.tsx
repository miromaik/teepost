import React, { useMemo, useRef, useState } from 'react';
import {
  Account,
  api,
  getPlatformLimit,
  HashtagGroup,
  MediaItem,
  PLATFORM_ORDER,
  PlatformId,
  PLATFORMS,
  Post,
  ScheduleSlotsResponse,
} from '../api';
import { formatBytes, optimizeImageForUpload } from '../image-optimizer';
import { LivePreviewPanel } from './LivePreviewPanel';
import { PlatformIcon } from './PlatformIcons';

interface ComposerWorkspaceProps {
  accounts: Account[];
  selectedIds: string[];
  onChangeSelectedIds: (ids: string[]) => void;
  editingPostId: string | null;
  onCancelEditingPost: () => void;
  masterText: string;
  onChangeMasterText: (text: string) => void;
  firstComment: string;
  onChangeFirstComment: (text: string) => void;
  customizePerNetwork: boolean;
  onChangeCustomizePerNetwork: (enabled: boolean) => void;
  customTexts: Record<string, string>;
  onChangeCustomTexts: (next: Record<string, string>) => void;
  media: MediaItem[];
  onChangeMedia: (items: MediaItem[]) => void;
  tags: string[];
  onChangeTags: (tags: string[]) => void;
  hashtagGroups: HashtagGroup[];
  onHashtagGroupsChanged: (groups: HashtagGroup[]) => void;
  scheduleInfo: ScheduleSlotsResponse | null;
  onPostSavedOrPublished: (post: Post) => void;
  onGoToAccounts: (preselectPlatform?: PlatformId) => void;
  onNavigateTab: (tab: 'queue' | 'drafts') => void;
}

function defaultScheduleInputValue(): string {
  const d = new Date(Date.now() + 60 * 60 * 1000);
  d.setMinutes(0, 0, 0);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(
    d.getHours()
  )}:${pad(d.getMinutes())}`;
}

export const ComposerWorkspace: React.FC<ComposerWorkspaceProps> = ({
  accounts,
  selectedIds,
  onChangeSelectedIds,
  editingPostId,
  onCancelEditingPost,
  masterText,
  onChangeMasterText,
  firstComment,
  onChangeFirstComment,
  customizePerNetwork,
  onChangeCustomizePerNetwork,
  customTexts,
  onChangeCustomTexts,
  media,
  onChangeMedia,
  tags,
  onChangeTags,
  hashtagGroups,
  onHashtagGroupsChanged,
  scheduleInfo,
  onPostSavedOrPublished,
  onGoToAccounts,
  onNavigateTab,
}) => {
  const [activeNetworkTab, setActiveNetworkTab] = useState<string>('');
  const [isDragging, setIsDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [publishing, setPublishing] = useState(false);
  const [savingMode, setSavingMode] = useState<'draft' | 'queue' | 'schedule' | null>(null);
  const [publishError, setPublishError] = useState<string | null>(null);
  const [statusNotice, setStatusNotice] = useState<{
    text: string;
    actionLabel?: string;
    actionTab?: 'queue' | 'drafts';
  } | null>(null);
  const [latestPost, setLatestPost] = useState<Post | null>(null);
  const [retryingAccountId, setRetryingAccountId] = useState<string | null>(null);
  const [showSchedulePicker, setShowSchedulePicker] = useState(false);
  const [scheduleDateTime, setScheduleDateTime] = useState<string>(defaultScheduleInputValue);
  const [showFirstComment, setShowFirstComment] = useState(firstComment.trim() !== '');
  const [showNewGroupForm, setShowNewGroupForm] = useState(false);
  const [newGroupName, setNewGroupName] = useState('');
  const [newGroupTags, setNewGroupTags] = useState('');

  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const enabledAccounts = useMemo(
    () => accounts.filter((a) => a.enabled),
    [accounts]
  );

  const selectedAccounts = useMemo(
    () => enabledAccounts.filter((a) => selectedIds.includes(a.id)),
    [enabledAccounts, selectedIds]
  );

  const hasMedia = media.length > 0;

  const currentTabId = useMemo(() => {
    if (!customizePerNetwork || selectedAccounts.length === 0) return '';
    if (selectedAccounts.some((a) => a.id === activeNetworkTab)) {
      return activeNetworkTab;
    }
    return selectedAccounts[0].id;
  }, [customizePerNetwork, selectedAccounts, activeNetworkTab]);

  const activeEditorText = useMemo(() => {
    if (!customizePerNetwork || !currentTabId) return masterText;
    return customTexts[currentTabId] ?? masterText;
  }, [customizePerNetwork, currentTabId, customTexts, masterText]);

  const handleToggleCustomize = (enabled: boolean) => {
    if (enabled) {
      const seeded: Record<string, string> = { ...customTexts };
      for (const acc of selectedAccounts) {
        if (seeded[acc.id] === undefined) {
          seeded[acc.id] = masterText;
        }
      }
      onChangeCustomTexts(seeded);
      if (selectedAccounts.length > 0) {
        setActiveNetworkTab(selectedAccounts[0].id);
      }
    }
    onChangeCustomizePerNetwork(enabled);
  };

  const handleEditorTextChange = (nextText: string) => {
    if (!customizePerNetwork || !currentTabId) {
      onChangeMasterText(nextText);
      return;
    }
    onChangeCustomTexts({
      ...customTexts,
      [currentTabId]: nextText,
    });
  };

  const handleAppendHashtags = (hashtags: string) => {
    const clean = hashtags.trim();
    if (!clean) return;
    const sep = activeEditorText.trim() === '' ? '' : '\n\n';
    handleEditorTextChange(`${activeEditorText.trimEnd()}${sep}${clean}`);
  };

  const handleSaveNewGroup = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newGroupName.trim() || !newGroupTags.trim()) return;
    try {
      const next = await api.saveHashtagGroups([
        ...hashtagGroups,
        { id: '', name: newGroupName.trim(), hashtags: newGroupTags.trim() },
      ]);
      onHashtagGroupsChanged(next);
      setNewGroupName('');
      setNewGroupTags('');
      setShowNewGroupForm(false);
    } catch (err) {
      setPublishError(err instanceof Error ? err.message : 'Failed to save hashtag group.');
    }
  };

  const limitAnalysis = useMemo(() => {
    if (selectedAccounts.length === 0) {
      return {
        strictestLimit: 280,
        strictestName: 'Default',
        activeLength: activeEditorText.length,
        exceededAccounts: [] as { account: Account; count: number; limit: number }[],
      };
    }

    let strictestLimit = Infinity;
    let strictestName = '';
    const exceededAccounts: { account: Account; count: number; limit: number }[] = [];

    for (const acc of selectedAccounts) {
      const limit = getPlatformLimit(acc.platform, hasMedia);
      if (limit < strictestLimit) {
        strictestLimit = limit;
        strictestName = PLATFORMS[acc.platform].shortLabel;
      }
      const text = customizePerNetwork ? customTexts[acc.id] ?? masterText : masterText;
      if (text.length > limit) {
        exceededAccounts.push({ account: acc, count: text.length, limit });
      }
    }

    const currentLimit =
      customizePerNetwork && currentTabId
        ? getPlatformLimit(
            selectedAccounts.find((a) => a.id === currentTabId)?.platform ?? 'x',
            hasMedia
          )
        : strictestLimit;

    return {
      strictestLimit: currentLimit,
      strictestName:
        customizePerNetwork && currentTabId
          ? PLATFORMS[
              selectedAccounts.find((a) => a.id === currentTabId)?.platform ?? 'x'
            ].shortLabel
          : strictestName,
      activeLength: activeEditorText.length,
      exceededAccounts,
    };
  }, [
    selectedAccounts,
    hasMedia,
    customizePerNetwork,
    customTexts,
    masterText,
    currentTabId,
    activeEditorText.length,
  ]);

  const handleFilesSelected = async (fileList: FileList | null) => {
    if (!fileList || fileList.length === 0) return;
    setUploadError(null);

    const remainingSlots = Math.max(0, 4 - media.length);
    if (remainingSlots === 0) {
      setUploadError('Max 4 images per post.');
      return;
    }

    const filesToProcess = Array.from(fileList).slice(0, remainingSlots);
    setUploading(true);
    try {
      const uploadedItems: MediaItem[] = [];
      for (const file of filesToProcess) {
        if (!file.type.startsWith('image/')) {
          throw new Error(`"${file.name}" is not an image.`);
        }
        const optimized = await optimizeImageForUpload(file);
        const stored = await api.uploadMedia(optimized.file);
        uploadedItems.push({
          ...stored,
          altText: '',
          wasCompressed: optimized.wasCompressed,
          originalSize: optimized.originalSize,
        });
      }
      onChangeMedia([...media, ...uploadedItems]);
    } catch (err) {
      setUploadError(err instanceof Error ? err.message : 'Upload failed.');
    } finally {
      setUploading(false);
      if (fileInputRef.current) fileInputRef.current.value = '';
    }
  };

  const handleMoveMedia = (idx: number, dir: -1 | 1) => {
    const target = idx + dir;
    if (target < 0 || target >= media.length) return;
    const copy = [...media];
    [copy[idx], copy[target]] = [copy[target], copy[idx]];
    onChangeMedia(copy);
  };

  const handleUpdateAltText = (id: string, altText: string) => {
    onChangeMedia(media.map((m) => (m.id === id ? { ...m, altText } : m)));
  };

  const handleRemoveMedia = (id: string) => {
    onChangeMedia(media.filter((m) => m.id !== id));
  };

  const handleToggleAccount = (id: string) => {
    if (selectedIds.includes(id)) {
      onChangeSelectedIds(selectedIds.filter((x) => x !== id));
    } else {
      const nextIds = [...selectedIds, id];
      onChangeSelectedIds(nextIds);
      if (customizePerNetwork && customTexts[id] === undefined) {
        onChangeCustomTexts({ ...customTexts, [id]: masterText });
      }
    }
  };

  const hasContent = useMemo(() => {
    if (media.length > 0) return true;
    if (!customizePerNetwork) return masterText.trim().length > 0;
    return selectedAccounts.some(
      (a) => (customTexts[a.id] ?? masterText).trim().length > 0
    );
  }, [media.length, customizePerNetwork, masterText, selectedAccounts, customTexts]);

  const canPublish =
    selectedAccounts.length > 0 &&
    hasContent &&
    limitAnalysis.exceededAccounts.length === 0 &&
    !publishing &&
    !savingMode &&
    !uploading;

  const canSaveDraft = hasContent && !publishing && !savingMode && !uploading;

  const handlePublish = async () => {
    setPublishError(null);
    setStatusNotice(null);
    setPublishing(true);
    try {
      const post = await api.publishPost({
        postId: editingPostId ?? undefined,
        accountIds: selectedAccounts.map((a) => a.id),
        masterText,
        firstComment: showFirstComment ? firstComment : '',
        customizePerNetwork,
        customTexts,
        media,
        tags,
      });
      setLatestPost(post);
      onPostSavedOrPublished(post);
      if (editingPostId) onCancelEditingPost();
    } catch (err) {
      setPublishError(err instanceof Error ? err.message : 'Publish failed.');
    } finally {
      setPublishing(false);
    }
  };

  const handleSaveMode = async (mode: 'draft' | 'queue' | 'schedule') => {
    setPublishError(null);
    setStatusNotice(null);
    setSavingMode(mode);
    try {
      let scheduledAtISO: string | undefined;
      if (mode === 'schedule') {
        const parsed = new Date(scheduleDateTime);
        if (Number.isNaN(parsed.getTime())) {
          throw new Error('Choose a valid date and time.');
        }
        scheduledAtISO = parsed.toISOString();
      }

      const saved = await api.savePost({
        id: editingPostId ?? undefined,
        mode,
        accountIds: selectedAccounts.map((a) => a.id),
        masterText,
        firstComment: showFirstComment ? firstComment : '',
        customizePerNetwork,
        customTexts,
        media,
        tags,
        scheduledAt: scheduledAtISO,
      });

      onPostSavedOrPublished(saved);
      setShowSchedulePicker(false);

      if (mode === 'draft') {
        setStatusNotice({ text: 'Saved to Drafts.', actionLabel: 'Open Drafts', actionTab: 'drafts' });
      } else {
        const whenStr = saved.scheduledAt
          ? new Date(saved.scheduledAt).toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })
          : 'next slot';
        setStatusNotice({ text: `Scheduled for ${whenStr}.`, actionLabel: 'Open Queue', actionTab: 'queue' });
      }
      handleResetDraft(true);
    } catch (err) {
      setPublishError(err instanceof Error ? err.message : 'Save failed.');
    } finally {
      setSavingMode(null);
    }
  };

  const handleRetryAccount = async (postId: string, accountId: string) => {
    setRetryingAccountId(accountId);
    try {
      const updated = await api.retryPostAccount(postId, accountId);
      setLatestPost(updated);
      onPostSavedOrPublished(updated);
    } catch (err) {
      setPublishError(err instanceof Error ? err.message : 'Retry failed.');
    } finally {
      setRetryingAccountId(null);
    }
  };

  const handleResetDraft = (keepNotice = false) => {
    onChangeMasterText('');
    onChangeFirstComment('');
    setShowFirstComment(false);
    onChangeCustomTexts({});
    onChangeCustomizePerNetwork(false);
    onChangeMedia([]);
    onChangeTags([]);
    onCancelEditingPost();
    setLatestPost(null);
    setPublishError(null);
    if (!keepNotice) setStatusNotice(null);
  };

  const remainingChars = limitAnalysis.strictestLimit - limitAnalysis.activeLength;
  const charProgress = Math.min(
    100,
    Math.round((limitAnalysis.activeLength / Math.max(1, limitAnalysis.strictestLimit)) * 100)
  );

  if (accounts.length === 0) {
    return (
      <div className="onboarding-wrap">
        <div className="surface-card onboarding-card">
          <h1>Connect your first channel</h1>
          <p className="muted-text">Pick a platform to start posting and scheduling.</p>
          <div className="onboarding-platform-buttons">
            {PLATFORM_ORDER.map((pid) => {
              const meta = PLATFORMS[pid];
              return (
                <button
                  key={pid}
                  type="button"
                  className="onboarding-platform-btn"
                  onClick={() => onGoToAccounts(pid)}
                >
                  <span
                    className="platform-badge-icon"
                    style={{ backgroundColor: meta.softBg, color: meta.color }}
                  >
                    <PlatformIcon platform={pid} size={20} />
                  </span>
                  <strong>{meta.label}</strong>
                </button>
              );
            })}
          </div>
        </div>
      </div>
    );
  }

  const allSelected = enabledAccounts.length > 0 && enabledAccounts.every((a) => selectedIds.includes(a.id));

  return (
    <div className="composer-grid">
      <section className="composer-column">
        {editingPostId && (
          <div className="editing-banner">
            <span>Editing saved post</span>
            <button type="button" className="text-btn" onClick={() => handleResetDraft()}>
              Cancel
            </button>
          </div>
        )}

        {statusNotice && (
          <div className="alert-banner alert-success notice-row">
            <span>{statusNotice.text}</span>
            {statusNotice.actionTab && statusNotice.actionLabel && (
              <button
                type="button"
                className="text-btn"
                onClick={() => onNavigateTab(statusNotice.actionTab!)}
              >
                {statusNotice.actionLabel} →
              </button>
            )}
          </div>
        )}

        <div className="surface-card composer-card">
          {/* Minimal Channel Selector */}
          <div className="account-selector-bar">
            {enabledAccounts.length === 0 ? (
              <div className="account-selector-empty">
                <span className="muted-text">No enabled channels</span>
                <button type="button" className="btn-primary btn-xs" onClick={() => onGoToAccounts()}>
                  + Add Channel
                </button>
              </div>
            ) : (
              <div className="account-pills-wrap">
                {enabledAccounts.map((account) => {
                  const isSelected = selectedIds.includes(account.id);
                  const meta = PLATFORMS[account.platform];
                  return (
                    <button
                      key={account.id}
                      type="button"
                      className={`account-pill ${isSelected ? 'selected' : ''}`}
                      onClick={() => handleToggleAccount(account.id)}
                      style={
                        isSelected
                          ? ({ '--pill-accent': meta.color } as React.CSSProperties)
                          : undefined
                      }
                      aria-pressed={isSelected}
                    >
                      <span
                        className="account-pill-icon"
                        style={{
                          color: isSelected
                            ? account.platform === 'x'
                              ? 'var(--x-on-brand)'
                              : '#fff'
                            : meta.color,
                          backgroundColor: isSelected ? meta.color : meta.softBg,
                        }}
                      >
                        <PlatformIcon platform={account.platform} size={13} />
                      </span>
                      <span className="account-pill-name">{account.name}</span>
                    </button>
                  );
                })}
                <button
                  type="button"
                  className="text-btn"
                  onClick={() =>
                    onChangeSelectedIds(allSelected ? [] : enabledAccounts.map((a) => a.id))
                  }
                >
                  {allSelected ? 'None' : 'All'}
                </button>
              </div>
            )}
          </div>

          <div className="composer-toolbar">
            <label className="switch-control">
              <input
                type="checkbox"
                checked={customizePerNetwork}
                onChange={(e) => handleToggleCustomize(e.target.checked)}
                disabled={selectedAccounts.length === 0}
              />
              <span className="switch-track" aria-hidden="true">
                <span className="switch-thumb" />
              </span>
              <span className="switch-label">Customize per network</span>
            </label>

            <div className="composer-toolbar-right">
              <button
                type="button"
                className={`btn-ghost btn-xs ${showFirstComment ? 'active-chip' : ''}`}
                onClick={() => setShowFirstComment((v) => !v)}
              >
                {showFirstComment ? '− First Comment' : '+ First Comment'}
              </button>
              {(masterText || media.length > 0 || latestPost || firstComment) && (
                <button type="button" className="text-btn" onClick={() => handleResetDraft()}>
                  Clear
                </button>
              )}
            </div>
          </div>

          {customizePerNetwork && selectedAccounts.length > 0 && (
            <div className="network-tabs" role="tablist">
              {selectedAccounts.map((acc) => {
                const meta = PLATFORMS[acc.platform];
                const isActive = acc.id === currentTabId;
                const tabText = customTexts[acc.id] ?? masterText;
                const tabLimit = getPlatformLimit(acc.platform, hasMedia);
                const over = tabText.length > tabLimit;
                return (
                  <button
                    key={acc.id}
                    type="button"
                    role="tab"
                    aria-selected={isActive}
                    className={`network-tab ${isActive ? 'active' : ''} ${over ? 'over' : ''}`}
                    onClick={() => setActiveNetworkTab(acc.id)}
                  >
                    <span style={{ color: meta.color }}>
                      <PlatformIcon platform={acc.platform} size={13} />
                    </span>
                    <span>{acc.name}</span>
                    <span className="tab-count">
                      {tabText.length}/{tabLimit}
                    </span>
                  </button>
                );
              })}
            </div>
          )}

          <div className="editor-wrap">
            <textarea
              className="composer-textarea"
              rows={6}
              placeholder="Write your post..."
              value={activeEditorText}
              onChange={(e) => handleEditorTextChange(e.target.value)}
            />

            <div className="counter-bar">
              <div className="counter-left">
                <div className="counter-meter">
                  <div
                    className={`counter-meter-fill ${remainingChars < 0 ? 'over' : remainingChars <= 20 ? 'warn' : ''}`}
                    style={{ width: `${charProgress}%` }}
                  />
                </div>
                <span
                  className={`counter-text ${remainingChars < 0 ? 'over' : remainingChars <= 20 ? 'warn' : ''}`}
                >
                  {limitAnalysis.activeLength}/{limitAnalysis.strictestLimit}
                </span>
              </div>

              {/* Saved Hashtag Groups */}
              <div className="hashtag-groups-bar">
                {hashtagGroups.map((grp) => (
                  <button
                    key={grp.id}
                    type="button"
                    className="tag-chip clickable"
                    title={grp.hashtags}
                    onClick={() => handleAppendHashtags(grp.hashtags)}
                  >
                    #{grp.name}
                  </button>
                ))}
                <button
                  type="button"
                  className="tag-chip clickable"
                  onClick={() => setShowNewGroupForm((v) => !v)}
                >
                  + Group
                </button>
              </div>
            </div>

            {showNewGroupForm && (
              <form onSubmit={handleSaveNewGroup} className="new-group-form">
                <input
                  type="text"
                  className="text-input"
                  placeholder="Group name (e.g. Launch)"
                  value={newGroupName}
                  onChange={(e) => setNewGroupName(e.target.value)}
                  required
                />
                <input
                  type="text"
                  className="text-input"
                  placeholder="#tag1 #tag2 #tag3"
                  value={newGroupTags}
                  onChange={(e) => setNewGroupTags(e.target.value)}
                  required
                />
                <button type="submit" className="btn-primary btn-xs">Save</button>
              </form>
            )}

            {limitAnalysis.exceededAccounts.length > 0 && (
              <div className="alert-banner alert-warning">
                Too long for:{' '}
                {limitAnalysis.exceededAccounts
                  .map(({ account, count, limit }) => `${account.name} (${count}/${limit})`)
                  .join(', ')}
              </div>
            )}
          </div>

          {showFirstComment && (
            <div className="first-comment-box">
              <span className="section-eyebrow">First Comment (auto-reply)</span>
              <textarea
                className="composer-textarea first-comment-textarea"
                rows={2}
                placeholder="Add links, hashtags, or context as an automatic first reply..."
                value={firstComment}
                onChange={(e) => onChangeFirstComment(e.target.value)}
              />
            </div>
          )}

          <div
            className={`media-dropzone ${isDragging ? 'dragging' : ''}`}
            onDragOver={(e) => {
              e.preventDefault();
              setIsDragging(true);
            }}
            onDragLeave={() => setIsDragging(false)}
            onDrop={(e) => {
              e.preventDefault();
              setIsDragging(false);
              void handleFilesSelected(e.dataTransfer.files);
            }}
          >
            <input
              ref={fileInputRef}
              type="file"
              accept="image/jpeg,image/png,image/webp,image/gif"
              multiple
              hidden
              onChange={(e) => void handleFilesSelected(e.target.files)}
            />

            <div className="dropzone-header">
              <span className="muted-text">
                Images ({media.length}/4) · Drop files or add
              </span>
              <button
                type="button"
                className="btn-tonal btn-xs"
                onClick={() => fileInputRef.current?.click()}
                disabled={media.length >= 4 || uploading}
              >
                {uploading ? 'Optimizing...' : '+ Add Image'}
              </button>
            </div>

            {uploadError && <div className="alert-banner alert-error">{uploadError}</div>}

            {media.length > 0 && (
              <div className="media-attachments-grid">
                {media.map((item, idx) => (
                  <div key={item.id} className="media-attachment-card">
                    <div className="media-thumb-wrap">
                      <img src={item.url} alt={item.altText || item.filename} />
                      <div className="media-overlay-btns">
                        {idx > 0 && (
                          <button
                            type="button"
                            className="media-mini-btn"
                            onClick={() => handleMoveMedia(idx, -1)}
                            title="Move left"
                          >
                            ←
                          </button>
                        )}
                        {idx < media.length - 1 && (
                          <button
                            type="button"
                            className="media-mini-btn"
                            onClick={() => handleMoveMedia(idx, 1)}
                            title="Move right"
                          >
                            →
                          </button>
                        )}
                        <button
                          type="button"
                          className="media-mini-btn"
                          onClick={() => handleRemoveMedia(item.id)}
                          title="Remove"
                        >
                          ×
                        </button>
                      </div>
                      <span className="media-size-badge">
                        {item.wasCompressed && item.originalSize
                          ? `${formatBytes(item.originalSize)} → ${formatBytes(item.sizeBytes)}`
                          : formatBytes(item.sizeBytes)}
                      </span>
                    </div>
                    <input
                      type="text"
                      className="alt-text-input"
                      placeholder="Alt text..."
                      value={item.altText}
                      onChange={(e) => handleUpdateAltText(item.id, e.target.value)}
                      maxLength={1000}
                    />
                  </div>
                ))}
              </div>
            )}
          </div>

          {publishError && <div className="alert-banner alert-error">{publishError}</div>}

          {showSchedulePicker && (
            <div className="schedule-picker-box">
              <div className="schedule-picker-row">
                <input
                  type="datetime-local"
                  className="text-input schedule-dt-input"
                  value={scheduleDateTime}
                  onChange={(e) => setScheduleDateTime(e.target.value)}
                />
                <button
                  type="button"
                  className="btn-primary btn-sm"
                  onClick={() => handleSaveMode('schedule')}
                  disabled={!canPublish}
                >
                  {savingMode === 'schedule' ? 'Saving...' : 'Confirm'}
                </button>
                <button
                  type="button"
                  className="btn-ghost btn-sm"
                  onClick={() => setShowSchedulePicker(false)}
                >
                  Cancel
                </button>
              </div>
            </div>
          )}

          <div className="composer-footer">
            <div className="composer-secondary-actions">
              <button
                type="button"
                className="btn-ghost btn-sm"
                onClick={() => handleSaveMode('draft')}
                disabled={!canSaveDraft}
              >
                {savingMode === 'draft' ? 'Saving...' : 'Draft'}
              </button>
              <button
                type="button"
                className="btn-tonal btn-sm"
                onClick={() => setShowSchedulePicker((v) => !v)}
                disabled={!canPublish}
              >
                Schedule
              </button>
              <button
                type="button"
                className="btn-tonal btn-sm"
                onClick={() => handleSaveMode('queue')}
                disabled={!canPublish}
                title={
                  scheduleInfo?.nextSlot
                    ? `Next: ${new Date(scheduleInfo.nextSlot).toLocaleString()}`
                    : 'Queue'
                }
              >
                {savingMode === 'queue' ? 'Queuing...' : 'Queue'}
              </button>
            </div>

            <button
              type="button"
              className="btn-primary publish-btn"
              onClick={handlePublish}
              disabled={!canPublish}
            >
              {publishing ? 'Posting...' : 'Post Now'}
            </button>
          </div>
        </div>

        {latestPost && (
          <div className="surface-card results-card">
            <div className="results-card-header">
              <h3>Results</h3>
              <span className="muted-text">
                {latestPost.results.filter((r) => r.status === 'success').length}/
                {latestPost.results.length}
              </span>
            </div>

            <div className="results-list">
              {latestPost.results.map((res) => {
                const meta = PLATFORMS[res.platform];
                const isSuccess = res.status === 'success';
                return (
                  <div key={res.accountId} className={`result-row ${isSuccess ? 'ok' : 'fail'}`}>
                    <div className="result-account">
                      <span
                        className="result-icon"
                        style={{ color: meta.color, backgroundColor: meta.softBg }}
                      >
                        <PlatformIcon platform={res.platform} size={15} />
                      </span>
                      <span className="result-name">{res.accountName}</span>
                    </div>

                    <div className="result-status-area">
                      {isSuccess ? (
                        <div className="result-success-group">
                          <span className="status-badge success">Sent</span>
                          {res.externalUrl && (
                            <a
                              href={res.externalUrl}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="external-link"
                            >
                              Open ↗
                            </a>
                          )}
                        </div>
                      ) : (
                        <div className="result-error-group">
                          <span className="result-error-msg" title={res.errorMessage}>
                            {res.errorMessage}
                          </span>
                          <button
                            type="button"
                            className="btn-tonal btn-xs"
                            onClick={() => handleRetryAccount(latestPost.id, res.accountId)}
                            disabled={retryingAccountId === res.accountId}
                          >
                            Retry
                          </button>
                        </div>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </section>

      <LivePreviewPanel
        selectedAccounts={selectedAccounts}
        masterText={masterText}
        firstComment={showFirstComment ? firstComment : ''}
        customizePerNetwork={customizePerNetwork}
        customTexts={customTexts}
        media={media}
      />
    </div>
  );
};
