"use client";

import { useState, useEffect } from "react";
import { getBlockedRequests } from "@/lib/api";

interface BlockedRequest {
  id: number;
  user_id: number | null;
  username: string;
  ip_address: string;
  domain: string;
  url_path: string;
  method: string;
  status: string;
  block_reason: string;
  timestamp: string;
}

export default function BlockedRequestsPage() {
  const [requests, setRequests] = useState<BlockedRequest[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    getBlockedRequests(200)
      .then((data) => setRequests(data))
      .catch(console.error)
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return <div className="loading"><div className="spinner" />Loading blocked requests...</div>;
  }

  // Aggregate by domain
  const domainCounts: Record<string, number> = {};
  requests.forEach((r) => {
    domainCounts[r.domain] = (domainCounts[r.domain] || 0) + 1;
  });
  const topDomains = Object.entries(domainCounts)
    .sort((a, b) => b[1] - a[1])
    .slice(0, 5);

  return (
    <>
      <div className="page-header">
        <h2>Blocked Requests</h2>
        <p>AI domain and API access attempts caught by the filter engine</p>
      </div>

      <div className="stats-grid">
        <div className="stat-card danger">
          <div className="label">Total Blocked</div>
          <div className="value">{requests.length}</div>
          <div className="sub">AI access attempts intercepted</div>
        </div>
        <div className="stat-card warning">
          <div className="label">Unique Domains</div>
          <div className="value">{Object.keys(domainCounts).length}</div>
          <div className="sub">Different AI domains attempted</div>
        </div>
        <div className="stat-card info">
          <div className="label">Top Blocked Domain</div>
          <div className="value" style={{ fontSize: 20 }}>{topDomains[0]?.[0] || "—"}</div>
          <div className="sub">{topDomains[0]?.[1] || 0} attempts</div>
        </div>
      </div>

      {/* Top Domains */}
      {topDomains.length > 0 && (
        <div className="data-table-container" style={{ marginBottom: 32 }}>
          <div className="data-table-header">
            <h3>🏆 Most Attempted AI Domains</h3>
          </div>
          {topDomains.map(([domain, count]) => (
            <div key={domain} className="health-item">
              <span className="health-label" style={{ color: "var(--accent-danger)" }}>{domain}</span>
              <span className="health-value">{count} attempts</span>
            </div>
          ))}
        </div>
      )}

      {/* Request Log Table */}
      <div className="data-table-container">
        <div className="data-table-header">
          <h3>🚫 Blocked Request Log</h3>
          <span className="badge danger">{requests.length} entries</span>
        </div>
        {requests.length === 0 ? (
          <div className="empty-state">
            <div className="icon">🎉</div>
            <p>No blocked requests — all contestants are behaving!</p>
          </div>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Time</th>
                <th>User</th>
                <th>IP Address</th>
                <th>Domain</th>
                <th>Method</th>
                <th>Reason</th>
              </tr>
            </thead>
            <tbody>
              {requests.map((req) => (
                <tr key={req.id}>
                  <td style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12, color: "var(--text-muted)", whiteSpace: "nowrap" }}>
                    {new Date(req.timestamp).toLocaleString()}
                  </td>
                  <td style={{ fontWeight: 600, color: "var(--text-primary)" }}>
                    {req.username || (req.user_id ? `User #${req.user_id}` : "—")}
                  </td>
                  <td style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 13 }}>
                    {req.ip_address}
                  </td>
                  <td style={{ color: "var(--accent-danger)", fontWeight: 500 }}>
                    {req.domain}
                  </td>
                  <td>
                    <span className="badge info">{req.method || "CONNECT"}</span>
                  </td>
                  <td style={{ fontSize: 12, color: "var(--text-muted)", maxWidth: 300, overflow: "hidden", textOverflow: "ellipsis" }}>
                    {req.block_reason}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
