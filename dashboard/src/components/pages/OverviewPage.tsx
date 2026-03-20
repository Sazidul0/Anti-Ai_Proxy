"use client";

import { useState, useEffect, useCallback } from "react";
import { getStats, getAlerts, connectWebSocket } from "@/lib/api";

interface Stats {
  proxy: { active_connections: number; total_requests: number; blocked_requests: number; allowed_requests: number };
  total_users: number;
  suspicious_users: number;
  active_sessions: number;
  total_sessions: number;
  total_requests: number;
  blocked_requests: number;
  filter?: { blocked_domains: number };
}

interface Alert {
  user_id: number;
  username: string;
  suspicion_score: number;
  is_suspicious: boolean;
  recent_events: any[];
  last_triggered: string;
}

export default function OverviewPage() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [loading, setLoading] = useState(true);
  const [liveConnections, setLiveConnections] = useState(0);

  const fetchData = useCallback(async () => {
    try {
      const [statsData, alertsData] = await Promise.all([getStats(), getAlerts()]);
      setStats(statsData);
      setAlerts(alertsData);
      setLiveConnections(statsData.proxy?.active_connections || 0);
    } catch (err) {
      console.error("Failed to fetch data:", err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 10000);

    const ws = connectWebSocket((data) => {
      if (data.stats) {
        setLiveConnections(data.stats.active_connections || 0);
      }
    });

    return () => {
      clearInterval(interval);
      if (ws) ws.close();
    };
  }, [fetchData]);

  if (loading) {
    return <div className="loading"><div className="spinner" />Loading dashboard...</div>;
  }

  const blockRate = stats && stats.total_requests > 0
    ? ((stats.blocked_requests / stats.total_requests) * 100).toFixed(1)
    : "0.0";

  return (
    <>
      <div className="page-header">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <div>
            <h2>Dashboard Overview</h2>
            <p>Real-time monitoring of the CTF Anti-AI Proxy Gateway</p>
          </div>
          <div className="live-indicator">
            <span className="blink" />
            LIVE
          </div>
        </div>
      </div>

      <div className="stats-grid">
        <div className="stat-card info">
          <div className="label">Active Connections</div>
          <div className="value">{liveConnections}</div>
          <div className="sub">Real-time proxy connections</div>
        </div>
        <div className="stat-card success">
          <div className="label">Active Sessions</div>
          <div className="value">{stats?.active_sessions || 0}</div>
          <div className="sub">of {stats?.total_sessions || 0} total sessions</div>
        </div>
        <div className="stat-card warning">
          <div className="label">Blocked Requests</div>
          <div className="value">{stats?.blocked_requests || 0}</div>
          <div className="sub">{blockRate}% block rate</div>
        </div>
        <div className="stat-card danger">
          <div className="label">Suspicious Users</div>
          <div className="value">{stats?.suspicious_users || 0}</div>
          <div className="sub">of {stats?.total_users || 0} total users</div>
        </div>
      </div>

      {/* Alerts Table */}
      <div className="data-table-container">
        <div className="data-table-header">
          <h3>⚠️ Suspicious Activity Alerts</h3>
          <span className="badge warning">{alerts.length} alert{alerts.length !== 1 ? "s" : ""}</span>
        </div>
        {alerts.length === 0 ? (
          <div className="empty-state">
            <div className="icon">✅</div>
            <p>No suspicious activity detected</p>
          </div>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>User</th>
                <th>Suspicion Score</th>
                <th>Status</th>
                <th>Events</th>
                <th>Last Triggered</th>
              </tr>
            </thead>
            <tbody>
              {alerts.map((alert) => {
                const scoreColor = alert.suspicion_score >= 50 ? "var(--accent-danger)"
                  : alert.suspicion_score >= 25 ? "var(--accent-warning)" : "var(--accent-success)";
                const scorePct = Math.min(alert.suspicion_score, 100);
                return (
                  <tr key={alert.user_id}>
                    <td style={{ fontWeight: 600, color: "var(--text-primary)" }}>{alert.username}</td>
                    <td>
                      <span style={{ color: scoreColor, fontWeight: 700 }}>{alert.suspicion_score}</span>
                      <div className="score-bar">
                        <div className="fill" style={{ width: `${scorePct}%`, background: scoreColor }} />
                      </div>
                    </td>
                    <td>
                      {alert.is_suspicious
                        ? <span className="badge danger"><span className="dot" />Suspicious</span>
                        : <span className="badge warning">Monitoring</span>
                      }
                    </td>
                    <td>{alert.recent_events?.length || 0}</td>
                    <td style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12, color: "var(--text-muted)" }}>
                      {alert.last_triggered ? new Date(alert.last_triggered).toLocaleString() : "—"}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {/* Quick Stats */}
      <div className="stats-grid">
        <div className="stat-card">
          <div className="label">Total Requests</div>
          <div className="value">{(stats?.total_requests || 0).toLocaleString()}</div>
          <div className="sub">All time proxied requests</div>
        </div>
        <div className="stat-card">
          <div className="label">Total Users</div>
          <div className="value">{stats?.total_users || 0}</div>
          <div className="sub">Registered contestants</div>
        </div>
        <div className="stat-card">
          <div className="label">Filter Rules</div>
          <div className="value">{stats?.filter?.blocked_domains || 0}</div>
          <div className="sub">Blocked domains loaded</div>
        </div>
      </div>
    </>
  );
}
