export type PlatformId = 'x' | 'bluesky' | 'telegram' | 'discord' | 'mastodon';

export type PostStatus =
  | 'idea'
  | 'draft'
  | 'scheduled'
  | 'publishing'
  | 'sent'
  | 'failed';

export interface Account {
  id: string;
  platform: PlatformId;
  name: string;
  enabled: boolean;
  credentials: Record<string, string>;
  createdAt: string;
  updatedAt: string;
}

export interface MediaItem {
  id: string;
  storedName: string;
  filename: string;
  mimeType: string;
  sizeBytes: number;
  width: number;
  height: number;
  altText: string;
  url: string;
  wasCompressed?: boolean;
  originalSize?: number;
}

export interface PostResult {
  id: string;
  postId: string;
  accountId: string;
  platform: PlatformId;
  accountName: string;
  textSent: string;
  status: 'success' | 'error';
  externalId: string;
  externalUrl: string;
  errorMessage: string;
  publishedAt: string;
}

export interface Post {
  id: string;
  status: PostStatus;
  masterText: string;
  firstComment: string;
  customizePerNetwork: boolean;
  customTexts: Record<string, string>;
  media: MediaItem[];
  targetAccountIds: string[];
  tags: string[];
  scheduledAt: string;
  results: PostResult[];
  createdAt: string;
  updatedAt: string;
}

export interface PostingSlot {
  id: string;
  weekday: number;
  timeHhmm: string;
  createdAt?: string;
}

export interface ScheduleSlotsResponse {
  slots: PostingSlot[];
  nextSlot: string;
  timezone: string;
  paused: boolean;
}

export interface HashtagGroup {
  id: string;
  name: string;
  hashtags: string;
}

export interface SystemInfo {
  version: string;
  timezone: string;
  serverTime: string;
  dbPath: string;
  mediaDir: string;
}

export interface AuthStatus {
  authRequired: boolean;
  authenticated: boolean;
}

export interface DisplayPreferences {
  timezone: string;
  timeFormat: '24h' | '12h';
}

export const TIMEZONE_OPTIONS = [
  'Local',
  'UTC',
  'Europe/Warsaw',
  'Europe/London',
  'Europe/Berlin',
  'Europe/Paris',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'Asia/Tokyo',
  'Australia/Sydney',
];

export function formatDateTime(
  iso: string | Date,
  prefs?: DisplayPreferences | null,
  includeDate = true
): string {
  if (!iso) return '';
  try {
    const d = iso instanceof Date ? iso : new Date(iso);
    const hour12 = prefs?.timeFormat === '12h';
    const timeZone =
      prefs?.timezone && prefs.timezone !== 'Local' ? prefs.timezone : undefined;
    return new Intl.DateTimeFormat(hour12 ? 'en-US' : 'en-GB', {
      ...(includeDate ? { month: 'short', day: 'numeric', weekday: 'short' } : {}),
      hour: '2-digit',
      minute: '2-digit',
      hour12,
      ...(timeZone ? { timeZone } : {}),
    }).format(d);
  } catch {
    return String(iso);
  }
}

export interface TestConnectionResponse {
  ok: boolean;
  error?: string;
  latencyMs?: number;
}

export interface CredentialField {
  key: string;
  label: string;
  placeholder: string;
  secret?: boolean;
  required: boolean;
}

export interface PlatformMeta {
  id: PlatformId;
  label: string;
  shortLabel: string;
  color: string;
  softBg: string;
  charLimitText: number;
  charLimitMedia: number;
  defaultNamePlaceholder: string;
  setupSteps: string[];
  fields: CredentialField[];
}

export const PLATFORM_ORDER: PlatformId[] = [
  'discord',
  'bluesky',
  'telegram',
  'mastodon',
  'x',
];

