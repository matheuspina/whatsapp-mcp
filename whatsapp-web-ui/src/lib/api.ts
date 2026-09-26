const getApiBaseUrl = () => {
  // One-origin deployments (the single Dockerfile, behind a reverse proxy) build with NEXT_PUBLIC_API_BASE_URL=/api.
  const configured = process.env.NEXT_PUBLIC_API_BASE_URL;
  if (configured) return configured.replace(/\/$/, "");
  if (typeof window === "undefined") return "http://localhost:8180/api";
  return `${window.location.protocol}//${window.location.hostname}:8180/api`;
};

// Pairing types
export interface PairResponse {
  success: boolean;
  code?: string;
  expires_in?: number;
  error?: string;
}

export interface PairingStatusResponse {
  success: boolean;
  in_progress: boolean;
  code?: string;
  expires_in?: number;
  complete: boolean;
  error?: string;
}

export interface ConnectionStatusResponse {
  success: boolean;
  connected: boolean;
  linked: boolean;
  jid?: string;
  uptime?: string;
  last_connected?: string;
  disconnected_for?: string;
  auto_reconnect_errors?: number;
}

export interface SyncStatusResponse {
  success: boolean;
  syncing: boolean;
  last_sync?: string;
  sync_progress: number;
  message_count: number;
  conversation_count: number;
  error?: string;
  recommendations?: string[];
}

// Media object storage (Cloudflare R2, S3, MinIO, ...)
export interface MediaStorageConfig {
  enabled: boolean;
  endpoint: string;
  region: string;
  bucket: string;
  access_key_id: string;
  path_style: boolean;
  prefix: string;
  keep_local: boolean;
  public_base_url: string;
  /** True when a secret access key is stored. The key itself never leaves the server. */
  secret_set: boolean;
}

export interface MediaStorageStatus {
  config: MediaStorageConfig;
  /** Where the active configuration comes from: environment variables lock the form. */
  source: "none" | "env" | "panel";
  active: boolean;
  warning?: string;
  last_error?: string;
  /** Files by state: local, pending_upload, uploading, uploaded, failed. */
  queue: Record<string, number>;
  workers: number;
}

/** What the form submits. An empty secret_access_key keeps the stored one. */
export type MediaStorageInput = Omit<MediaStorageConfig, "secret_set"> & { secret_access_key?: string };

// Webhook types
export interface WebhookTrigger {
  trigger_type: "all" | "chat_jid" | "instance_jid" | "sender" | "keyword" | "media_type";
  trigger_value: string;
  match_type: "exact" | "contains" | "regex";
  enabled: boolean;
}

export interface Webhook {
  id: string;
  name: string;
  webhook_url: string;
  secret_token?: string;
  enabled: boolean;
  triggers: WebhookTrigger[];
  created_at: string;
  updated_at?: string;
}

export interface WebhookLog {
  id: string;
  webhook_id: string;
  message_id: string;
  chat_jid: string;
  trigger_type: string;
  trigger_value: string;
  payload: string;
  response_status?: number;
  response_body?: string;
  delivered_at?: string;
  created_at: string;
  attempt_count: number;
}

export interface WebhooksResponse {
  success: boolean;
  data?: Webhook[];
  error?: string;
}

export interface WebhookResponse {
  success: boolean;
  data?: Webhook;
  error?: string;
}

export interface WebhookLogsResponse {
  success: boolean;
  data?: WebhookLog[];
  error?: string;
}

// Organization & Multi-Instance types
export interface Department {
  id: number;
  name: string;
  description: string;
  created_at: string;
  updated_at: string;
}

export interface Employee {
  id: number;
  name: string;
  role: string;
  email?: string;
  active: boolean;
  department_id?: number | null;
  department_name?: string | null;
  created_at: string;
  updated_at: string;
}

export type InstanceStatus = "pairing" | "connected" | "disconnected" | "logged_out" | "removed";

