"use client";

import { useState } from "react";
import Sidebar from "./Sidebar";
import OverviewPage from "./pages/OverviewPage";
import UsersPage from "./pages/UsersPage";
import SessionsPage from "./pages/SessionsPage";
import BlockedRequestsPage from "./pages/BlockedRequestsPage";
import SystemHealthPage from "./pages/SystemHealthPage";

type Page = "overview" | "users" | "sessions" | "blocked" | "health";

export default function Dashboard() {
  const [currentPage, setCurrentPage] = useState<Page>("overview");

  const renderPage = () => {
    switch (currentPage) {
      case "overview":
        return <OverviewPage />;
      case "users":
        return <UsersPage />;
      case "sessions":
        return <SessionsPage />;
      case "blocked":
        return <BlockedRequestsPage />;
      case "health":
        return <SystemHealthPage />;
      default:
        return <OverviewPage />;
    }
  };

  return (
    <div className="app-layout">
      <Sidebar currentPage={currentPage} onNavigate={setCurrentPage} />
      <main className="main-content">{renderPage()}</main>
    </div>
  );
}
