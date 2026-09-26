"use client";

import { useEffect, useState, useCallback } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Notice, noticeToneVariants } from "@/components/common/notice";
import { StatCard, StatGrid } from "@/components/common/stat-card";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { Separator } from "@/components/ui/separator";
import { CheckCircle, RefreshCw, Settings, Plus, MessageSquare, Clock, AlertTriangle, Loader2, XCircle, WifiOff, Zap } from "lucide-react";
import { WhatsAppAPI, SyncStatusResponse, ConnectionStatusResponse } from "@/lib/api";
import { usePairing } from "@/lib/store";

interface DashboardProps {
  onOpenSettings: () => void;
}

export function Dashboard({ onOpenSettings }: DashboardProps) {
  const { reset } = usePairing();
  const [syncStatus, setSyncStatus] = useState<SyncStatusResponse | null>(null);
  const [connStatus, setConnStatus] = useState<ConnectionStatusResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [reconnecting, setReconnecting] = useState(false);

  const fetchStatus = useCallback(async () => {
    try {
      const api = new WhatsAppAPI();
      const [sync, conn] = await Promise.all([
        api.getSyncStatus(),
        api.getConnectionStatus(),
      ]);
      setSyncStatus(sync);
      setConnStatus(conn);
    } catch (error) {
      console.error("Failed to fetch status:", error);
    } finally {
      setLoading(false);
    }
  }, []);

  // Polling loop: state is only set from the promise callbacks, never synchronously in the effect body.
  useEffect(() => {
    let ignore = false;
    const poll = () => {
      const api = new WhatsAppAPI();
      Promise.all([api.getSyncStatus(), api.getConnectionStatus()])
        .then(([sync, conn]) => {
          if (ignore) return;
          setSyncStatus(sync);
          setConnStatus(conn);
        })
        .catch((error: unknown) => {
          console.error("Failed to fetch status:", error);
        })
        .finally(() => {
          if (!ignore) setLoading(false);
        });
    };
    poll();
    const interval = setInterval(poll, 5000);
    return () => {
      ignore = true;
      clearInterval(interval);
    };
  }, []);

  const handleRefresh = () => {
    setLoading(true);
    fetchStatus();
  };

  const handleReconnect = async () => {
    setReconnecting(true);
    try {
      const api = new WhatsAppAPI();
      await api.reconnect();
    } catch (error) {
      console.error("Failed to reconnect:", error);
    } finally {
      setTimeout(() => {
        setReconnecting(false);
        fetchStatus();
      }, 3000);
    }
  };

  const isConnected = connStatus?.connected ?? true;
  const isDisconnected = connStatus !== null && !connStatus.connected;
  const hasReconnectErrors = (connStatus?.auto_reconnect_errors ?? 0) > 0;

  return (
    <Card className="w-full max-w-lg">
      <CardHeader>
        <CardTitle className="flex items-center justify-center gap-2">
          {isConnected ? (
            <>
              <CheckCircle className="h-6 w-6 text-success" />
              Conectado
            </>
          ) : hasReconnectErrors ? (
            <>
              <Loader2 className="h-6 w-6 text-warning animate-spin" />
              Reconectando...
            </>
          ) : (
            <>
              <XCircle className="h-6 w-6 text-destructive" />
              Desconectado
            </>
          )}
        </CardTitle>
        <CardDescription>Seu WhatsApp está conectado a este painel.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Connection details when disconnected */}
        {isDisconnected && (
          <Notice variant={hasReconnectErrors ? "warning" : "destructive"} icon={WifiOff} title="Conexão perdida">
            <div className="text-sm text-muted-foreground space-y-1">
              {connStatus?.disconnected_for && (
                <p>Desconectado há <span className="font-medium text-foreground">{connStatus.disconnected_for}</span></p>
              )}
              {connStatus?.last_connected && (
                <p>Última conexão: <span className="text-foreground">{new Date(connStatus.last_connected).toLocaleString()}</span></p>
              )}
              {hasReconnectErrors && (
                <p>Tentativas de reconexão: <span className="text-foreground">{connStatus?.auto_reconnect_errors}</span></p>
              )}
            </div>
          </Notice>
        )}

        {/* Uptime when connected */}
        {isConnected && connStatus?.uptime && (
          <div className="text-center text-sm text-muted-foreground">
            Conectado há <span>{connStatus.uptime}</span>
            {connStatus.last_connected && (
              <> &middot; desde <span>{new Date(connStatus.last_connected).toLocaleTimeString()}</span></>
            )}
          </div>
        )}

        <StatGrid columns={2}>
          <StatCard
            icon={syncStatus?.syncing ? <Loader2 className="animate-spin text-warning" /> : <CheckCircle className="text-success" />}
            label="Sincronização"
            value={syncStatus?.syncing ? "Em andamento" : "Concluída"}
          >
            {syncStatus && <Progress value={syncStatus.sync_progress} className="mt-2 h-2" />}
          </StatCard>

          <StatCard
            icon={<Clock className="text-muted-foreground" />}
            label="Última sincronização"
            large={false}
            value={syncStatus?.last_sync ? new Date(syncStatus.last_sync).toLocaleString() : "Em andamento..."}
          />

          <StatCard
            icon={<MessageSquare className="text-muted-foreground" />}
            label="Mensagens"
            value={syncStatus?.message_count?.toLocaleString() || "0"}
          />

          <StatCard
            icon={<MessageSquare className="text-muted-foreground" />}
            label="Conversas"
            value={syncStatus?.conversation_count?.toLocaleString() || "0"}
          />
        </StatGrid>

        {syncStatus?.recommendations && syncStatus.recommendations.length > 0 && (
          <Notice variant="warning" icon={AlertTriangle} title="Recomendações">
            <ul className="text-sm text-muted-foreground space-y-1">
              {syncStatus.recommendations.map((rec, i) => (
                <li key={i} className="flex items-start gap-2">
                  <span className={noticeToneVariants({ variant: "warning" })}>•</span>
                  {rec}
                </li>
              ))}
            </ul>
          </Notice>
        )}

        {syncStatus?.error && (
          <Notice variant="destructive" icon={AlertTriangle} title="Erro">
            <p className="text-sm text-muted-foreground">{syncStatus.error}</p>
          </Notice>
        )}

        <Separator />

        <div className="flex gap-2 justify-center flex-wrap">
          {isDisconnected && (
            <Button variant="destructive" size="sm" onClick={handleReconnect} disabled={reconnecting}>
              <Zap className={"h-4 w-4 mr-2 " + (reconnecting ? "animate-pulse" : "")} />
              {reconnecting ? "Reconectando..." : "Reconectar agora"}
            </Button>
          )}
          <Button variant="outline" size="sm" onClick={handleRefresh} disabled={loading}>
            <RefreshCw className={"h-4 w-4 mr-2 " + (loading ? "animate-spin" : "")} />
            Atualizar
          </Button>
          <Button variant="outline" size="sm" onClick={onOpenSettings}>
            <Settings className="h-4 w-4 mr-2" />
            Configurações
          </Button>
          <Button variant="outline" size="sm" onClick={reset}>
            <Plus className="h-4 w-4 mr-2" />
            Novo aparelho
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
