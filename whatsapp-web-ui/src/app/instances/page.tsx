"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import {
  Smartphone,
  Plus,
  RefreshCw,
  PowerOff,
  Trash2,
  Loader2,
  User,
  ShieldCheck,
} from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { WhatsAppAPI, Instance, Employee } from "@/lib/api";

export default function InstancesPage() {
  const [instances, setInstances] = useState<Instance[]>([]);
  const [employees, setEmployees] = useState<Employee[]>([]);
  const [loading, setLoading] = useState(true);

  // Pairing Modal state
  const [pairingOpen, setPairingOpen] = useState(false);
  const [alias, setAlias] = useState("");
  const [selectedEmpId, setSelectedEmpId] = useState<string>("none");
  const [isGeneratingQR, setIsGeneratingQR] = useState(false);
  const [qrCode, setQrCode] = useState<string | null>(null);

  // Action states
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [deleteTargetJid, setDeleteTargetJid] = useState<string | null>(null);

  const api = useMemo(() => new WhatsAppAPI(), []);

  const loadData = useCallback(async () => {
    try {
      const [instData, empData] = await Promise.all([
        api.getInstances(),
        api.getEmployees(),
      ]);
      setInstances(instData);
      setEmployees(empData);
    } catch (err: unknown) {
      toast.error("Erro ao carregar instâncias", {
        description: err instanceof Error ? err.message : "Falha na comunicação",
      });
    } finally {
      setLoading(false);
    }
  }, [api]);

  useEffect(() => {
    let ignore = false;
    Promise.all([api.getInstances(), api.getEmployees()])
      .then(([instData, empData]) => {
        if (!ignore) {
          setInstances(instData);
          setEmployees(empData);
          setLoading(false);
        }
      })
      .catch((err: unknown) => {
        if (!ignore) {
          toast.error("Erro ao carregar instâncias", {
            description: err instanceof Error ? err.message : "Falha na comunicação",
          });
          setLoading(false);
        }
      });

    const interval = setInterval(loadData, 8000);
    return () => {
      ignore = true;
      clearInterval(interval);
    };
  }, [api, loadData]);

  const openNewPairing = () => {
    setAlias("");
    setSelectedEmpId("none");
    setQrCode(null);
    setIsGeneratingQR(false);
    setPairingOpen(true);
  };

  const handleGenerateQR = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!alias.trim()) {
      toast.error("Informe um nome/identificador para a instância.");
      return;
    }

    try {
      setIsGeneratingQR(true);
      const empId = selectedEmpId !== "none" ? parseInt(selectedEmpId, 10) : null;
      const res = await api.createInstancePair(alias.trim(), empId);

      if (res.qr_code) {
        setQrCode(res.qr_code);
        toast.info("QR Code gerado! Aponte o WhatsApp para escanear.");
      } else {
        toast.warning("QR Code em processamento. Aguarde alguns segundos...");
      }
    } catch (err: unknown) {
      toast.error("Erro ao iniciar pareamento", {
        description: err instanceof Error ? err.message : "Ocorreu um erro no servidor",
      });
    } finally {
      setIsGeneratingQR(false);
    }
  };

  const handleReconnect = async (jid: string) => {
    try {
      setActionLoading(`reconnect-${jid}`);
      await api.reconnectInstance(jid);
      toast.success("Comando de reconexão enviado.");
      await loadData();
    } catch (err: unknown) {
      toast.error("Erro ao reconectar instância", {
        description: err instanceof Error ? err.message : "Falha ao enviar comando",
      });
    } finally {
      setActionLoading(null);
    }
  };

  const handleDisconnect = async (jid: string) => {
    try {
      setActionLoading(`disconnect-${jid}`);
      await api.disconnectInstance(jid);
      toast.info("Instância desconectada.");
      await loadData();
    } catch (err: unknown) {
      toast.error("Erro ao desconectar instância", {
        description: err instanceof Error ? err.message : "Falha ao enviar comando",
      });
    } finally {
      setActionLoading(null);
    }
  };

  const handleDelete = async () => {
    if (!deleteTargetJid) return;
    try {
      await api.deleteInstance(deleteTargetJid);
      toast.success("Instância removida com sucesso.");
      setDeleteTargetJid(null);
      await loadData();
    } catch (err: unknown) {
      toast.error("Erro ao remover instância", {
        description: err instanceof Error ? err.message : "Falha ao excluir",
      });
    }
  };

  return (
    <PageContainer>
      <PageHeader
        title="Instâncias WhatsApp"
        description="Acompanhe aparelhos corporativos conectados e adicione novos números via Aparelhos Conectados."
        actions={
          <Button onClick={openNewPairing} className="gap-2">
            <Plus className="size-4" />
            Conectar Nova Instância
          </Button>
        }
      />

      {/* Enterprise Disclaimer Card */}
      <div className="flex items-start gap-3 rounded-lg border border-primary/20 bg-primary/5 p-4 text-sm text-foreground">
        <ShieldCheck className="size-5 shrink-0 text-primary mt-0.5" />
        <div className="space-y-1">
          <p className="font-semibold text-primary">Arquitetura de Baixo Risco & Governança Passiva</p>
          <p className="text-muted-foreground text-xs leading-relaxed">
            As instâncias são vinculadas como <strong>Aparelhos Conectados (Multi-Device)</strong>. O colaborador mantém o celular físico e utiliza o aplicativo WhatsApp Business normalmente. O sistema atua de maneira passiva ingerindo mensagens para fins de conformidade e inteligência artificial, minimizando riscos de restrição de conta.
          </p>
        </div>
      </div>

      {loading ? (
        <div className="flex h-48 items-center justify-center">
          <Loader2 className="size-8 animate-spin text-muted-foreground" />
        </div>
      ) : instances.length === 0 ? (
        <Card className="flex flex-col items-center justify-center py-12 text-center">
          <div className="flex size-14 items-center justify-center rounded-full bg-muted">
            <Smartphone className="size-7 text-muted-foreground" />
          </div>
          <CardTitle className="mt-4 text-lg">Nenhuma instância conectada</CardTitle>
          <CardDescription className="max-w-sm mt-1">
            Conecte o primeiro aparelho WhatsApp escaneando o QR Code pelo aplicativo no celular.
          </CardDescription>
          <Button onClick={openNewPairing} className="mt-6 gap-2">
            <Plus className="size-4" />
            Conectar Primeira Instância
          </Button>
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {instances.map((inst) => {
            const isConnected = inst.status === "connected" && inst.is_active;
            const isPairing = inst.status === "pairing";

            return (
              <Card key={inst.jid} className="relative overflow-hidden transition-all hover:border-primary/50">
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between">
                    <div>
                      <div className="flex items-center gap-2">
                        <CardTitle className="text-base font-semibold">
                          {inst.alias || "Instância WhatsApp"}
                        </CardTitle>
                        <Badge
                          variant={isConnected ? "default" : isPairing ? "secondary" : "outline"}
                          className="text-[11px] gap-1"
                        >
                          <span
                            className={`size-1.5 rounded-full ${
                              isConnected ? "bg-emerald-400" : isPairing ? "bg-amber-400" : "bg-zinc-400"
                            }`}
                          />
                          {isConnected ? "Conectado" : isPairing ? "Pareando..." : "Desconectado"}
                        </Badge>
                      </div>
                      <p className="text-xs text-muted-foreground font-mono mt-1">
                        {inst.phone_number || inst.jid}
                      </p>
                    </div>

                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-8 text-muted-foreground hover:text-destructive"
                      onClick={() => setDeleteTargetJid(inst.jid)}
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  </div>
                </CardHeader>

                <CardContent className="space-y-3 pt-0">
                  <div className="flex flex-wrap items-center gap-2">
                    {inst.employee_name ? (
                      <Badge variant="secondary" className="gap-1 text-xs">
                        <User className="size-3" />
                        {inst.employee_name}
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-xs text-muted-foreground">
                        Sem colaborador vinculado
                      </Badge>
                    )}
                  </div>

                  <div className="flex items-center justify-between border-t pt-3">
                    <span className="text-[11px] text-muted-foreground">
                      {inst.connected_at
                        ? `Conectado: ${new Date(inst.connected_at).toLocaleString("pt-BR")}`
                        : "Nunca conectado"}
                    </span>

                    <div className="flex items-center gap-1">
                      {isConnected ? (
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-8 gap-1.5 text-xs text-muted-foreground hover:text-destructive"
                          disabled={actionLoading === `disconnect-${inst.jid}`}
                          onClick={() => handleDisconnect(inst.jid)}
                        >
                          {actionLoading === `disconnect-${inst.jid}` ? (
                            <Loader2 className="size-3.5 animate-spin" />
                          ) : (
                            <PowerOff className="size-3.5" />
                          )}
                          Desconectar
                        </Button>
                      ) : (
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-8 gap-1.5 text-xs text-primary hover:text-primary"
                          disabled={actionLoading === `reconnect-${inst.jid}`}
                          onClick={() => handleReconnect(inst.jid)}
                        >
                          {actionLoading === `reconnect-${inst.jid}` ? (
                            <Loader2 className="size-3.5 animate-spin" />
                          ) : (
                            <RefreshCw className="size-3.5" />
                          )}
                          Reconectar
                        </Button>
                      )}
                    </div>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {/* Modal de Pareamento / Conexão */}
      <Dialog open={pairingOpen} onOpenChange={setPairingOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Conectar Aparelho WhatsApp</DialogTitle>
            <DialogDescription>
              {qrCode
                ? "Abra o WhatsApp no celular e escaneie o código abaixo em Aparelhos Conectados."
                : "Defina o nome de identificação da instância e o colaborador vinculado."}
            </DialogDescription>
          </DialogHeader>

          {!qrCode ? (
            <form onSubmit={handleGenerateQR} className="space-y-4 py-2">
              <div className="grid gap-2">
                <Label htmlFor="inst-alias">Nome da Instância *</Label>
                <Input
                  id="inst-alias"
                  placeholder="Ex: Celular Vendas 01, Suporte Suíte 2"
                  value={alias}
                  onChange={(e) => setAlias(e.target.value)}
                  autoFocus
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="inst-emp">Colaborador Vinculado (Opcional)</Label>
                <Select value={selectedEmpId} onValueChange={setSelectedEmpId}>
                  <SelectTrigger id="inst-emp">
                    <SelectValue placeholder="Selecione um colaborador" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">Nenhum / Uso Geral</SelectItem>
                    {employees.map((emp) => (
                      <SelectItem key={emp.id} value={emp.id.toString()}>
                        {emp.name} {emp.role ? `(${emp.role})` : ""}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <DialogFooter className="pt-2">
                <Button type="button" variant="outline" onClick={() => setPairingOpen(false)}>
                  Cancelar
                </Button>
                <Button type="submit" disabled={isGeneratingQR}>
                  {isGeneratingQR && <Loader2 className="mr-2 size-4 animate-spin" />}
                  Gerar QR Code
                </Button>
              </DialogFooter>
            </form>
          ) : (
            <div className="flex flex-col items-center py-4 space-y-4 text-center">
              <div className="rounded-xl border bg-white p-4 shadow-sm">
                <QRCodeSVG value={qrCode} size={230} level="M" />
              </div>

              <div className="space-y-1.5 text-xs text-muted-foreground max-w-xs">
                <p className="font-semibold text-foreground">Como conectar:</p>
                <p>1. No WhatsApp do celular, vá em <strong>Configurações</strong>.</p>
                <p>2. Toque em <strong>Aparelhos Conectados</strong>.</p>
                <p>3. Toque em <strong>Conectar um aparelho</strong> e aponte a câmera.</p>
              </div>

              <Button
                variant="outline"
                className="w-full"
                onClick={() => {
                  setPairingOpen(false);
                  loadData();
                }}
              >
                Concluir / Fechar
              </Button>
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* Confirmação de Exclusão */}
      <AlertDialog open={deleteTargetJid !== null} onOpenChange={(open) => !open && setDeleteTargetJid(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remover Instância</AlertDialogTitle>
            <AlertDialogDescription>
              Tem certeza que deseja desconectar e desvincular este aparelho? A sessão no WhatsApp Web será encerrada.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction onClick={handleDelete} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
              Desconectar e Excluir
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </PageContainer>
  );
}
