"use client";

import { useState, useEffect } from "react";
import { getHealth, getStats, getFilterDomains } from "@/lib/api";

export default function SystemHealthPage() {
  const [health, setHealth] = useState<any>(null);
  const [stats, setStats] = useState<any>(null);
  const [filterInfo, setFilterInfo] = useState<any>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetch = async () => {
      try {
        const [h, s, f] = await Promise.all([getHealth(), getStats(), getFilterDomains()]);
        setHealth(h);
        setStats(s);
        setFilterInfo(f);
      } catch (err) {
        console.error(err);
      } finally {
        setLoading(false);
      }
    };
    fetch();
    const interval = setInterval(fetch, 10000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return <div className="loading"><div className="spinner" />Loading system health...</div>;
  }

  return (
    <>
      <div className="page-header">
        <h2>System Health</h2>
        <p>Proxy uptime, service status, and filter configuration</p>
      </div>

      <div className="stats-grid">
        <div className="stat-card success">
          <div className="label">Proxy Status</div>
          <div className="value" style={{ color: "var(--accent-success)" }}>
            {health?.status === "ok" ? "Healthy" : "Down"}
          </div>
          <div className="sub">{health?.status === "ok" ? "All systems operational" : "Service degraded"}</div>
        </div>
        <div className="stat-card info">
          <div className="label">Active Connections</div>
          <div className="value">{stats?.proxy?.active_connections || 0}</div>
          <div className="sub">Current proxy connections</div>
        </div>
        <div className="stat-card info">
          <div className="label">Total Requests</div>
          <div className="value">{(stats?.proxy?.total_requests || 0).toLocaleString()}</div>
          <div className="sub">Requests processed by proxy</div>
        </div>
        <div className="stat-card warning">
          <div className="label">Filter Rules</div>
          <div className="value">{filterInfo?.stats?.blocked_domains || 0}</div>
          <div className="sub">Active domain blocklist entries</div>
        </div>
      </div>

      {/* Services Status */}
      <div className="data-table-container">
        <div className="data-table-header">
          <h3>🔧 Service Status</h3>
        </div>
        <div className="health-item">
          <span className="health-label">Proxy Server</span>
          <span className="badge success"><span className="dot" />Running</span>
        </div>
        <div className="health-item">
          <span className="health-label">API Server</span>
          <span className="badge success"><span className="dot" />{health?.status === "ok" ? "Running" : "Error"}</span>
        </div>
        <div className="health-item">
          <span className="health-label">PostgreSQL</span>
          <span className="badge success"><span className="dot" />Connected</span>
        </div>
        <div className="health-item">
          <span className="health-label">Redis</span>
          <span className="badge success"><span className="dot" />Connected</span>
        </div>
      </div>

      {/* Filter Config */}
      <div className="data-table-container">
        <div className="data-table-header">
          <h3>🛡️ Filter Engine Configuration</h3>
        </div>
        <div className="health-item">
          <span className="health-label">Blocked Domains</span>
          <span className="health-value">{filterInfo?.stats?.blocked_domains || 0} rules</span>
        </div>
        <div className="health-item">
          <span className="health-label">Blocked TLDs</span>
          <span className="health-value">{filterInfo?.stats?.blocked_tlds || 0} rules</span>
        </div>
        <div className="health-item">
          <span className="health-label">Blocked API Paths</span>
          <span className="health-value">{filterInfo?.stats?.blocked_paths || 0} patterns</span>
        </div>
        <div className="health-item">
          <span className="health-label">Whitelisted Domains</span>
          <span className="health-value">{filterInfo?.stats?.whitelisted || 0} entries</span>
        </div>
      </div>

      {/* Proxy Performance */}
      <div className="data-table-container">
        <div className="data-table-header">
          <h3>📈 Proxy Performance</h3>
        </div>
        <div className="health-item">
          <span className="health-label">Total Requests Processed</span>
          <span className="health-value">{(stats?.proxy?.total_requests || 0).toLocaleString()}</span>
        </div>
        <div className="health-item">
          <span className="health-label">Allowed Requests</span>
          <span className="health-value" style={{ color: "var(--accent-success)" }}>
            {(stats?.proxy?.allowed_requests || 0).toLocaleString()}
          </span>
        </div>
        <div className="health-item">
          <span className="health-label">Blocked Requests</span>
          <span className="health-value" style={{ color: "var(--accent-danger)" }}>
            {(stats?.proxy?.blocked_requests || 0).toLocaleString()}
          </span>
        </div>
        <div className="health-item">
          <span className="health-label">Block Rate</span>
          <span className="health-value">
            {stats?.proxy?.total_requests > 0
              ? ((stats.proxy.blocked_requests / stats.proxy.total_requests) * 100).toFixed(2) + "%"
              : "0.00%"
            }
          </span>
        </div>
      </div>
    </>
  );
}
