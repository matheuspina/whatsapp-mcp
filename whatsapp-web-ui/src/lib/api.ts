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

// Webhook types
export interface WebhookTrigger {
  trigger_type: "all" | "chat_jid" | "sender" | "keyword" | "media_type";
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
  department_id?: number | null;
  department_name?: string | null;
  phone_number: string;
  created_at: string;
  updated_at: string;
}

export interface Instance {
  jid: string;
  phone_number: string;
  alias: string;
  employee_id?: number | null;
  employee_name?: string | null;
  status: "connected" | "disconnected" | "pairing" | "error";
  is_active: boolean;
  connected_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface InstancePairResponse {
  success: boolean;
  temp_id?: string;
  qr_code?: string;
  message?: string;
  error?: string;
}

export interface ChatItem {
  jid: string;
  name?: string;
  last_message_time: string;
  is_group: boolean;
  unread_count?: number;
  instance_jid?: string;
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

  async createEmployee(data: { name: string; role?: string; department_id?: number | null; phone_number?: string }): Promise<Employee> {
    const res = await this.request<{ success: boolean; employee: Employee }>("/employees", {
      method: "POST",
      body: JSON.stringify(data),
    });
    return res.employee;
  }

  async updateEmployee(id: number, data: { name?: string; role?: string; department_id?: number | null; phone_number?: string }): Promise<Employee> {
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
  async getInstances(): Promise<Instance[]> {
    const res = await this.request<{ success: boolean; instances?: Instance[] }>("/instances");
    return res.instances || [];
  }

  async createInstancePair(alias: string, employeeId?: number | null): Promise<InstancePairResponse> {
    return this.request<InstancePairResponse>("/instances/pair", {
      method: "POST",
      body: JSON.stringify({ alias, employee_id: employeeId }),
    });
  }

  async reconnectInstance(jid: string): Promise<void> {
    await this.request(`/instances/${encodeURIComponent(jid)}/reconnect`, { method: "POST" });
  }

  async disconnectInstance(jid: string): Promise<void> {
    await this.request(`/instances/${encodeURIComponent(jid)}/disconnect`, { method: "POST" });
  }

  async deleteInstance(jid: string): Promise<void> {
    await this.request(`/instances/${encodeURIComponent(jid)}`, { method: "DELETE" });
  }

  // Chats and Messages
  async getChats(): Promise<ChatItem[]> {
    const res = await this.request<{ success: boolean; chats?: ChatItem[] }>("/chats");
    return res.chats || [];
  }

  async getMessages(chatJid: string, limit = 100): Promise<MessageItem[]> {
    const res = await this.request<{ success: boolean; messages?: MessageItem[] }>(
      `/messages?chat_jid=${encodeURIComponent(chatJid)}&limit=${limit}`
    );
    return res.messages || [];
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