/** A monitored WhatsApp number. `status` is what the bridge last recorded; `live` is its connection right now. */
export interface Instance {
  id: number;
  phone_jid?: string;
  phone_number?: string;
  alias?: string;
  status: InstanceStatus;
  live: boolean;
  employee_id?: number | null;
  employee_name?: string;
  department_id?: number | null;
  department_name?: string;
  allow_send: boolean;
  corporate_asset_confirmed: boolean;
  corporate_terms_version?: string;
  corporate_confirmed_by?: string;
  corporate_confirmed_at?: string;
  paired_at?: string;
  last_seen_at?: string;
  created_at: string;
  updated_at: string;
}

export interface InstancePairRequest {
  alias: string;
  employee_id?: number | null;
  allow_send: boolean;
  corporate_asset_confirmed: boolean;
}

export interface InstancePairResponse {
  success: boolean;
  instance?: Instance;
  qr_code?: string;
  message?: string;
  error?: string;
}

export type PairingState = "pending" | "paired" | "expired";

export interface InstanceQRResponse {
  success: boolean;
  status: PairingState;
  qr_code?: string;
  instance?: Instance;
}

export interface InstanceUpdate {
  alias?: string;
  employee_id?: number | null;
  allow_send?: boolean;
  corporate_asset_confirmed?: boolean;
}

export interface ChatItem {
  jid: string;
  name?: string;
  last_message_time: string;
  is_group: boolean;
  unread_count?: number;
}

export interface MessageItem {
  id: string;
  chat_jid: string;
  sender: string;
  sender_name?: string;
  content: string;
  timestamp: string;
  is_from_me: boolean;
  media_type?: string;
  instance_jid?: string;
  is_deleted_remote?: boolean;
  is_edited?: boolean;
}

export interface MessageConversation {
  instance_jid?: string;
  instance_alias?: string;
  chat_jid: string;
  chat_name?: string;
  is_group: boolean;
  last_message?: string;
  last_sender_name?: string;
  last_message_time: string;
  last_is_from_me?: boolean;
  message_count: number;
  employee_id?: number;
  employee_name?: string;
  department_id?: number;
  department_name?: string;
}

/** A captured message with the number, person and department that held it when it was exchanged. */
export interface FeedMessage {
  id: string;
  chat_jid: string;
  chat_name?: string;
  sender: string;
  sender_name?: string;
  content: string;
  timestamp: string;
  is_from_me: boolean;
  media_type?: string;
  instance_jid?: string;
  instance_alias?: string;
  employee_id?: number;
  employee_name?: string;
  department_id?: number;
  department_name?: string;
  is_edited?: boolean;
  is_deleted_remote: boolean;
  deleted_at?: string;
  deleted_by?: string;
}

export interface FeedFilters {
  instance?: string;
  employee_id?: number;
  department_id?: number;
  chat_jid?: string;
  q?: string;
  deleted_only?: boolean;
  since?: string;
  until?: string;
  before?: string;
  limit?: number;
}

export interface MessageVersion {
  id: number;
  content: string;
  reason: "edit" | "delete";
  recorded_at: string;
}

export interface AccessLogEntry {
  id: number;
  ts: string;
  actor?: string;
  client_id?: string;
  action: string;
  resource?: string;
  params?: string;
  result_count?: number;
}

export interface PrivacyLogEntry {
  id: number;
  ts: string;
  actor?: string;
  action: string;
  subject?: string;
  details?: string;
}

// Fired when the bridge rejects a request because the session is missing or expired.
export const UNAUTHORIZED_EVENT = "wa:unauthorized";

// Endpoints where a 401 is an expected answer rather than a lost session.
const AUTH_PROBE_ENDPOINTS = ["/auth/login", "/auth/me", "/auth/logout"];

// Session details as reported by the bridge. The token itself never reaches the browser:
// it lives in an HttpOnly cookie set by the server.
export interface AuthUser {
  username: string;
  expires_at: string;
}

export interface ActiveSession {
  id: string;
  username: string;
  created_at: string;
  last_seen_at: string;
  expires_at: string;
  ip: string;
  user_agent: string;
  current: boolean;
}

export class WhatsAppAPI {
  private baseUrl: string;

  constructor() {
    this.baseUrl = getApiBaseUrl();
  }

