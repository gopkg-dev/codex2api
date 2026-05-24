import type {
  AccountEventTrendPoint,
  AccountUsageDetail,
  AddAccountRequest,
  AddATAccountRequest,
  AddOpenAIResponsesAccountRequest,
  AddOpenAIResponsesAccountsRequest,
  AdminErrorResponse,
  APIKeysResponse,
  AccountsResponse,
  ChartAggregation,
  CreateAccountResponse,
  CreateAccountsResponse,
  CreateAPIKeyResponse,
  CreateAPIKeyRequest,
  FetchOpenAIResponsesModelsRequest,
  FetchOpenAIResponsesModelsResponse,
  HealthResponse,
  IPBansResponse,
  IPStatsSort,
  IPStatsWindow,
  IPUsageStatsResponse,
  MessageResponse,
  ModelSyncResponse,
  ModelsResponse,
  OAuthExchangeResponse,
  OAuthURLResponse,
  OpsErrorSummary,
  OpsOverviewResponse,
  PublicHomeResponse,
  PublicModelCheckResponse,
  SiteBranding,
  StatsResponse,
  CPAExportEntry,
  SystemSettings,
  UpdateAccountSchedulerRequest,
  UpdateAPIKeyRequest,
  UpdateOpenAIResponsesAccountRequest,
  UsageLogsResponse,
  UsageLogsPagedResponse,
  UsageStats,
  AccountGroup,
  AccountGroupsResponse,
  CreateAccountGroupRequest,
  UpdateAccountGroupRequest,
} from "./types";

const BASE = "/api/admin";
export const ADMIN_AUTH_REQUIRED_EVENT = "codex2api:admin-auth-required";
const ADMIN_AUTH_RESET_KEY = "admin_auth_reset_at";

export function getAdminKey(): string {
  return localStorage.getItem("admin_key") ?? "";
}

export function clearAdminKey() {
  localStorage.removeItem("admin_key");
}

export function setAdminKey(key: string) {
  if (key) {
    localStorage.setItem("admin_key", key);
  } else {
    clearAdminKey();
  }
}

export function resetAdminAuthState() {
  clearAdminKey();
  localStorage.setItem(ADMIN_AUTH_RESET_KEY, String(Date.now()));
  window.dispatchEvent(new Event(ADMIN_AUTH_REQUIRED_EVENT));
}

function extractAdminErrorMessage(body: string, status: number): string {
  if (!body.trim()) {
    return `HTTP ${status}`;
  }

  try {
    const parsed = JSON.parse(body) as Partial<AdminErrorResponse>;
    if (typeof parsed.error === "string" && parsed.error.trim()) {
      return parsed.error;
    }
  } catch {
    // ignore JSON parse error and fall back to raw text
  }

  return body;
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  if (
    options.body !== undefined &&
    options.body !== null &&
    !headers.has("Content-Type")
  ) {
    headers.set("Content-Type", "application/json");
  }

  const adminKey = getAdminKey();
  if (adminKey) {
    headers.set("X-Admin-Key", adminKey);
  }

  const res = await fetch(BASE + path, {
    ...options,
    cache: options.cache ?? "no-store",
    headers,
  });

  if (!res.ok) {
    const body = await res.text();
    if (res.status === 401) {
      resetAdminAuthState();
    }
    throw new Error(extractAdminErrorMessage(body, res.status));
  }

  return (await res.json()) as T;
}

async function requestPublic<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const res = await fetch(path, {
    ...options,
    cache: options.cache ?? "no-store",
  });

  if (!res.ok) {
    const body = await res.text();
    throw new Error(extractAdminErrorMessage(body, res.status));
  }

  return (await res.json()) as T;
}

async function requestBlob(
  path: string,
  options: RequestInit = {},
): Promise<Blob> {
  const headers = new Headers(options.headers);

  const adminKey = getAdminKey();
  if (adminKey) {
    headers.set("X-Admin-Key", adminKey);
  }

  const res = await fetch(BASE + path, {
    ...options,
    cache: options.cache ?? "no-store",
    headers,
  });

  if (!res.ok) {
    const body = await res.text();
    if (res.status === 401) {
      resetAdminAuthState();
    }
    throw new Error(extractAdminErrorMessage(body, res.status));
  }

  return res.blob();
}