export const PLATFORMS: Record<PlatformId, PlatformMeta> = {
  discord: {
    id: 'discord',
    label: 'Discord',
    shortLabel: 'Discord',
    color: '#5865F2',
    softBg: 'rgba(88, 101, 242, 0.12)',
    charLimitText: 2000,
    charLimitMedia: 2000,
    defaultNamePlaceholder: '#announcements',
    setupSteps: [
      'Channel Settings -> Integrations -> Webhooks -> New Webhook.',
      'Click "Copy Webhook URL" and paste it below.',
    ],
    fields: [
      {
        key: 'webhookUrl',
        label: 'Webhook URL',
        placeholder: 'https://discord.com/api/webhooks/...',
        secret: true,
        required: true,
      },
    ],
  },
  bluesky: {
    id: 'bluesky',
    label: 'Bluesky',
    shortLabel: 'Bluesky',
    color: '#0285FF',
    softBg: 'rgba(2, 133, 255, 0.12)',
    charLimitText: 300,
    charLimitMedia: 300,
    defaultNamePlaceholder: 'alice.bsky.social',
    setupSteps: [
      'Bluesky Settings -> Privacy & Security -> App Passwords.',
      'Create a new App Password and enter it with your handle.',
    ],
    fields: [
      {
        key: 'identifier',
        label: 'Handle or DID',
        placeholder: 'alice.bsky.social',
        required: true,
      },
      {
        key: 'appPassword',
        label: 'App Password',
        placeholder: 'xxxx-xxxx-xxxx-xxxx',
        secret: true,
        required: true,
      },
      {
        key: 'pdsUrl',
        label: 'PDS URL (optional)',
        placeholder: 'https://bsky.social',
        required: false,
      },
    ],
  },
  telegram: {
    id: 'telegram',
    label: 'Telegram',
    shortLabel: 'Telegram',
    color: '#229ED9',
    softBg: 'rgba(34, 158, 217, 0.12)',
    charLimitText: 4096,
    charLimitMedia: 1024,
    defaultNamePlaceholder: '@mychannel',
    setupSteps: [
      'Create a bot via @BotFather (/newbot) to get the Bot Token.',
      'Add the bot as an Admin in your channel, then enter @channel or Chat ID.',
    ],
    fields: [
      {
        key: 'botToken',
        label: 'Bot Token',
        placeholder: '123456789:ABCdef...',
        secret: true,
        required: true,
      },
      {
        key: 'chatId',
        label: 'Channel (@name) or Chat ID',
        placeholder: '@mychannel or -1001234567890',
        required: true,
      },
    ],
  },
  mastodon: {
    id: 'mastodon',
    label: 'Mastodon',
    shortLabel: 'Mastodon',
    color: '#6364FF',
    softBg: 'rgba(99, 100, 255, 0.12)',
    charLimitText: 500,
    charLimitMedia: 500,
    defaultNamePlaceholder: '@alice@mastodon.social',
    setupSteps: [
      'Preferences -> Development -> New application (write:statuses, write:media).',
      'Copy "Your access token" and paste it with your instance URL.',
    ],
    fields: [
      {
        key: 'instanceUrl',
        label: 'Instance URL',
        placeholder: 'https://mastodon.social',
        required: true,
      },
      {
        key: 'accessToken',
        label: 'Access Token',
        placeholder: 'Paste your access token',
        secret: true,
        required: true,
      },
    ],
  },
  x: {
    id: 'x',
    label: 'X (Twitter)',
    shortLabel: 'X',
    color: 'var(--x-brand)',
    softBg: 'var(--x-brand-soft)',
    charLimitText: 280,
    charLimitMedia: 280,
    defaultNamePlaceholder: '@handle',
    setupSteps: [
      'developer.x.com -> Project App -> User authentication: Read and write.',
      'Copy all 4 OAuth 1.0a keys from "Keys and tokens".',
    ],
    fields: [
      {
        key: 'apiKey',
        label: 'API Key',
        placeholder: 'Consumer Key',
        required: true,
      },
      {
        key: 'apiSecret',
        label: 'API Secret',
        placeholder: 'Consumer Secret',
        secret: true,
        required: true,
      },
      {
        key: 'accessToken',
        label: 'Access Token',
        placeholder: 'Access Token',
        required: true,
      },
      {
        key: 'accessTokenSecret',
        label: 'Access Token Secret',
        placeholder: 'Access Token Secret',
        secret: true,
        required: true,
      },
    ],
  },
};

export function getPlatformLimit(platform: PlatformId, hasMedia: boolean): number {
  const meta = PLATFORMS[platform];
  return hasMedia ? meta.charLimitMedia : meta.charLimitText;
}

async function requestJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    credentials: 'same-origin',
    ...init,
    headers: {
      ...(init?.body instanceof FormData ? {} : { 'Content-Type': 'application/json' }),
      ...(init?.headers ?? {}),
    },
  });

  if (!res.ok) {
    let message = `HTTP ${res.status}`;
    try {
      const errData = (await res.json()) as { error?: string };
      if (errData?.error) message = errData.error;
    } catch {
      // ignore non-json error bodies
    }
    throw new Error(message);
  }
  return (await res.json()) as T;
}