  private async request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
    const response = await fetch(`${this.baseUrl}${endpoint}`, {
      ...options,
      // The session is an HttpOnly cookie; the browser attaches it, scripts never touch it.
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        ...options.headers,
      },
    });

    // Errors from proxies or older bridges may not be JSON; never let that mask the status code.
    let data: { error?: string } & Record<string, unknown> = {};
    try {
      data = await response.json();
    } catch {
      /* non-JSON body */
    }

    if (!response.ok) {
      if (response.status === 401 && typeof window !== "undefined" && !AUTH_PROBE_ENDPOINTS.includes(endpoint)) {
        window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
      }
      throw new APIError(response.status, (data.error as string) || response.statusText || "Request failed");
    }

    return data as T;
  }

  // Authentication methods
  async login(username: string, password: string): Promise<AuthUser> {
    const res = await this.request<{ data: AuthUser }>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    return res.data;
  }

  async logout(): Promise<void> {
    await this.request("/auth/logout", { method: "POST" });
  }

  async me(): Promise<AuthUser> {
    const res = await this.request<{ data: AuthUser }>("/auth/me");
    return res.data;
  }

  async listSessions(): Promise<ActiveSession[]> {
    const res = await this.request<{ data: ActiveSession[] }>("/auth/sessions");
    return res.data || [];
  }

  async revokeSession(id: string): Promise<void> {
    await this.request(`/auth/sessions/${encodeURIComponent(id)}`, { method: "DELETE" });
  }

  // Media object storage settings
  async getMediaStorage(): Promise<MediaStorageStatus> {
    const res = await this.request<{ data: MediaStorageStatus }>("/settings/media-storage");
    return res.data;
  }

  async saveMediaStorage(input: MediaStorageInput): Promise<MediaStorageStatus> {
    const res = await this.request<{ data: MediaStorageStatus }>("/settings/media-storage", {
      method: "PUT",
      body: JSON.stringify(input),
    });
    return res.data;
  }

  async testMediaStorage(input: MediaStorageInput): Promise<string> {
    const res = await this.request<{ message?: string }>("/settings/media-storage/test", {
      method: "POST",
      body: JSON.stringify(input),
    });
    return res.message || "Connection works";
  }

  async retryFailedMedia(): Promise<number> {
    const res = await this.request<{ queued: number }>("/settings/media-storage/retry", { method: "POST" });
    return res.queued;
  }

  // Pairing methods
  async pair(phoneNumber: string): Promise<PairResponse> {
    return this.request<PairResponse>("/pair", {
      method: "POST",
      body: JSON.stringify({ phone_number: phoneNumber }),
    });
  }

  async getPairingStatus(): Promise<PairingStatusResponse> {
    return this.request<PairingStatusResponse>("/pairing");
  }

  async getConnectionStatus(): Promise<ConnectionStatusResponse> {
    return this.request<ConnectionStatusResponse>("/connection");
  }

  async getSyncStatus(): Promise<SyncStatusResponse> {
    return this.request<SyncStatusResponse>("/sync-status");
  }

  async reconnect(): Promise<void> {
    await this.request("/reconnect", { method: "POST" });
  }

  // Webhook methods
  async getWebhooks(): Promise<Webhook[]> {
    const response = await this.request<WebhooksResponse>("/webhooks");
    return response.data || [];
  }

  async createWebhook(webhook: Omit<Webhook, "id" | "created_at" | "updated_at">): Promise<Webhook> {
    const response = await this.request<WebhookResponse>("/webhooks", {
      method: "POST",
      body: JSON.stringify(webhook),
    });
    return response.data!;
  }

  async updateWebhook(id: string, webhook: Partial<Webhook>): Promise<Webhook> {
    const response = await this.request<WebhookResponse>(`/webhooks/${id}`, {
      method: "PUT",
      body: JSON.stringify(webhook),
    });
    return response.data!;
  }

  async deleteWebhook(id: string): Promise<void> {
    await this.request(`/webhooks/${id}`, { method: "DELETE" });
  }

  async toggleWebhook(id: string, enabled: boolean): Promise<void> {
    await this.request(`/webhooks/${id}/enable`, {
      method: "POST",
      body: JSON.stringify({ enabled }),
    });
  }

  async testWebhook(id: string): Promise<void> {
    await this.request(`/webhooks/${id}/test`, { method: "POST" });
  }

  async getWebhookLogs(id: string): Promise<WebhookLog[]> {
    const response = await this.request<WebhookLogsResponse>(`/webhooks/${id}/logs`);
    return response.data || [];
  }

  // Organization methods
  async getDepartments(): Promise<Department[]> {
    const res = await this.request<{ success: boolean; departments?: Department[] }>("/departments");
    return res.departments || [];
  }

  async createDepartment(data: { name: string; description?: string }): Promise<Department> {
    const res = await this.request<{ success: boolean; department: Department }>("/departments", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return res.department;
  }

  async updateDepartment(id: number, data: { name: string; description?: string }): Promise<Department> {
    const res = await this.request<{ success: boolean; department: Department }>(`/departments/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
    return res.department;
  }

  async deleteDepartment(id: number): Promise<void> {
    await this.request(`/departments/${id}`, { method: "DELETE" });
  }

  // Employee methods
  async getEmployees(params?: { department_id?: number; q?: string }): Promise<Employee[]> {
    let query = "";
    if (params) {
      const search = new URLSearchParams();
      if (params.department_id) search.set("department_id", params.department_id.toString());
      if (params.q) search.set("q", params.q);
      const str = search.toString();
      if (str) query = `?${str}`;
    }
    const res = await this.request<{ success: boolean; employees?: Employee[] }>(`/employees${query}`);
    return res.employees || [];
  }

  async createEmployee(data: { name: string; role?: string; email?: string; department_id?: number | null }): Promise<Employee> {
    const res = await this.request<{ success: boolean; employee: Employee }>("/employees", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return res.employee;
  }

  /** Partial update: only the fields given change. `department_id: null` removes the department. */
  async updateEmployee(
    id: number,
    data: { name?: string; role?: string; email?: string; active?: boolean; department_id?: number | null }
  ): Promise<Employee> {
    const res = await this.request<{ success: boolean; employee: Employee }>(`/employees/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
    return res.employee;
  }

  async deleteEmployee(id: number): Promise<void> {
    await this.request(`/employees/${id}`, { method: "DELETE" });
  }

  // Multi-Instance methods
  async getInstances(includeRemoved = false): Promise<Instance[]> {
    const res = await this.request<{ success: boolean; instances?: Instance[] }>(
      `/instances${includeRemoved ? "?include_removed=true" : ""}`
    );
    return res.instances || [];
  }

  /** Starts pairing a new number. The response carries the instance (status "pairing") and the first QR code. */
  async pairInstance(data: InstancePairRequest): Promise<InstancePairResponse> {
    return this.request<InstancePairResponse>("/instances", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  /** The current QR code of a pairing (WhatsApp rotates it every ~20 seconds) and whether it finished. */
  async getInstanceQR(id: number): Promise<InstanceQRResponse> {
    return this.request<InstanceQRResponse>(`/instances/${id}/qr`);
  }

  async updateInstance(id: number, data: InstanceUpdate): Promise<Instance> {
    const res = await this.request<{ success: boolean; instance: Instance }>(`/instances/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
    return res.instance;
  }

  async reconnectInstance(id: number): Promise<void> {
    await this.request(`/instances/${id}/reconnect`, { method: "POST" });
  }

  async disconnectInstance(id: number): Promise<void> {
    await this.request(`/instances/${id}/disconnect`, { method: "POST" });
  }

  /** Retires the number and drops its session. Messages already captured are kept. */
  async removeInstance(id: number): Promise<void> {
    await this.request(`/instances/${id}`, { method: "DELETE" });
  }

  // Chats and Messages
  async getChats(instance?: string): Promise<ChatItem[]> {
    const res = await this.request<{ success: boolean; chats?: ChatItem[] }>(
      `/chats${instance ? `?instance=${encodeURIComponent(instance)}` : ""}`
    );
    return res.chats || [];
  }

  async getMessages(chatJid: string, limit = 100): Promise<MessageItem[]> {
    const res = await this.request<{ success: boolean; messages?: MessageItem[] }>(
      `/messages?chat_jid=${encodeURIComponent(chatJid)}&limit=${limit}`
    );
    return res.messages || [];
  }

  // Audit
  /** Messages across all numbers, newest first, filtered by number, person, department, text or dates. */
  async getMessageFeed(filters: FeedFilters = {}): Promise<FeedMessage[]> {
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(filters)) {
      if (value !== undefined && value !== "" && value !== false) search.set(key, String(value));
    }
    const qs = search.toString();
    const res = await this.request<{ success: boolean; messages?: FeedMessage[] }>(
      `/messages/feed${qs ? `?${qs}` : ""}`
    );
    return res.messages || [];
  }

  async getMessageConversations(filters: FeedFilters = {}): Promise<MessageConversation[]> {
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(filters)) {
      if (value !== undefined && value !== "" && value !== false) search.set(key, String(value));
    }
    const qs = search.toString();
    const res = await this.request<{ success: boolean; conversations?: MessageConversation[] }>(
      `/messages/chats${qs ? `?${qs}` : ""}`
    );
    return res.conversations || [];
  }

  async getMessageConversationMessages(
    instance: string,
    chatJid: string,
    limit = 100,
    before?: string
  ): Promise<{ messages: FeedMessage[]; hasMore: boolean }> {
    const search = new URLSearchParams({
      instance,
      chat_jid: chatJid,
      limit: String(limit),
    });
    if (before) search.set("before", before);
    const res = await this.request<{ success: boolean; messages?: FeedMessage[]; has_more?: boolean }>(
      `/messages/chats/messages?${search.toString()}`
    );
    return { messages: res.messages || [], hasMore: res.has_more ?? false };
  }

  async getMessageVersions(instance: string, chatJid: string, messageId: string): Promise<MessageVersion[]> {
    const search = new URLSearchParams({ instance, chat_jid: chatJid, message_id: messageId });
    const res = await this.request<{ success: boolean; versions?: MessageVersion[] }>(
      `/messages/versions?${search.toString()}`
    );
    return res.versions || [];
  }

  async getAccessLog(limit = 100): Promise<AccessLogEntry[]> {
    const res = await this.request<{ success: boolean; entries?: AccessLogEntry[] }>(`/access-log?limit=${limit}`);
    return res.entries || [];
  }

  // Privacy (LGPD)
  async anonymizeSubject(subject: string): Promise<{ messages: number; chats: number; media_removed: number; pseudonym: string }> {
    return this.request("/privacy/anonymize", { method: "POST", body: JSON.stringify({ subject }) });
  }

  async purgeOlderThan(days: number): Promise<{ removed: number }> {
    return this.request("/privacy/purge", { method: "POST", body: JSON.stringify({ days }) });
  }

  async getPrivacyLog(limit = 100): Promise<PrivacyLogEntry[]> {
    const res = await this.request<{ success: boolean; entries?: PrivacyLogEntry[] }>(`/privacy/log?limit=${limit}`);
    return res.entries || [];
  }
}

export class APIError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = "APIError";
  }
}

export const getErrorMessage = (error: unknown): { title: string; description: string; action?: string } => {
  if (error instanceof APIError) {
    switch (error.status) {
      case 401:
        return {
          title: "Unauthorized",
          description: "Your session is missing or has expired.",
          action: "Sign in again",
        };
      case 404:
        return {
          title: "Bridge Not Found",
          description: "Cannot connect to the WhatsApp bridge.",
          action: "Verify the bridge is running",
        };
      case 429:
        return {
          title: "Rate Limited",
          description: "Too many requests. Please wait.",
          action: "Will retry automatically...",
        };
      default:
        return {
          title: `Error ${error.status}`,
          description: error.message,
        };
    }
  }

  if (error instanceof TypeError && error.message.includes("fetch")) {
    return {
      title: "Network Error",
      description: "Cannot reach the WhatsApp bridge.",
      action: "Check your connection",
    };
  }

  return {
    title: "Unknown Error",
    description: error instanceof Error ? error.message : "An unexpected error occurred",
  };
};
