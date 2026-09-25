"use client";

import { useEffect, useState } from "react";
import { Loader2, LogOut, Monitor, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ActiveSession, WhatsAppAPI, getErrorMessage } from "@/lib/api";

/** Turns a raw User-Agent into something a person can recognise ("Chrome on macOS"). */
export function describeUserAgent(ua: string): string {
  if (!ua) return "Unknown device";
  const browser = /Edg\//.test(ua)
    ? "Edge"
    : /OPR\//.test(ua)
      ? "Opera"
      : /Firefox\//.test(ua)
        ? "Firefox"
        : /Chrome\//.test(ua)
          ? "Chrome"
          : /Safari\//.test(ua)
            ? "Safari"
            : "";
  // Order matters: iOS UAs also say "Mac OS X", Android UAs also say "Linux".
  const os = /iPhone|iPad/.test(ua)
    ? "iOS"
    : /Android/.test(ua)
      ? "Android"
      : /Windows/.test(ua)
        ? "Windows"
        : /Mac OS X|Macintosh/.test(ua)
          ? "macOS"
          : /Linux/.test(ua)
            ? "Linux"
            : "";
  if (browser && os) return `${browser} on ${os}`;
  return browser || os || ua.slice(0, 40);
}

const formatDate = (iso: string) => new Date(iso).toLocaleString();

/**
 * Lists every signed-in session as the bridge sees it, so it looks the same from any
 * browser and doesn't depend on anything stored locally. Any session can end any other.
 */
export function ActiveSessions() {
  const [sessions, setSessions] = useState<ActiveSession[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [revoking, setRevoking] = useState<string | null>(null);

  const [reloadTick, setReloadTick] = useState(0);

  // Same pattern as the auth gate: state is set from promise callbacks, `cancelled` drops stale answers.
  useEffect(() => {
    let cancelled = false;
    new WhatsAppAPI().listSessions().then(
      (list) => {
        if (cancelled) return;
        setSessions(list);
        setLoading(false);
      },
      (error) => {
        if (cancelled) return;
        toast.error(getErrorMessage(error).title, { description: getErrorMessage(error).description });
        setLoading(false);
      }
    );
    return () => {
      cancelled = true;
    };
  }, [reloadTick]);

  const load = () => {
    setLoading(true);
    setReloadTick((n) => n + 1);
  };

  const revoke = async (session: ActiveSession) => {
    setRevoking(session.id);
    try {
      await new WhatsAppAPI().revokeSession(session.id);
      toast.success("Session ended");
      load();
    } catch (error) {
      toast.error(getErrorMessage(error).title, { description: getErrorMessage(error).description });
    } finally {
      setRevoking(null);
    }
  };

  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between space-y-0">
        <div className="space-y-1.5">
          <CardTitle>Active sessions</CardTitle>
          <CardDescription>
            Everyone currently signed in to this panel. Sessions are kept on the server, so this list is the
            same from any browser.
          </CardDescription>
        </div>
        <Button variant="outline" size="sm" onClick={load} disabled={loading}>
          <RefreshCw className={"mr-2 h-4 w-4 " + (loading ? "animate-spin" : "")} />
          Refresh
        </Button>
      </CardHeader>
      <CardContent className="space-y-3">
        {sessions === null && <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />}
        {sessions?.length === 0 && <p className="text-sm text-muted-foreground">No active sessions.</p>}
        {sessions?.map((s) => (
          <div key={s.id} className="flex items-center justify-between gap-4 rounded-lg border p-3">
            <div className="flex min-w-0 items-start gap-3">
              <Monitor className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" />
              <div className="min-w-0 text-sm">
                <div className="flex flex-wrap items-center gap-2 font-medium">
                  {describeUserAgent(s.user_agent)}
                  {s.current && <Badge>This session</Badge>}
                </div>
                <div className="text-muted-foreground">
                  {s.username} · {s.ip}
                </div>
                <div className="text-xs text-muted-foreground">
                  Signed in {formatDate(s.created_at)} · Last active {formatDate(s.last_seen_at)}
                </div>
              </div>
            </div>
            {!s.current && (
              <Button variant="outline" size="sm" onClick={() => revoke(s)} disabled={revoking === s.id}>
                {revoking === s.id ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <LogOut className="mr-2 h-4 w-4" />
                )}
                End
              </Button>
            )}
          </div>
        ))}
      </CardContent>
    </Card>
  );
}
