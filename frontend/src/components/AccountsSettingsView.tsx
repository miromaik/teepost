import React, { useEffect, useState } from 'react';
import {
  Account,
  api,
  DisplayPreferences,
  PLATFORM_ORDER,
  PlatformId,
  PLATFORMS,
  SystemInfo,
  TestConnectionResponse,
  TIMEZONE_OPTIONS,
} from '../api';
import { PlatformIcon } from './PlatformIcons';

interface AccountsSettingsViewProps {
  accounts: Account[];
  onAccountsChanged: () => Promise<void> | void;
  authRequired: boolean;
  onAuthChanged: (nextAuthRequired: boolean) => void;
  preferences: DisplayPreferences;
  onPreferencesChanged: (prefs: DisplayPreferences) => void;
  initialPlatformModal?: PlatformId | null;
  onClearInitialModal?: () => void;
}

interface EditorState {
  mode: 'create' | 'edit';
  accountId?: string;
  platform: PlatformId;
  name: string;
  enabled: boolean;
  credentials: Record<string, string>;
}

export const AccountsSettingsView: React.FC<AccountsSettingsViewProps> = ({
  accounts,
  onAccountsChanged,
  authRequired,
  onAuthChanged,
  preferences,
  onPreferencesChanged,
  initialPlatformModal,
  onClearInitialModal,
}) => {
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [showHelp, setShowHelp] = useState(false);
  const [saving, setSaving] = useState(false);
  const [editorError, setEditorError] = useState<string | null>(null);
  const [editorTestResult, setEditorTestResult] = useState<TestConnectionResponse | null>(null);
  const [testingEditor, setTestingEditor] = useState(false);

  const [testingAccountId, setTestingAccountId] = useState<string | null>(null);
  const [savedTestResults, setSavedTestResults] = useState<Record<string, TestConnectionResponse>>({});
  const [globalError, setGlobalError] = useState<string | null>(null);

  const [newPassword, setNewPassword] = useState('');
  const [savingPassword, setSavingPassword] = useState(false);
  const [passwordMessage, setPasswordMessage] = useState<{ type: 'ok' | 'err'; text: string } | null>(null);

  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null);

  useEffect(() => {
    void api.getSystemInfo().then(setSystemInfo).catch(() => {});
  }, []);

  const openCreateModal = (platform: PlatformId) => {
    setEditorError(null);
    setEditorTestResult(null);
    setShowHelp(false);
    setEditor({
      mode: 'create',
      platform,
      name: '',
      enabled: true,
      credentials: {},
    });
  };

  useEffect(() => {
    if (initialPlatformModal) {
      openCreateModal(initialPlatformModal);
      onClearInitialModal?.();
    }
  }, [initialPlatformModal, onClearInitialModal]);

  const openEditModal = (account: Account) => {
    setEditorError(null);
    setEditorTestResult(null);
    setShowHelp(false);
    setEditor({
      mode: 'edit',
      accountId: account.id,
      platform: account.platform,
      name: account.name,
      enabled: account.enabled,
      credentials: { ...(account.credentials ?? {}) },
    });
  };

  const handleSaveEditor = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editor) return;
    setSaving(true);
    setEditorError(null);
    try {
      if (editor.mode === 'create') {
        await api.createAccount({
          platform: editor.platform,
          name: editor.name,
          enabled: editor.enabled,
          credentials: editor.credentials,
        });
      } else if (editor.accountId) {
        await api.updateAccount(editor.accountId, {
          platform: editor.platform,
          name: editor.name,
          enabled: editor.enabled,
          credentials: editor.credentials,
        });
      }
      await onAccountsChanged();
      setEditor(null);
    } catch (err) {
      setEditorError(err instanceof Error ? err.message : 'Failed to save channel.');
    } finally {
      setSaving(false);
    }
  };

  const handleTestEditorCredentials = async () => {
    if (!editor) return;
    setTestingEditor(true);
    setEditorTestResult(null);
    try {
      const res = await api.testUnsavedCredentials(editor.platform, editor.credentials);
      setEditorTestResult(res);
    } catch (err) {
      setEditorTestResult({
        ok: false,
        error: err instanceof Error ? err.message : 'Test failed',
      });
    } finally {
      setTestingEditor(false);
    }
  };

  const handleToggleEnabled = async (acc: Account) => {
    setGlobalError(null);
    try {
      await api.updateAccount(acc.id, {
        platform: acc.platform,
        name: acc.name,
        enabled: !acc.enabled,
        credentials: acc.credentials,
      });
      await onAccountsChanged();
    } catch (err) {
      setGlobalError(err instanceof Error ? err.message : 'Update failed.');
    }
  };

  const handleTestSaved = async (acc: Account) => {
    setTestingAccountId(acc.id);
    try {
      const res = await api.testSavedAccount(acc.id);
      setSavedTestResults((prev) => ({ ...prev, [acc.id]: res }));
    } catch (err) {
      setSavedTestResults((prev) => ({
        ...prev,
        [acc.id]: { ok: false, error: err instanceof Error ? err.message : 'Test failed' },
      }));
    } finally {
      setTestingAccountId(null);
    }
  };

  const handleDelete = async (acc: Account) => {
    setGlobalError(null);
    try {
      await api.deleteAccount(acc.id);
      await onAccountsChanged();
    } catch (err) {
      setGlobalError(err instanceof Error ? err.message : 'Delete failed.');
    }
  };

  const handleSavePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newPassword.trim()) return;
    setSavingPassword(true);
    setPasswordMessage(null);
    try {
      const res = await api.setAuthPassword({ enabled: true, password: newPassword });
      onAuthChanged(res.authRequired);
      setNewPassword('');
      setPasswordMessage({ type: 'ok', text: 'Password saved.' });
    } catch (err) {
      setPasswordMessage({
        type: 'err',
        text: err instanceof Error ? err.message : 'Failed to save password.',
      });
    } finally {
      setSavingPassword(false);
    }
  };

  const handleDisablePassword = async () => {
    setSavingPassword(true);
    setPasswordMessage(null);
    try {
      const res = await api.setAuthPassword({ enabled: false, password: '' });
      onAuthChanged(res.authRequired);
      setNewPassword('');
      setPasswordMessage({ type: 'ok', text: 'Password disabled.' });
    } catch (err) {
      setPasswordMessage({
        type: 'err',
        text: err instanceof Error ? err.message : 'Failed to disable password.',
      });
    } finally {
      setSavingPassword(false);
    }
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>Settings</h1>
        {systemInfo && (
          <span className="pastel-pill mint">
            v{systemInfo.version} · {systemInfo.timezone}
          </span>
        )}
      </div>

      {/* Minimal 1-line password bar + Time & Format */}
      <div className="settings-top-grid">
        <section className="surface-card security-card">
          <div className="security-card-head">
            <strong>Dashboard Password</strong>
            <span className={`enabled-pill ${authRequired ? 'on' : 'off'}`}>
              {authRequired ? 'Enabled' : 'Disabled'}
            </span>
          </div>

          <form onSubmit={handleSavePassword} className="security-form">
            <input
              type="password"
              className="text-input security-input"
              placeholder={authRequired ? 'New password...' : 'Set password...'}
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
            />
            <button
              type="submit"
              className="btn-primary btn-sm"
              disabled={savingPassword || !newPassword.trim()}
            >
              {authRequired ? 'Change' : 'Set Password'}
            </button>
            {authRequired && (
              <button
                type="button"
                className="btn-ghost btn-sm danger-text"
                onClick={handleDisablePassword}
                disabled={savingPassword}
              >
                Disable
              </button>
            )}
          </form>

          {passwordMessage && (
            <div className={`alert-banner ${passwordMessage.type === 'ok' ? 'alert-success' : 'alert-error'}`}>
              {passwordMessage.text}
            </div>
          )}
        </section>

        <section className="surface-card security-card">
          <div className="security-card-head">
            <strong>Time & Format</strong>
          </div>

          <div className="security-form">
            <select
              className="text-input security-input"
              value={preferences.timezone}
              onChange={(e) => {
                const next: DisplayPreferences = { ...preferences, timezone: e.target.value };
                void api.savePreferences(next).then(onPreferencesChanged).catch(() => {});
              }}
              aria-label="Timezone"
            >
              {TIMEZONE_OPTIONS.map((tz) => (
                <option key={tz} value={tz}>
                  {tz === 'Local' ? 'Browser / Server Local' : tz}
                </option>
              ))}
            </select>

            <div className="segmented-control">
              <button
                type="button"
                className={`seg-btn ${preferences.timeFormat === '24h' ? 'active' : ''}`}
                onClick={() => {
                  const next: DisplayPreferences = { ...preferences, timeFormat: '24h' };
                  void api.savePreferences(next).then(onPreferencesChanged).catch(() => {});
                }}
              >
                24h (14:30)
              </button>
              <button
                type="button"
                className={`seg-btn ${preferences.timeFormat === '12h' ? 'active' : ''}`}
                onClick={() => {
                  const next: DisplayPreferences = { ...preferences, timeFormat: '12h' };
                  void api.savePreferences(next).then(onPreferencesChanged).catch(() => {});
                }}
              >
                12h (2:30 PM)
              </button>
            </div>
          </div>
        </section>
      </div>

      {globalError && <div className="alert-banner alert-error">{globalError}</div>}

      <div className="platform-cards-grid">
        {PLATFORM_ORDER.map((platformId) => {
          const meta = PLATFORMS[platformId];
          const platformAccounts = accounts.filter((a) => a.platform === platformId);

          return (
            <section key={platformId} className="surface-card platform-section-card">
              <div className="platform-section-head">
                <div className="platform-title-group">
                  <span
                    className="platform-badge-icon"
                    style={{ backgroundColor: meta.softBg, color: meta.color }}
                  >
                    <PlatformIcon platform={platformId} size={20} />
                  </span>
                  <h2>{meta.label}</h2>
                </div>

                <button
                  type="button"
                  className="btn-tonal btn-sm"
                  onClick={() => openCreateModal(platformId)}
                >
                  + Connect
                </button>
              </div>

              {platformAccounts.length > 0 && (
                <div className="configured-accounts-list">
                  {platformAccounts.map((acc) => {
                    const testRes = savedTestResults[acc.id];
                    return (
                      <div key={acc.id} className={`configured-account-row ${!acc.enabled ? 'disabled' : ''}`}>
                        <div className="configured-account-main">
                          <span className="configured-account-name">{acc.name}</span>

                          <div className="configured-account-actions">
                            <button
                              type="button"
                              className="btn-ghost btn-xs"
                              onClick={() => handleTestSaved(acc)}
                              disabled={testingAccountId === acc.id}
                            >
                              {testingAccountId === acc.id ? '...' : 'Test'}
                            </button>
                            <button
                              type="button"
                              className="btn-ghost btn-xs"
                              onClick={() => handleToggleEnabled(acc)}
                            >
                              {acc.enabled ? 'On' : 'Off'}
                            </button>
                            <button
                              type="button"
                              className="btn-ghost btn-xs"
                              onClick={() => openEditModal(acc)}
                            >
                              Edit
                            </button>
                            <button
                              type="button"
                              className="btn-ghost btn-xs danger-text"
                              onClick={() => handleDelete(acc)}
                            >
                              ×
                            </button>
                          </div>
                        </div>

                        {testRes && (
                          <div className={`test-feedback ${testRes.ok ? 'ok' : 'fail'}`}>
                            {testRes.ok ? `OK (${testRes.latencyMs ?? 0} ms)` : testRes.error}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </section>
          );
        })}
      </div>

      {editor && (
        <div className="modal-backdrop" role="dialog" aria-modal="true">
          <div className="surface-card modal-card">
            <div className="modal-header">
              <div className="platform-title-group">
                <span
                  className="platform-badge-icon"
                  style={{
                    backgroundColor: PLATFORMS[editor.platform].softBg,
                    color: PLATFORMS[editor.platform].color,
                  }}
                >
                  <PlatformIcon platform={editor.platform} size={20} />
                </span>
                <h3>{PLATFORMS[editor.platform].label}</h3>
              </div>
              <div className="modal-header-actions">
                <button
                  type="button"
                  className="btn-ghost btn-xs"
                  onClick={() => setShowHelp((v) => !v)}
                >
                  ? Help
                </button>
                <button
                  type="button"
                  className="icon-btn"
                  onClick={() => setEditor(null)}
                  aria-label="Close"
                >
                  ×
                </button>
              </div>
            </div>

            {showHelp && (
              <div className="setup-guide-box">
                <ol className="setup-guide-list">
                  {PLATFORMS[editor.platform].setupSteps.map((step, i) => (
                    <li key={i}>{step}</li>
                  ))}
                </ol>
              </div>
            )}

            <form onSubmit={handleSaveEditor} className="modal-form">
              <div className="form-field">
                <label className="field-label" htmlFor="account-name">Name</label>
                <input
                  id="account-name"
                  type="text"
                  className="text-input"
                  placeholder={PLATFORMS[editor.platform].defaultNamePlaceholder}
                  value={editor.name}
                  onChange={(e) => setEditor({ ...editor, name: e.target.value })}
                  required
                />
              </div>

              {PLATFORMS[editor.platform].fields.map((field) => (
                <div key={field.key} className="form-field">
                  <label className="field-label" htmlFor={`cred-${field.key}`}>
                    {field.label}
                  </label>
                  <input
                    id={`cred-${field.key}`}
                    type={field.secret ? 'password' : 'text'}
                    className="text-input"
                    placeholder={field.placeholder}
                    value={editor.credentials[field.key] ?? ''}
                    onChange={(e) =>
                      setEditor({
                        ...editor,
                        credentials: { ...editor.credentials, [field.key]: e.target.value },
                      })
                    }
                    required={field.required}
                  />
                </div>
              ))}

              <label className="switch-control">
                <input
                  type="checkbox"
                  checked={editor.enabled}
                  onChange={(e) => setEditor({ ...editor, enabled: e.target.checked })}
                />
                <span className="switch-track" aria-hidden="true">
                  <span className="switch-thumb" />
                </span>
                <span className="switch-label">Enabled</span>
              </label>

              {editorTestResult && (
                <div className={`alert-banner ${editorTestResult.ok ? 'alert-success' : 'alert-error'}`}>
                  {editorTestResult.ok
                    ? `Connected (${editorTestResult.latencyMs ?? 0} ms)`
                    : editorTestResult.error}
                </div>
              )}

              {editorError && <div className="alert-banner alert-error">{editorError}</div>}

              <div className="modal-footer">
                <button
                  type="button"
                  className="btn-tonal"
                  onClick={handleTestEditorCredentials}
                  disabled={testingEditor}
                >
                  {testingEditor ? 'Testing...' : 'Test'}
                </button>
                <div className="modal-footer-right">
                  <button type="button" className="btn-ghost" onClick={() => setEditor(null)}>
                    Cancel
                  </button>
                  <button type="submit" className="btn-primary" disabled={saving}>
                    {saving ? 'Saving...' : 'Save'}
                  </button>
                </div>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
