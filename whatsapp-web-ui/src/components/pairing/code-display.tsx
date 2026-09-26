"use client";

import { useEffect, useState, useCallback } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Clock, Loader2, CheckCircle, AlertCircle, Smartphone, Copy } from "lucide-react";
import { WhatsAppAPI, getErrorMessage } from "@/lib/api";
import { usePairing } from "@/lib/store";
import { toast } from "sonner";

type PairingStatus = "waiting" | "success" | "error" | "expired";

interface StatusInfo {
  icon: typeof Loader2;
  text: string;
  color: string;
  animate: boolean;
}

const statusConfigs: Record<PairingStatus, StatusInfo> = {
  waiting: { icon: Loader2, text: "Aguardando confirmação no celular...", color: "text-muted-foreground", animate: true },
  success: { icon: CheckCircle, text: "Aparelho conectado", color: "text-success", animate: false },
  error: { icon: AlertCircle, text: "Não foi possível conectar", color: "text-destructive", animate: false },
  expired: { icon: AlertCircle, text: "O código expirou", color: "text-destructive", animate: false },
};

export function CodeDisplay() {
  const { pairingCode, expiresIn, setStep, setJid } = usePairing();
  const [countdown, setCountdown] = useState(expiresIn);
  const [status, setStatus] = useState<PairingStatus>("waiting");

  const copyCode = () => {
    navigator.clipboard.writeText(pairingCode);
    toast.success("Código copiado");
  };

  const checkStatus = useCallback(async () => {
    try {
      const api = new WhatsAppAPI();
      const result = await api.getPairingStatus();

      if (result.complete) {
        setStatus("success");
        const connStatus = await api.getConnectionStatus();
        if (connStatus.jid) {
          setJid(connStatus.jid);
        }
        toast.success("Aparelho conectado", {
          description: "Seu WhatsApp está pronto para uso",
        });
        setTimeout(() => setStep("dashboard"), 1500);
      } else if (result.error) {
        setStatus("error");
        toast.error("Não foi possível conectar", { description: result.error });
      }
    } catch (error) {
      const msg = getErrorMessage(error);
      console.error("Polling error:", msg);
    }
  }, [setJid, setStep]);

  useEffect(() => {
    if (status !== "waiting") return;

    const countdownInterval = setInterval(() => {
      setCountdown((prev) => {
        if (prev <= 1) {
          setStatus("expired");
          return 0;
        }
        return prev - 1;
      });
    }, 1000);

    const pollInterval = setInterval(checkStatus, 2000);

    return () => {
      clearInterval(countdownInterval);
      clearInterval(pollInterval);
    };
  }, [status, checkStatus]);

  const currentStatus = statusConfigs[status];
  const StatusIcon = currentStatus.icon;

  return (
    <Card className="w-full max-w-md">
      <CardHeader>
        <CardTitle>Digite este código no celular</CardTitle>
        <CardDescription>Abra o WhatsApp e siga os passos abaixo</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="relative">
          <div
            className="text-3xl font-mono font-semibold text-center py-8 px-4 rounded-lg bg-primary text-primary-foreground tracking-widest cursor-pointer hover:opacity-90 transition-opacity"
            onClick={copyCode}
            title="Click to copy"
          >
            {pairingCode}
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="absolute top-2 right-2 text-primary-foreground/70 hover:text-primary-foreground hover:bg-primary-foreground/20"
            onClick={copyCode}
          >
            <Copy className="h-4 w-4" />
          </Button>
        </div>

        <div className="flex items-center gap-2">
          <Clock className="h-4 w-4 text-muted-foreground" />
          <span className={countdown <= 30 ? "text-destructive font-medium" : "text-muted-foreground"}>
            {countdown > 0 ? `Faltam ${countdown}s` : "Expirado"}
          </span>
        </div>

        <div className={"flex items-center justify-center gap-2 " + currentStatus.color}>
          <StatusIcon className={"h-5 w-5 " + (currentStatus.animate ? "animate-spin" : "")} />
          <span>{currentStatus.text}</span>
        </div>

        <div className="border rounded-lg p-4">
          <div className="flex items-center gap-2 mb-4">
            <Smartphone className="h-5 w-5 text-success" />
            <span className="font-medium">No seu celular:</span>
          </div>

          <Tabs defaultValue="android" className="w-full">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="android">Android</TabsTrigger>
              <TabsTrigger value="ios">iOS</TabsTrigger>
            </TabsList>
            <TabsContent value="android" className="mt-4">
              <ol className="list-decimal list-inside space-y-2 text-sm text-muted-foreground">
                <li>Abra o <strong className="text-foreground">WhatsApp</strong></li>
                <li>Toque em <strong className="text-foreground">Configurações</strong></li>
                <li>Toque em <strong className="text-foreground">Aparelhos conectados</strong></li>
                <li>Toque em <strong className="text-foreground">Conectar aparelho</strong></li>
                <li>Escolha <strong className="text-foreground">Conectar com número de telefone</strong></li>
                <li>Informe o número e toque em <strong className="text-foreground">Avançar</strong></li>
                <li>Digite o código <Badge variant="secondary" className="font-mono">{pairingCode}</Badge></li>
              </ol>
            </TabsContent>
            <TabsContent value="ios" className="mt-4">
              <ol className="list-decimal list-inside space-y-2 text-sm text-muted-foreground">
                <li>Abra o <strong className="text-foreground">WhatsApp</strong></li>
                <li>Toque em <strong className="text-foreground">Configurações</strong></li>
                <li>Toque em <strong className="text-foreground">Aparelhos conectados</strong></li>
                <li>Toque em <strong className="text-foreground">Conectar aparelho</strong></li>
                <li>Escolha <strong className="text-foreground">Conectar com número de telefone</strong></li>
                <li>Informe o número e toque em <strong className="text-foreground">Avançar</strong></li>
                <li>Digite o código <Badge variant="secondary" className="font-mono">{pairingCode}</Badge></li>
              </ol>
            </TabsContent>
          </Tabs>
        </div>

        {status === "expired" && (
          <Button variant="outline" className="w-full" onClick={() => setStep("phone")}>
            Gerar novo código
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
