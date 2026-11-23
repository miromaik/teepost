import React from 'react';
import { Account, getPlatformLimit, MediaItem, PLATFORMS } from '../api';
import { PlatformIcon } from './PlatformIcons';

interface LivePreviewPanelProps {
  selectedAccounts: Account[];
  masterText: string;
  firstComment: string;
  customizePerNetwork: boolean;
  customTexts: Record<string, string>;
  media: MediaItem[];
}

function renderFormattedText(text: string): React.ReactNode {
  if (!text.trim()) {
    return <span className="preview-placeholder">Preview appears as you type...</span>;
  }

  const tokenRegex = /(https?:\/\/[^\s]+|#[\p{L}\p{N}_]+|@[A-Za-z0-9_.]+)/gu;
  const parts = text.split(tokenRegex);

  return parts.map((part, idx) => {
    if (/^https?:\/\//.test(part)) {
      return <span key={idx} className="preview-token-link">{part}</span>;
    }
    if (/^#[\p{L}\p{N}_]+$/u.test(part) || /^@[A-Za-z0-9_.]+$/.test(part)) {
      return <span key={idx} className="preview-token-tag">{part}</span>;
    }
    return <React.Fragment key={idx}>{part}</React.Fragment>;
  });
}

export const LivePreviewPanel: React.FC<LivePreviewPanelProps> = ({
  selectedAccounts,
  masterText,
  firstComment,
  customizePerNetwork,
  customTexts,
  media,
}) => {
  return (
    <aside className="preview-column">
      <div className="preview-header">
        <h2>Preview</h2>
        <span className="preview-count-chip">{selectedAccounts.length}</span>
      </div>

      {selectedAccounts.length === 0 ? (
        <div className="surface-card preview-empty">
          <p className="muted-text">Select a channel to preview.</p>
        </div>
      ) : (
        <div className="preview-list">
          {selectedAccounts.map((acc) => {
            const meta = PLATFORMS[acc.platform];
            const text = customizePerNetwork
              ? customTexts[acc.id] ?? masterText
              : masterText;
            const limit = getPlatformLimit(acc.platform, media.length > 0);
            const isOver = text.length > limit;

            return (
              <article key={acc.id} className="surface-card preview-card">
                <div className="preview-card-head">
                  <div className="preview-author">
                    <span
                      className="preview-avatar"
                      style={{ backgroundColor: meta.softBg, color: meta.color }}
                    >
                      <PlatformIcon platform={acc.platform} size={15} />
                    </span>
                    <div>
                      <div className="preview-author-name">{acc.name}</div>
                      <div className="preview-author-meta">{meta.shortLabel}</div>
                    </div>
                  </div>
                  <span className={`char-badge ${isOver ? 'over' : ''}`}>
                    {text.length}/{limit}
                  </span>
                </div>

                <div className="preview-body">{renderFormattedText(text)}</div>

                {media.length > 0 && (
                  <div className={`preview-media-grid count-${Math.min(media.length, 4)}`}>
                    {media.slice(0, 4).map((item) => (
                      <div key={item.id} className="preview-media-tile">
                        <img src={item.url} alt={item.altText || item.filename} />
                        {item.altText && <span className="alt-pill">ALT</span>}
                      </div>
                    ))}
                  </div>
                )}

                {firstComment.trim() !== '' && (
                  <div className="preview-first-comment">
                    <span className="preview-reply-label">First Comment</span>
                    <div className="preview-body">{renderFormattedText(firstComment)}</div>
                  </div>
                )}
              </article>
            );
          })}
        </div>
      )}
    </aside>
  );
};
