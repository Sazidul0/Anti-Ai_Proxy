"use client";

import { useState, useEffect } from "react";
import { getUsers } from "@/lib/api";

interface User {
  id: number;
  username: string;
  role: string;
  suspicion_score: number;
  is_suspicious: boolean;
  created_at: string;
  active_sessions: number;
  total_requests: number;
  blocked_requests: number;
}

export default function UsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");

  useEffect(() => {
    getUsers()
      .then((data) => setUsers(data))
      .catch(console.error)
      .finally(() => setLoading(false));
  }, []);

  const filtered = users.filter((u) =>
    u.username.toLowerCase().includes(search.toLowerCase())
  );

  if (loading) {
    return <div className="loading"><div className="spinner" />Loading users...</div>;
  }

  return (
    <>
      <div className="page-header">
        <h2>Users</h2>
        <p>Monitor registered users, their activity, and suspicion scores</p>
      </div>

      <div className="stats-grid">
        <div className="stat-card info">
          <div className="label">Total Users</div>
          <div className="value">{users.length}</div>
        </div>
        <div className="stat-card success">
          <div className="label">Active Now</div>
          <div className="value">{users.filter(u => u.active_sessions > 0).length}</div>
        </div>
        <div className="stat-card danger">
          <div className="label">Suspicious</div>
          <div className="value">{users.filter(u => u.is_suspicious).length}</div>
        </div>
      </div>

      <div className="data-table-container">
        <div className="data-table-header">
          <h3>All Users</h3>
          <div className="form-group" style={{ margin: 0 }}>
            <input
              type="text"
              placeholder="Search users..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              style={{ width: 220, padding: "8px 12px", fontSize: 13 }}
            />
          </div>
        </div>
        <table className="data-table">
          <thead>
            <tr>
              <th>Username</th>
              <th>Role</th>
              <th>Status</th>
              <th>Suspicion</th>
              <th>Sessions</th>
              <th>Total Reqs</th>
              <th>Blocked</th>
              <th>Joined</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((user) => {
              const scoreColor = user.suspicion_score >= 50 ? "var(--accent-danger)"
                : user.suspicion_score >= 25 ? "var(--accent-warning)" : "var(--accent-success)";
              return (
                <tr key={user.id}>
                  <td style={{ fontWeight: 600, color: "var(--text-primary)" }}>
                    {user.username}
                  </td>
                  <td>
                    <span className={`badge ${user.role === "admin" ? "info" : "success"}`}>
                      {user.role}
                    </span>
                  </td>
                  <td>
                    {user.active_sessions > 0
                      ? <span className="badge success"><span className="dot" />Online</span>
                      : <span className="badge" style={{ background: "rgba(100,116,139,0.15)", color: "var(--text-muted)" }}>Offline</span>
                    }
                  </td>
                  <td>
                    <span style={{ color: scoreColor, fontWeight: 700 }}>{user.suspicion_score}</span>
                    <div className="score-bar">
                      <div className="fill" style={{ width: `${Math.min(user.suspicion_score, 100)}%`, background: scoreColor }} />
                    </div>
                    {user.is_suspicious && <span className="badge danger" style={{ marginLeft: 8, fontSize: 10 }}>⚠ FLAGGED</span>}
                  </td>
                  <td>{user.active_sessions}</td>
                  <td>{user.total_requests.toLocaleString()}</td>
                  <td style={{ color: user.blocked_requests > 0 ? "var(--accent-danger)" : "var(--text-muted)" }}>
                    {user.blocked_requests}
                  </td>
                  <td style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12, color: "var(--text-muted)" }}>
                    {new Date(user.created_at).toLocaleDateString()}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        {filtered.length === 0 && (
          <div className="empty-state">
            <div className="icon">🔍</div>
            <p>No users found</p>
          </div>
        )}
      </div>
    </>
  );
}
