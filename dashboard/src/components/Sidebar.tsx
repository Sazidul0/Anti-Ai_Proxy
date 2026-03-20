"use client";

import { clearToken } from "@/lib/api";

interface SidebarProps {
  currentPage: string;
  onNavigate: (page: any) => void;
}

const navItems = [
  { id: "overview", label: "Overview", icon: "📊" },
  { id: "users", label: "Users", icon: "👥" },
  { id: "sessions", label: "Sessions", icon: "🔗" },
  { id: "blocked", label: "Blocked Requests", icon: "🚫" },
  { id: "health", label: "System Health", icon: "💚" },
];

export default function Sidebar({ currentPage, onNavigate }: SidebarProps) {
  const handleLogout = () => {
    clearToken();
    window.location.reload();
  };

  return (
    <aside className="sidebar">
      <div className="sidebar-brand">
        <h1>🛡️ Anti-AI Proxy</h1>
        <p>CTF Gateway Dashboard</p>
      </div>

      <nav className="sidebar-nav">
        {navItems.map((item) => (
          <a
            key={item.id}
            className={`nav-item ${currentPage === item.id ? "active" : ""}`}
            onClick={() => onNavigate(item.id)}
            href="#"
          >
            <span className="icon">{item.icon}</span>
            {item.label}
          </a>
        ))}
      </nav>

      <div style={{ padding: "0 12px", marginTop: "auto" }}>
        <a className="nav-item" onClick={handleLogout} href="#"
           style={{ color: "var(--accent-danger)" }}>
          <span className="icon">🚪</span>
          Sign Out
        </a>
      </div>
    </aside>
  );
}
