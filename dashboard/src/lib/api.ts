const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8081';
const WS_BASE = process.env.NEXT_PUBLIC_WS_URL || 'ws://localhost:8081';

// ── Auth ──

let authToken: string | null = null;

export function setToken(token: string) {
  authToken = token;
  if (typeof window !== 'undefined') {
    localStorage.setItem('antiproxy_token', token);
  }
}

export function getToken(): string | null {
  if (authToken) return authToken;
  if (typeof window !== 'undefined') {
    authToken = localStorage.getItem('antiproxy_token');
  }
  return authToken;
}

export function clearToken() {
  authToken = null;
  if (typeof window !== 'undefined') {
    localStorage.removeItem('antiproxy_token');
  }
}

async function fetchAPI(path: string, options: RequestInit = {}) {
  const token = getToken();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string> || {}),
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
  });

  if (res.status === 401) {
    clearToken();
    if (typeof window !== 'undefined') {
      window.location.href = '/';
    }
    throw new Error('Unauthorized');
  }

  return res.json();
}

// ── API Functions ──

export async function login(username: string, password: string) {
  const data = await fetchAPI('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  });
  if (data.token) setToken(data.token);
  return data;
}

export async function getUsers() {
  return fetchAPI('/api/users');
}

export async function getSessions() {
  return fetchAPI('/api/sessions');
}

export async function getBlockedRequests(limit = 100) {
  return fetchAPI(`/api/blocked-requests?limit=${limit}`);
}

export async function getStats() {
  return fetchAPI('/api/stats');
}

export async function getAlerts() {
  return fetchAPI('/api/alerts');
}

export async function getHealth() {
  return fetchAPI('/api/health');
}

export async function getFilterDomains() {
  return fetchAPI('/api/filter/domains');
}

export async function addFilterDomain(domain: string) {
  return fetchAPI('/api/filter/domains', {
    method: 'POST',
    body: JSON.stringify({ domain }),
  });
}

export async function removeFilterDomain(domain: string) {
  return fetchAPI(`/api/filter/domains/${domain}`, {
    method: 'DELETE',
  });
}

// ── WebSocket ──

export function connectWebSocket(onMessage: (data: any) => void): WebSocket | null {
  if (typeof window === 'undefined') return null;

  const ws = new WebSocket(`${WS_BASE}/api/ws`);
  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      onMessage(data);
    } catch {}
  };
  ws.onerror = () => {};
  ws.onclose = () => {
    // Auto-reconnect after 3 seconds
    setTimeout(() => connectWebSocket(onMessage), 3000);
  };
  return ws;
}
