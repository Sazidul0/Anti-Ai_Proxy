"use client";

import { useState, useEffect } from "react";
import { getSessions } from "@/lib/api";

interface Session {
  id: number;
  user_id: number;
  username: string;
  session_token: string;
  ip_address: string;
  device_fingerprint: string;
  is_active: boolean;
  connection_start: string;
  last_activity: string;
}

export default function SessionsPage() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetch = () => {
      getSessions()
        .then((data) => setSessions(data))
        .catch(console.error)
        .finally(() => setLoading(false));
    };
    fetch();
    const interval = setInterval(fetch, 5000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return <div className="loading"><div className="spinner" />Loading sessions...</div>;
  }

  const formatDuration = (start: string) => {
    const diff = Date.now() - new Date(start).getTime();
    const mins = Math.floor(diff / 60000);
    const hrs = Math.floor(mins / 60);
    if (hrs > 0) return `${hrs}h ${mins % 60}m`;
    return `${mins}m`;
  };

  return (
    <>
      <div className="page-header">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <div>
            <h2>Active Sessions</h2>
            <p>Currently connected proxy sessions</p>
          </div>
          <div className="live-indicator">
            <span className="blink" />
            {sessions.length} active
          </div>
        </div>
      </div>

      <div className="data-table-container">
        <div className="data-table-header">
          <h3>🔗 Live Sessions</h3>
        </div>
        {sessions.length === 0 ? (
          <div className="empty-state">
            <div className="icon">🔌</div>
            <p>No active sessions</p>
          </div>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>User</th>
                <th>IP Address</th>
                <th>Session Token</th>
                <th>Duration</th>
                <th>Last Activity</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {sessions.map((sess) => (
                <tr key={sess.id}>
                  <td style={{ fontWeight: 600, color: "var(--text-primary)" }}>
                    {sess.username || `User #${sess.user_id}`}
                  </td>
                  <td style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 13 }}>
                    {sess.ip_address}
                  </td>
                  <td style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12, color: "var(--text-muted)" }}>
                    {sess.session_token.substring(0, 12)}...
                  </td>
                  <td>{formatDuration(sess.connection_start)}</td>
                  <td style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12, color: "var(--text-muted)" }}>
                    {new Date(sess.last_activity).toLocaleTimeString()}
                  </td>
                  <td>
                    <span className="badge success"><span className="dot" />Active</span>
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