function buildOpsErrorSearchParams(params: {
  start: string;
  end: string;
  status?: string;
  errorKind?: string;
  endpoint?: string;
  apiKeyId?: string;
  stream?: string;
  fast?: string;
  q?: string;
  dedupe?: boolean;
  excludeStatus?: string;
}) {
  const search = new URLSearchParams();
  search.set("start", params.start);
  search.set("end", params.end);
  if (params.status) search.set("status", params.status);
  if (params.errorKind) search.set("error_kind", params.errorKind);
  if (params.endpoint) search.set("endpoint", params.endpoint);
  if (params.apiKeyId) search.set("api_key_id", params.apiKeyId);
  if (params.stream) search.set("stream", params.stream);
  if (params.fast) search.set("fast", params.fast);
  if (params.q) search.set("q", params.q);
  if (typeof params.dedupe === "boolean")
    search.set("dedupe", String(params.dedupe));
  if (params.excludeStatus) search.set("exclude_status", params.excludeStatus);
  return search;
}

export const api = {
  getBranding: () => requestPublic<SiteBranding>("/api/branding"),
  getPublicHome: (params?: { ipWindow?: IPStatsWindow }) => {
    const search = new URLSearchParams();
    if (params?.ipWindow) search.set("ip_window", params.ipWindow);
    const suffix = search.toString() ? `?${search.toString()}` : "";
    return requestPublic<PublicHomeResponse>(`/api/public/home${suffix}`);
  },
  getPublicChartData: (params: {
    start: string;
    end: string;
    bucketMinutes: number;
  }) =>
    requestPublic<ChartAggregation>(
      `/api/public/chart-data?start=${encodeURIComponent(params.start)}&end=${encodeURIComponent(params.end)}&bucket_minutes=${params.bucketMinutes}`,
    ),
  getPublicIPBans: (params?: { ip?: string }) => {
    const search = new URLSearchParams();
    const ip = params?.ip?.trim();
    if (ip) search.set("ip", ip);
    const suffix = search.toString() ? `?${search.toString()}` : "";
    return requestPublic<IPBansResponse>(`/api/public/ip-bans${suffix}`);
  },
  checkPublicModels: (data: { base_url: string; api_key: string }) =>
    requestPublic<PublicModelCheckResponse>("/api/public/model-check", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    }),
  getStats: () => request<StatsResponse>("/stats"),
  getAccounts: () => request<AccountsResponse>("/accounts"),
  addAccount: (data: AddAccountRequest) =>
    request<CreateAccountResponse>("/accounts", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  addATAccount: (data: AddATAccountRequest) =>
    request<CreateAccountResponse>("/accounts/at", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  addOpenAIResponsesAccount: (data: AddOpenAIResponsesAccountRequest) =>
    request<CreateAccountResponse>("/accounts/openai-responses", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  addOpenAIResponsesAccounts: (data: AddOpenAIResponsesAccountsRequest) =>
    request<CreateAccountsResponse>("/accounts/openai-responses/batch", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  fetchOpenAIResponsesModels: (data: FetchOpenAIResponsesModelsRequest) =>
    request<FetchOpenAIResponsesModelsResponse>(
      "/accounts/openai-responses/models",
      { method: "POST", body: JSON.stringify(data) },
    ),
  updateOpenAIResponsesAccount: (
    id: number,
    data: UpdateOpenAIResponsesAccountRequest,
  ) =>
    request<MessageResponse>(`/accounts/${id}/openai-responses`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),
  deleteAccount: (id: number) =>
    request<MessageResponse>(`/accounts/${id}`, { method: "DELETE" }),
  refreshAccount: (id: number) =>
    request<MessageResponse>(`/accounts/${id}/refresh`, { method: "POST" }),
  forceUsageProbe: () =>
    request<{ triggered: boolean; concurrency: number; reason?: string }>(
      "/accounts/usage/probe",
      { method: "POST" },
    ),
  updateAccountScheduler: (id: number, data: UpdateAccountSchedulerRequest) =>
    request<MessageResponse>(`/accounts/${id}/scheduler`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),
  listAccountGroups: () => request<AccountGroupsResponse>("/account-groups"),
  createAccountGroup: (data: CreateAccountGroupRequest) =>
    request<{ id: number; message: string }>("/account-groups", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  updateAccountGroup: (id: number, data: UpdateAccountGroupRequest) =>
    request<MessageResponse>(`/account-groups/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),
  deleteAccountGroup: (id: number, force = false) =>
    request<MessageResponse>(
      `/account-groups/${id}${force ? "?force=true" : ""}`,
      { method: "DELETE" },
    ),
  toggleAccountEnabled: (id: number, enabled: boolean) =>
    request<MessageResponse>(`/accounts/${id}/enable`, {
      method: "POST",
      body: JSON.stringify({ enabled }),
    }),
  toggleAccountLock: (id: number, locked: boolean) =>
    request<MessageResponse>(`/accounts/${id}/lock`, {
      method: "POST",
      body: JSON.stringify({ locked }),
    }),
  resetAccountStatus: (id: number) =>
    request<MessageResponse>(`/accounts/${id}/reset-status`, {
      method: "POST",
    }),
  batchResetStatus: (ids: number[]) =>
    request<{ message: string; success: number; failed: number }>(
      "/accounts/batch-reset-status",
      { method: "POST", body: JSON.stringify({ ids }) },
    ),
  getAccountUsage: (id: number) =>
    request<AccountUsageDetail>(`/accounts/${id}/usage`),
  updateAccountCredit: (
    id: number,
    data: { credit_enabled: boolean; credit_skip_usage_window: boolean },
  ) =>
    request<MessageResponse>(`/accounts/${id}/credit`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),
  getHealth: () => request<HealthResponse>("/health"),
  getOpsOverview: (params?: { ipWindow?: IPStatsWindow }) => {
    const search = new URLSearchParams();
    if (params?.ipWindow) search.set("ip_window", params.ipWindow);
    const suffix = search.toString() ? `?${search.toString()}` : "";
    return request<OpsOverviewResponse>(`/ops/overview${suffix}`);
  },
  getOpsErrorSummary: (params: {
    start: string;
    end: string;
    status?: string;
    errorKind?: string;
    endpoint?: string;
    apiKeyId?: string;
    stream?: string;
    fast?: string;
    q?: string;
  }) => {
    const search = buildOpsErrorSearchParams(params);
    return request<OpsErrorSummary>(`/ops/errors/summary?${search.toString()}`);
  },
  getOpsErrors: (params: {
    start: string;
    end: string;
    page: number;
    pageSize?: number;
    status?: string;
    errorKind?: string;
    endpoint?: string;
    apiKeyId?: string;
    stream?: string;
    fast?: string;
    q?: string;
  }) => {
    const search = buildOpsErrorSearchParams(params);
    search.set("page", String(params.page));
    if (params.pageSize) search.set("page_size", String(params.pageSize));
    return request<UsageLogsPagedResponse>(`/ops/errors?${search.toString()}`);
  },
  downloadOpsErrors: (params: {
    start: string;
    end: string;
    status?: string;
    errorKind?: string;
    endpoint?: string;
    apiKeyId?: string;
    stream?: string;
    fast?: string;
    q?: string;
    dedupe?: boolean;
    excludeStatus?: string;
  }) => {
    const search = buildOpsErrorSearchParams(params);
    return requestBlob(`/ops/errors/export?${search.toString()}`);
  },
  getUsageStats: (params: { start?: string; end?: string } = {}) => {
    const searchParams = new URLSearchParams();
    if (params.start) searchParams.set("start", params.start);
    if (params.end) searchParams.set("end", params.end);
    const qs = searchParams.toString();
    return request<UsageStats>(qs ? `/usage/stats?${qs}` : "/usage/stats");
  },
  getUsageLogs: (
    params: { start?: string; end?: string; limit?: number } = {},
  ) => {
    const searchParams = new URLSearchParams();
    if (params.start && params.end) {
      searchParams.set("start", params.start);
      searchParams.set("end", params.end);
    } else if (params.limit) {
      searchParams.set("limit", String(params.limit));
    }
    return request<UsageLogsResponse>(`/usage/logs?${searchParams.toString()}`);
  },
  getUsageLogsPaged: (params: {
    start: string;
    end: string;
    page: number;
    pageSize?: number;
    email?: string;
    model?: string;
    endpoint?: string;
    apiKeyId?: string;
    fast?: string;
    stream?: string;
  }) => {
    const searchParams = new URLSearchParams();
    searchParams.set("start", params.start);
    searchParams.set("end", params.end);
    searchParams.set("page", String(params.page));
    if (params.pageSize) searchParams.set("page_size", String(params.pageSize));
    if (params.email) searchParams.set("email", params.email);
    if (params.model) searchParams.set("model", params.model);
    if (params.endpoint) searchParams.set("endpoint", params.endpoint);
    if (params.apiKeyId) searchParams.set("api_key_id", params.apiKeyId);
    if (params.fast) searchParams.set("fast", params.fast);
    if (params.stream) searchParams.set("stream", params.stream);
    return request<UsageLogsPagedResponse>(
      `/usage/logs?${searchParams.toString()}`,
    );
  },
  getChartData: (params: {
    start: string;
    end: string;
    bucketMinutes: number;
  }) => {
    const searchParams = new URLSearchParams();
    searchParams.set("start", params.start);
    searchParams.set("end", params.end);
    searchParams.set("bucket_minutes", String(params.bucketMinutes));
    return request<ChartAggregation>(
      `/usage/chart-data?${searchParams.toString()}`,
    );
  },
  getAccountEventTrend: (params: {
    start: string;
    end: string;
    bucketMinutes: number;
  }) => {
    const sp = new URLSearchParams();
    sp.set("start", params.start);
    sp.set("end", params.end);
    sp.set("bucket_minutes", String(params.bucketMinutes));
    return request<{ trend: AccountEventTrendPoint[] }>(
      `/accounts/event-trend?${sp.toString()}`,
    );
  },
  getAPIKeys: () => request<APIKeysResponse>("/keys"),
  createAPIKey: (data: CreateAPIKeyRequest) =>
    request<CreateAPIKeyResponse>("/keys", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  deleteAPIKey: (id: number) =>
    request<MessageResponse>(`/keys/${id}`, { method: "DELETE" }),
  updateAPIKey: (id: number, data: UpdateAPIKeyRequest) =>
    request<MessageResponse>(`/keys/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),
  clearUsageLogs: () =>
    request<MessageResponse>("/usage/logs", { method: "DELETE" }),
  getSettings: () => request<SystemSettings>("/settings"),
  updateSettings: (data: Partial<SystemSettings>) =>
    request<SystemSettings>("/settings", {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  getIPBans: (
    includeInactive = true,
    params?: { page?: number; pageSize?: number },
  ) => {
    const search = new URLSearchParams({
      include_inactive: includeInactive ? "1" : "0",
    });
    if (params?.page) search.set("page", String(params.page));
    if (params?.pageSize) search.set("page_size", String(params.pageSize));
    return request<IPBansResponse>(`/ip-bans?${search.toString()}`);
  },
  getIPUsageStats: (params?: {
    window?: IPStatsWindow;
    page?: number;
    pageSize?: number;
    sort?: IPStatsSort;
    order?: "asc" | "desc";
  }) => {
    const search = new URLSearchParams();
    if (params?.window) search.set("window", params.window);
    if (params?.page) search.set("page", String(params.page));
    if (params?.pageSize) search.set("page_size", String(params.pageSize));
    if (params?.sort) search.set("sort", params.sort);
    if (params?.order) search.set("order", params.order);
    const suffix = search.toString() ? `?${search.toString()}` : "";
    return request<IPUsageStatsResponse>(`/ip-stats${suffix}`);
  },
  createIPBan: (data: {
    ip: string;
    reason?: string;
    source?: string;
    expires_in_minutes?: number;
  }) =>
    request<{ ban: IPBansResponse["bans"][number] }>("/ip-bans", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  createIPBansBatch: (data: {
    ips: string[];
    reason?: string;
    source?: string;
    expires_in_minutes?: number;
  }) =>
    request<{
      bans: IPBansResponse["bans"];
      errors: Array<{ ip: string; error: string }>;
      created: number;
      error_count: number;
    }>("/ip-bans/batch", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  unbanIPBan: (id: number) =>
    request<MessageResponse>(`/ip-bans/${id}/unban`, { method: "POST" }),
  deleteIPBan: (id: number) =>
    request<MessageResponse>(`/ip-bans/${id}`, { method: "DELETE" }),
  getModels: () => request<ModelsResponse>("/models"),
  syncModels: () =>
    request<ModelSyncResponse>("/models/sync", { method: "POST" }),
  batchTestAccounts: (ids?: number[]) =>
    request<{
      total: number;
      success: number;
      failed: number;
      banned: number;
      rate_limited: number;
    }>("/accounts/batch-test", {
      method: "POST",
      body: ids ? JSON.stringify({ ids }) : undefined,
    }),
  cleanBanned: () =>
    request<{ message: string; cleaned: number }>("/accounts/clean-banned", {
      method: "POST",
    }),
  cleanRateLimited: () =>
    request<{ message: string; cleaned: number }>(
      "/accounts/clean-rate-limited",
      { method: "POST" },
    ),
  cleanError: () =>
    request<{ message: string; cleaned: number }>("/accounts/clean-error", {
      method: "POST",
    }),
  exportAccounts: (params: { filter: "healthy" | "all"; ids?: number[] }) => {
    const sp = new URLSearchParams({ filter: params.filter });
    if (params.ids && params.ids.length > 0)
      sp.set("ids", params.ids.join(","));
    return request<CPAExportEntry[]>(`/accounts/export?${sp.toString()}`);
  },
  downloadAccountAuthJSON: (id: number) =>
    requestBlob(`/accounts/${id}/auth-json`),
  migrateAccounts: (data: { url: string; admin_key: string }) =>
    request<{
      message: string;
      total: number;
      imported: number;
      duplicate: number;
      failed: number;
    }>("/accounts/migrate", { method: "POST", body: JSON.stringify(data) }),
  // Proxies
  listProxies: () => request<{ proxies: ProxyRow[] }>("/proxies"),
  addProxies: (data: { urls?: string[]; url?: string; label?: string }) =>
    request<{ message: string; inserted: number; total: number }>("/proxies", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  deleteProxy: (id: number) =>
    request<MessageResponse>(`/proxies/${id}`, { method: "DELETE" }),
  updateProxy: (
    id: number,
    data: { url?: string; label?: string; enabled?: boolean },
  ) =>
    request<MessageResponse>(`/proxies/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),
  batchDeleteProxies: (ids: number[]) =>
    request<{ message: string; deleted: number }>("/proxies/batch-delete", {
      method: "POST",
      body: JSON.stringify({ ids }),
    }),
  testProxy: (url: string, id?: number, lang?: string) =>
    request<ProxyTestResult>("/proxies/test", {
      method: "POST",
      body: JSON.stringify({ url, id, lang }),
    }),
  // OAuth
  generateOAuthURL: (data: { proxy_url?: string; redirect_uri?: string }) =>
    request<OAuthURLResponse>("/oauth/generate-auth-url", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  exchangeOAuthCode: (data: {
    session_id: string;
    code: string;
    state: string;
    name?: string;
    proxy_url?: string;
  }) =>
    request<OAuthExchangeResponse>("/oauth/exchange-code", {
      method: "POST",
      body: JSON.stringify(data),
    }),
};

export interface ProxyRow {
  id: number;
  url: string;
  label: string;
  enabled: boolean;
  created_at: string;
  test_ip: string;
  test_location: string;
  test_latency_ms: number;
}

export interface ProxyTestResult {
  success: boolean;
  ip?: string;
  country?: string;
  region?: string;
  city?: string;
  isp?: string;
  latency_ms?: number;
  location?: string;
  error?: string;
}
