"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState, useCallback } from "react";
import { cn } from "@/lib/utils";
import { Link2, LogOut, MessageSquare, Settings, Webhook } from "lucide-react";
import { WhatsAppAPI } from "@/lib/api";
import { useAuth } from "@/lib/store";

const navItems = [
  {
    title: "Pairing",
    href: "/pairing",
    icon: Link2,
    description: "Link WhatsApp device",
  },
  {
    title: "Webhooks",
    href: "/webhooks",
    icon: Webhook,
    description: "Manage webhook endpoints",
  },
];

type ConnectionDotStatus = "connected" | "disconnected" | "unknown";

export function Sidebar() {
  const pathname = usePathname();
  const { username, setAnon } = useAuth();
  const [connStatus, setConnStatus] = useState<ConnectionDotStatus>("unknown");

  const pollConnection = useCallback(async () => {
    try {
      const api = new WhatsAppAPI();
      const status = await api.getConnectionStatus();
      setConnStatus(status.connected ? "connected" : "disconnected");
    } catch {
      setConnStatus("unknown");
    }
  }, []);

  useEffect(() => {
    pollConnection();
    const interval = setInterval(pollConnection, 10000);
    return () => clearInterval(interval);
  }, [pollConnection]);

  const handleLogout = async () => {
    try {
      await new WhatsAppAPI().logout();
    } finally {
      setAnon(); // the gate redirects to /login
    }
  };

  const dotColor = {
    connected: "bg-green-500",
    disconnected: "bg-red-500",
    unknown: "bg-yellow-500",
  }[connStatus];

  return (
    <aside className="w-64 bg-card border-r min-h-screen p-4 flex flex-col">
      <div className="flex items-center gap-2 px-2 mb-8">
        <div className="relative">
          <MessageSquare className="h-8 w-8 text-green-500" />
          <span
            className={cn("absolute -top-0.5 -right-0.5 h-3 w-3 rounded-full border-2 border-card", dotColor)}
            title={connStatus === "connected" ? "Connected" : connStatus === "disconnected" ? "Disconnected" : "Checking..."}
          />
        </div>
        <div>
          <h1 className="font-bold text-lg">WhatsApp MCP</h1>
          <p className="text-xs text-muted-foreground">by Matheus Pina</p>
        </div>
      </div>

      <nav className="flex-1 space-y-2">
        {navItems.map((item) => {
          const isActive = pathname === item.href || pathname.startsWith(item.href + "/");
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "flex items-center gap-3 px-3 py-2 rounded-lg transition-colors",
                isActive
                  ? "bg-primary text-primary-foreground"
                  : "hover:bg-muted text-muted-foreground hover:text-foreground"
              )}
            >
              <item.icon className="h-5 w-5" />
              <div>
                <div className="font-medium text-sm">{item.title}</div>
                <div className={cn("text-xs", isActive ? "text-primary-foreground/80" : "text-muted-foreground")}>
                  {item.description}
                </div>
              </div>
            </Link>
          );
        })}
      </nav>

      <div className="border-t pt-4 mt-4">
        <Link
          href="/settings"
          className={cn(
            "flex items-center gap-3 px-3 py-2 rounded-lg transition-colors",
            pathname === "/settings"
              ? "bg-primary text-primary-foreground"
              : "hover:bg-muted text-muted-foreground hover:text-foreground"
          )}
        >
          <Settings className="h-5 w-5" />
          <span className="font-medium text-sm">Settings</span>
        </Link>
        <div className="mt-2 flex items-center justify-between gap-2 px-3 py-2 text-sm">
          <span className="truncate text-muted-foreground" title={username}>
            {username}
          </span>
          <button
            type="button"
            onClick={handleLogout}
            className="flex shrink-0 items-center gap-1 text-muted-foreground transition-colors hover:text-foreground"
          >
            <LogOut className="h-4 w-4" />
            Sign out
          </button>
        </div>
      </div>
    </aside>
  );
}
