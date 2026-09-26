"use client";

import { useEffect, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { RefreshCw } from "lucide-react";
import { Sidebar } from "@/components/layout/sidebar";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { APIError, UNAUTHORIZED_EVENT, WhatsAppAPI } from "@/lib/api";
import { useAuth, useSettings } from "@/lib/store";

const isLoginPath = (pathname: string) => pathname.replace(/\/+$/, "") === "/login";

/**
 * Asks the bridge who the current session belongs to and shows the app shell
 * only when the answer is "someone". The browser holds no credential the page
 * can read: the session is an HttpOnly cookie, so the server is the source of truth.
 */
export function AuthGate({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { status, setAuthed, setAnon } = useAuth();
  const darkMode = useSettings((s) => s.darkMode);
  const [unreachable, setUnreachable] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const onLogin = isLoginPath(pathname);

  useEffect(() => {
    document.documentElement.classList.toggle("dark", darkMode);
  }, [darkMode]);

  // Ask the server who we are. State is set from the promise callbacks (never synchronously in the
  // effect), and `cancelled` drops a late answer if the gate unmounts or a retry supersedes it.
  useEffect(() => {
    let cancelled = false;
    new WhatsAppAPI().me().then(
      (user) => {
        if (cancelled) return;
        setUnreachable(false);
        setAuthed(user.username);
      },
      (error) => {
        if (cancelled) return;
        if (error instanceof APIError) {
          setUnreachable(false);
          setAnon(); // 401 (or 501 when login is not configured): not signed in
        } else {
          setUnreachable(true); // network failure: don't pretend we know
        }
      }
    );
    return () => {
      cancelled = true;
    };
  }, [attempt, setAuthed, setAnon]);

  // Any request answered with 401 means the session ended (expired or revoked elsewhere).
  useEffect(() => {
    window.addEventListener(UNAUTHORIZED_EVENT, setAnon);
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, setAnon);
  }, [setAnon]);

  useEffect(() => {
    if (status === "anon" && !onLogin) router.replace("/login/");
    if (status === "authed" && onLogin) router.replace("/");
  }, [status, onLogin, router]);

  if (unreachable) {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-4 p-8 text-center">
        <p className="font-medium">Cannot reach the WhatsApp bridge</p>
        <p className="text-sm text-muted-foreground">Check that the containers are running, then try again.</p>
        <Button variant="outline" onClick={() => setAttempt((n) => n + 1)}>
          <RefreshCw className="mr-2 h-4 w-4" />
          Retry
        </Button>
      </div>
    );
  }

  if (status === "checking") {
    return (
      <div className="flex min-h-screen items-center justify-center text-muted-foreground">Loading...</div>
    );
  }

  if (onLogin) return status === "anon" ? <>{children}</> : null;
  if (status !== "authed") return null; // redirecting to /login

  const getPageTitle = (path: string) => {
    const p = path.replace(/\/+$/, "");
    if (!p) return "Overview";
    if (p.startsWith("/pairing")) return "Device Pairing";
    if (p.startsWith("/webhooks")) return "Webhooks";
    if (p.startsWith("/mcp-clients")) return "MCP Clients";
    if (p.startsWith("/settings")) return "Settings";
    return "Dashboard";
  };

  return (
    <SidebarProvider>
      <Sidebar />
      <SidebarInset>
        <header className="flex h-14 shrink-0 items-center gap-2 border-b bg-background px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <span className="text-sm font-medium text-foreground">{getPageTitle(pathname)}</span>
        </header>
        <div className="flex flex-1 flex-col overflow-auto bg-muted/30">{children}</div>
      </SidebarInset>
    </SidebarProvider>
  );
}