export const api = {
  getSystemInfo: () => requestJSON<SystemInfo>('/api/system/info'),

  getAuthStatus: () => requestJSON<AuthStatus>('/api/auth/status'),

  login: (password: string) =>
    requestJSON<{ authenticated: boolean }>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ password }),
    }),

  logout: () =>
    requestJSON<{ authenticated: boolean }>('/api/auth/logout', { method: 'POST' }),

  setAuthPassword: (payload: { enabled: boolean; password: string }) =>
    requestJSON<AuthStatus>('/api/auth/password', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  listAccounts: () => requestJSON<Account[]>('/api/accounts'),

  createAccount: (payload: {
    platform: PlatformId;
    name: string;
    enabled: boolean;
    credentials: Record<string, string>;
  }) =>
    requestJSON<Account>('/api/accounts', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  updateAccount: (
    id: string,
    payload: {
      platform: PlatformId;
      name: string;
      enabled: boolean;
      credentials: Record<string, string>;
    }
  ) =>
    requestJSON<Account>(`/api/accounts/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),

  deleteAccount: (id: string) =>
    requestJSON<{ deleted: boolean }>(`/api/accounts/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  testSavedAccount: (id: string) =>
    requestJSON<TestConnectionResponse>(`/api/accounts/${encodeURIComponent(id)}/test`, {
      method: 'POST',
    }),

  testUnsavedCredentials: (platform: PlatformId, credentials: Record<string, string>) =>
    requestJSON<TestConnectionResponse>('/api/accounts/test', {
      method: 'POST',
      body: JSON.stringify({ platform, credentials }),
    }),

  uploadMedia: (file: File) => {
    const form = new FormData();
    form.append('file', file);
    return requestJSON<MediaItem>('/api/media', { method: 'POST', body: form });
  },

  publishPost: (payload: {
    postId?: string;
    accountIds: string[];
    masterText: string;
    firstComment?: string;
    customizePerNetwork: boolean;
    customTexts: Record<string, string>;
    media: MediaItem[];
    tags?: string[];
  }) =>
    requestJSON<Post>('/api/publish', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  savePost: (payload: {
    id?: string;
    mode: 'draft' | 'idea' | 'schedule' | 'queue';
    accountIds: string[];
    masterText: string;
    firstComment?: string;
    customizePerNetwork: boolean;
    customTexts: Record<string, string>;
    media: MediaItem[];
    tags: string[];
    scheduledAt?: string;
  }) =>
    requestJSON<Post>('/api/posts/save', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  publishSavedPostNow: (id: string) =>
    requestJSON<Post>(`/api/posts/${encodeURIComponent(id)}/publish-now`, {
      method: 'POST',
    }),

  retryPostAccount: (postId: string, accountId: string) =>
    requestJSON<Post>(
      `/api/posts/${encodeURIComponent(postId)}/retry/${encodeURIComponent(accountId)}`,
      { method: 'POST' }
    ),

  listPosts: () => requestJSON<Post[]>('/api/posts'),

  deletePost: (id: string) =>
    requestJSON<{ deleted: boolean }>(`/api/posts/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  getScheduleSlots: () => requestJSON<ScheduleSlotsResponse>('/api/schedule-slots'),

  updateScheduleSlots: (slots: PostingSlot[]) =>
    requestJSON<ScheduleSlotsResponse>('/api/schedule-slots', {
      method: 'PUT',
      body: JSON.stringify({ slots }),
    }),

  setQueuePaused: (paused: boolean) =>
    requestJSON<{ paused: boolean }>('/api/queue/pause', {
      method: 'POST',
      body: JSON.stringify({ paused }),
    }),

  reorderQueue: (postId: string, direction: 'up' | 'down') =>
    requestJSON<Post[]>('/api/queue/reorder', {
      method: 'POST',
      body: JSON.stringify({ postId, direction }),
    }),

  shuffleQueue: () =>
    requestJSON<Post[]>('/api/queue/shuffle', { method: 'POST' }),

  listHashtagGroups: () => requestJSON<HashtagGroup[]>('/api/hashtag-groups'),

  saveHashtagGroups: (groups: HashtagGroup[]) =>
    requestJSON<HashtagGroup[]>('/api/hashtag-groups', {
      method: 'PUT',
      body: JSON.stringify({ groups }),
    }),

  getPreferences: () => requestJSON<DisplayPreferences>('/api/preferences'),

  savePreferences: (prefs: DisplayPreferences) =>
    requestJSON<DisplayPreferences>('/api/preferences', {
      method: 'PUT',
      body: JSON.stringify(prefs),
    }),
};
