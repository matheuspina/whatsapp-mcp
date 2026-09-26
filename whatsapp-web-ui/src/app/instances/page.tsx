"use client";

import { useEffect, useState, useCallback, useMemo, useRef } from "react";
import {
  Smartphone,
  Plus,
  RefreshCw,
  PowerOff,
  Trash2,
  Loader2,
  User,
  ShieldCheck,
  ShieldAlert,
  Send,
  Eye,
  Search,
  MoreHorizontal,
  Info,
  CheckCircle2,
  X,
  LayoutGrid,
  Table as TableIcon,
  ChevronDown,
  ChevronUp,
} from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { WhatsAppAPI, Instance, Employee, PairingState } from "@/lib/api";

const CORPORATE_TERMS =
  "Declaro que este número de WhatsApp é um ativo da empresa, usado para atividades profissionais, " +
  "e que o colaborador responsável foi informado de que as conversas serão registradas e poderão ser " +
  "auditadas, conforme a política interna e a LGPD.";

const QR_POLL_MS = 2000;
const LIST_POLL_MS = 8000;
const NO_EMPLOYEE = "none";

function errorText(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback;
}

function formatPhone(inst: Instance): string {
  if (!inst.phone_number) return "Aguardando leitura do QR Code";
  const num = inst.phone_number;
  if (num.startsWith("55") && num.length >= 12) {
    const ddd = num.slice(2, 4);
    const rest = num.slice(4);
    if (rest.length === 9) {
      return `+55 (${ddd}) ${rest.slice(0, 5)}-${rest.slice(5)}`;
    }
    if (rest.length === 8) {
      return `+55 (${ddd}) ${rest.slice(0, 4)}-${rest.slice(4)}`;
    }
  }
  return `+${num}`;
}

interface Pairing {
  id: number;
  qr: string | null;
  state: PairingState;
}

export default function InstancesPage() {
  const [instances, setInstances] = useState<Instance[]>([]);
  const [employees, setEmployees] = useState<Employee[]>([]);
  const [loading, setLoading] = useState(true);

  // Filters & Views
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "online" | "offline" | "unconfirmed">("all");
  const [viewMode, setViewMode] = useState<"grid" | "table">("grid");
  const [showHowItWorks, setShowHowItWorks] = useState(false);

  // Pairing dialog
  const [pairingOpen, setPairingOpen] = useState(false);
  const [alias, setAlias] = useState("");
  const [employeeId, setEmployeeId] = useState<string>(NO_EMPLOYEE);
  const [allowSend, setAllowSend] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const [starting, setStarting] = useState(false);
  const [pairing, setPairing] = useState<Pairing | null>(null);
  const pairingId = useRef<number | null>(null);

  // Confirmation of legacy numbers
  const [confirmTarget, setConfirmTarget] = useState<Instance | null>(null);
  const [confirmChecked, setConfirmChecked] = useState(false);

  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [removeTarget, setRemoveTarget] = useState<Instance | null>(null);

  const api = useMemo(() => new WhatsAppAPI(), []);

  const loadData = useCallback(async () => {
    try {
      const [instData, empData] = await Promise.all([api.getInstances(), api.getEmployees()]);
      setInstances(instData);
      setEmployees(empData.filter((e) => e.active));
    } catch (err: unknown) {
      toast.error("Erro ao carregar instâncias", { description: errorText(err, "Falha na comunicação") });
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
          setEmployees(empData.filter((e) => e.active));
          setLoading(false);
        }
      })
      .catch((err: unknown) => {
        if (!ignore) {
          toast.error("Erro ao carregar instâncias", { description: errorText(err, "Falha na comunicação") });
          setLoading(false);
        }
      });

    const interval = setInterval(loadData, LIST_POLL_MS);
    return () => {
      ignore = true;
      clearInterval(interval);
    };
  }, [api, loadData]);

  // QR Code rotation polling
  const activePairingId = pairing && pairing.state === "pending" ? pairing.id : null;
  useEffect(() => {
    if (activePairingId === null) return;
    let ignore = false;
    const interval = setInterval(() => {
      api
        .getInstanceQR(activePairingId)
        .then((res) => {
          if (ignore) return;
          setPairing((current) =>
            current && current.id === activePairingId
              ? { ...current, state: res.status, qr: res.qr_code || current.qr }
              : current
          );
          if (res.status === "paired") {
            pairingId.current = null;
            toast.success("Número conectado com sucesso!");
            setPairingOpen(false);
            setPairing(null);
            loadData();
          }
        })
        .catch(() => {
          /* next tick retries */
        });
    }, QR_POLL_MS);
    return () => {
      ignore = true;
      clearInterval(interval);
    };
  }, [activePairingId, api, loadData]);

  const openNewPairing = () => {
    setAlias("");
    setEmployeeId(NO_EMPLOYEE);
    setAllowSend(false);
    setConfirmed(false);
    setPairing(null);
    setStarting(false);
    setPairingOpen(true);
  };

  const closePairing = async () => {
    const abandoned = pairingId.current;
    pairingId.current = null;
    setPairingOpen(false);
    setPairing(null);
    if (abandoned !== null) {
      try {
        await api.removeInstance(abandoned);
      } catch {
        /* expires automatically */
      }
      loadData();
    }
  };

  const handleStartPairing = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!alias.trim()) {
      toast.error("Informe um nome para identificar o aparelho.");
      return;
    }
    if (!confirmed) {
      toast.error("É obrigatório concordar com o termo de ativo corporativo e LGPD.");
      return;
    }
    try {
      setStarting(true);
      const res = await api.pairInstance({
        alias: alias.trim(),
        employee_id: employeeId !== NO_EMPLOYEE ? parseInt(employeeId, 10) : null,
        allow_send: allowSend,
        corporate_asset_confirmed: true,
      });
      if (!res.instance) throw new Error(res.error || "Resposta inesperada do servidor");
      pairingId.current = res.instance.id;
      setPairing({ id: res.instance.id, qr: res.qr_code || null, state: "pending" });
      if (!res.qr_code) toast.info("Gerando QR Code...");
    } catch (err: unknown) {
      toast.error("Erro ao iniciar pareamento", { description: errorText(err, "Ocorreu um erro no servidor") });
    } finally {
      setStarting(false);
    }
  };

  const runAction = async (key: string, action: () => Promise<unknown>, success: string, failure: string) => {
    try {
      setActionLoading(key);
      await action();
      toast.success(success);
      await loadData();
    } catch (err: unknown) {
      toast.error(failure, { description: errorText(err, "Falha ao executar a ação") });
    } finally {
      setActionLoading(null);
    }
  };

  const handleEmployeeChange = (inst: Instance, value: string) =>
    runAction(
      `employee-${inst.id}`,
      () => api.updateInstance(inst.id, { employee_id: value === NO_EMPLOYEE ? null : parseInt(value, 10) }),
      "Responsável atualizado com sucesso.",
      "Erro ao vincular colaborador"
    );

  const handleAllowSend = (inst: Instance, allow: boolean) =>
    runAction(
      `send-${inst.id}`,
      () => api.updateInstance(inst.id, { allow_send: allow }),
      allow ? "Envio habilitado para este número." : "Número agora está em modo somente leitura.",
      "Erro ao alterar permissão de envio"
    );

  const handleConfirmAsset = async () => {
    if (!confirmTarget) return;
    const target = confirmTarget;
    setConfirmTarget(null);
    await runAction(
      `confirm-${target.id}`,
      () => api.updateInstance(target.id, { corporate_asset_confirmed: true }),
      "Ativo corporativo e LGPD confirmados.",
      "Erro ao confirmar ativo"
    );
  };

  const handleRemove = async () => {
    if (!removeTarget) return;
    const target = removeTarget;
    setRemoveTarget(null);
    await runAction(
      `remove-${target.id}`,
      () => api.removeInstance(target.id),
      "Número desconectado e removido do monitoramento.",
      "Erro ao remover número"
    );
  };

  const visible = instances.filter((i) => i.status !== "pairing" || i.id === pairing?.id);
  const unconfirmed = visible.filter((i) => !i.corporate_asset_confirmed && i.status !== "pairing");

  // Filtering
  const filteredInstances = useMemo(() => {
    return visible.filter((inst) => {
      const q = search.trim().toLowerCase();
      const matchesSearch =
        !q ||
        (inst.alias && inst.alias.toLowerCase().includes(q)) ||
        (inst.phone_number && inst.phone_number.includes(q)) ||
        (inst.employee_name && inst.employee_name.toLowerCase().includes(q)) ||
        (inst.department_name && inst.department_name.toLowerCase().includes(q));

      const matchesStatus =
        statusFilter === "all" ||
        (statusFilter === "online" && inst.live) ||
        (statusFilter === "offline" && !inst.live && inst.status !== "pairing") ||
        (statusFilter === "unconfirmed" && !inst.corporate_asset_confirmed);

      return matchesSearch && matchesStatus;
    });
  }, [visible, search, statusFilter]);

  // Stats
  const onlineCount = visible.filter((i) => i.live).length;
  const sendAllowedCount = visible.filter((i) => i.allow_send).length;

  return (
    <PageContainer>
      <PageHeader
        title="Números WhatsApp"
        description="Aparelhos conectados para monitoramento e auditoria em conformidade com as diretrizes corporativas."
        actions={
          <Button onClick={openNewPairing} className="gap-2 shadow-sm">
            <Plus className="size-4" />
            Conectar Novo Aparelho
          </Button>
        }
      />

      {/* KPI Cards */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Card className="p-4 shadow-sm border-border/70">
          <div className="flex items-center justify-between text-muted-foreground">
            <span className="text-xs font-medium uppercase tracking-wider">Total</span>
            <Smartphone className="size-4 text-primary" />
          </div>
          <p className="mt-2 text-2xl font-bold tracking-tight text-foreground">
            {loading ? "..." : visible.length}
          </p>
          <span className="text-[11px] text-muted-foreground">aparelhos cadastrados</span>
        </Card>

        <Card className="p-4 shadow-sm border-border/70">
          <div className="flex items-center justify-between text-muted-foreground">
            <span className="text-xs font-medium uppercase tracking-wider">Online</span>
            <span className="size-2 rounded-full bg-emerald-500 animate-pulse" />
          </div>
          <p className="mt-2 text-2xl font-bold tracking-tight text-emerald-600 dark:text-emerald-400">
            {loading ? "..." : onlineCount}
          </p>
          <span className="text-[11px] text-muted-foreground">com conexão ativa</span>
        </Card>

        <Card className="p-4 shadow-sm border-border/70">
          <div className="flex items-center justify-between text-muted-foreground">
            <span className="text-xs font-medium uppercase tracking-wider">Envio Liberado</span>
            <Send className="size-4 text-sky-600 dark:text-sky-400" />
          </div>
          <p className="mt-2 text-2xl font-bold tracking-tight text-foreground">
            {loading ? "..." : sendAllowedCount}
          </p>
          <span className="text-[11px] text-muted-foreground">com permissão de disparo</span>
        </Card>

        <Card className="p-4 shadow-sm border-border/70">
          <div className="flex items-center justify-between text-muted-foreground">
            <span className="text-xs font-medium uppercase tracking-wider">LGPD & Compliance</span>
            <ShieldCheck className="size-4 text-emerald-600 dark:text-emerald-400" />
          </div>
          <p className="mt-2 text-2xl font-bold tracking-tight text-foreground">
            {loading ? "..." : `${visible.length - unconfirmed.length}/${visible.length}`}
          </p>
          <span className="text-[11px] text-muted-foreground">termos confirmados</span>
        </Card>
      </div>

      {/* Pending attestation alert */}
      {unconfirmed.length > 0 && (
        <Card className="border-warning/50 bg-warning/5 p-4 shadow-sm">
          <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
            <div className="flex items-center gap-3">
              <div className="flex size-9 items-center justify-center rounded-lg bg-warning/15 text-warning shrink-0">
                <ShieldAlert className="size-5" />
              </div>
              <div>
                <p className="text-sm font-semibold text-foreground">
                  {unconfirmed.length === 1
                    ? "1 número com termo corporativo pendente"
                    : `${unconfirmed.length} números com termo corporativo pendente`}
                </p>
                <p className="text-xs text-muted-foreground">
                  Números sem termo confirmado podem não registrar mensagens conforme políticas de compliance e LGPD.
                </p>
              </div>
            </div>
            <Button
              variant="outline"
              size="sm"
              className="border-warning/50 text-xs font-medium shrink-0"
              onClick={() => setStatusFilter("unconfirmed")}
            >
              Ver pendentes ({unconfirmed.length})
            </Button>
          </div>
        </Card>
      )}

      {/* How it works expandable card */}
      <Card className="border-border/70 shadow-sm overflow-hidden">
        <button
          type="button"
          onClick={() => setShowHowItWorks(!showHowItWorks)}
          className="flex w-full items-center justify-between p-3.5 text-left text-xs font-medium text-muted-foreground hover:text-foreground transition-colors"
        >
          <span className="flex items-center gap-2">
            <Info className="size-4 text-primary" />
            Como funciona o monitoramento de aparelhos WhatsApp
          </span>
          {showHowItWorks ? <ChevronUp className="size-4" /> : <ChevronDown className="size-4" />}
        </button>

        {showHowItWorks && (
          <CardContent className="border-t bg-muted/20 p-4 text-xs leading-relaxed text-muted-foreground space-y-2">
            <p>
              • <strong>Aparelho Conectado:</strong> O número é emparelhado como sessão web vinculada. O colaborador continua usando o WhatsApp normalmente no celular e o bridge registra eventos para auditoria.
            </p>
            <p>
              • <strong>Somente Leitura por Padrão:</strong> Para segurança da conta, o envio por IA ou automação é desativado por padrão e deve ser ativado individualmente por aparelho.
            </p>
            <p>
              • <strong>Validade da Sessão:</strong> O WhatsApp desvincula automaticamente aparelhos cujo celular fique mais de 14 dias sem conexão com a internet.
            </p>
          </CardContent>
        )}
      </Card>

      {/* Filter and View Controls */}
      <div className="flex flex-col sm:flex-row items-center justify-between gap-3">
        <div className="flex flex-1 items-center gap-2 w-full max-w-lg">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="Buscar por nome, telefone ou colaborador..."
              className="pl-9 pr-9"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            {search && (
              <button
                type="button"
                onClick={() => setSearch("")}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
              >
                <X className="size-4" />
              </button>
            )}
          </div>

          <Select
            value={statusFilter}
            onValueChange={(v) => setStatusFilter(v as "all" | "online" | "offline" | "unconfirmed")}
          >
            <SelectTrigger className="w-36">
              <SelectValue placeholder="Status" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Todos</SelectItem>
              <SelectItem value="online">Online</SelectItem>
              <SelectItem value="offline">Offline</SelectItem>
              <SelectItem value="unconfirmed">Sem LGPD</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className="flex items-center gap-2 self-end sm:self-auto">
          <span className="text-xs text-muted-foreground mr-1">
            {filteredInstances.length} {filteredInstances.length === 1 ? "aparelho" : "aparelhos"}
          </span>

          <div className="flex items-center rounded-lg border bg-muted/30 p-0.5">
            <Button
              variant={viewMode === "grid" ? "secondary" : "ghost"}
              size="icon"
              className="size-8"
              onClick={() => setViewMode("grid")}
              title="Visualização em Grade"
            >
              <LayoutGrid className="size-4" />
            </Button>
            <Button
              variant={viewMode === "table" ? "secondary" : "ghost"}
              size="icon"
              className="size-8"
              onClick={() => setViewMode("table")}
              title="Visualização em Tabela"
            >
              <TableIcon className="size-4" />
            </Button>
          </div>
        </div>
      </div>

      {/* Content */}
      {loading ? (
        <Card className="p-12">
          <div className="flex flex-col items-center justify-center gap-3 text-center">
            <Loader2 className="size-8 animate-spin text-primary" />
            <p className="text-sm text-muted-foreground">Carregando aparelhos monitorados...</p>
          </div>
        </Card>
      ) : visible.length === 0 ? (
        <Card className="border-dashed p-12">
          <div className="flex flex-col items-center justify-center gap-3 text-center">
            <div className="flex size-14 items-center justify-center rounded-full bg-muted">
              <Smartphone className="size-7 text-muted-foreground" />
            </div>
            <CardTitle className="text-base font-semibold">Nenhum aparelho conectado</CardTitle>
            <CardDescription className="max-w-md text-xs">
              Conecte os números corporativos para iniciar a captura de mensagens, desambiguação de contexto por IA e auditoria de atendimento.
            </CardDescription>
            <Button onClick={openNewPairing} className="mt-2 gap-2">
              <Plus className="size-4" />
              Conectar Primeiro Aparelho
            </Button>
          </div>
        </Card>
      ) : filteredInstances.length === 0 ? (
        <Card className="border-dashed p-8">
          <div className="flex flex-col items-center justify-center gap-2 text-center">
            <Search className="size-6 text-muted-foreground" />
            <p className="text-sm font-medium">Nenhum aparelho encontrado para os filtros selecionados.</p>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setSearch("");
                setStatusFilter("all");
              }}
              className="mt-1 text-xs"
            >
              Limpar filtros
            </Button>
          </div>
        </Card>
      ) : viewMode === "grid" ? (
        /* Grid Cards View */
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filteredInstances.map((inst) => {
            const isOnline = inst.live;
            const isActionLoading = actionLoading?.includes(`-${inst.id}`);

            return (
              <Card
                key={inst.id}
                className="group relative flex flex-col justify-between overflow-hidden transition-all duration-200 hover:border-primary/50 hover:shadow-md"
              >
                {/* Header */}
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-3 min-w-0">
                      <div
                        className={`flex size-10 items-center justify-center rounded-lg shrink-0 ${
                          isOnline
                            ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                            : "bg-muted text-muted-foreground"
                        }`}
                      >
                        <Smartphone className="size-5" />
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2 flex-wrap">
                          <CardTitle className="text-base font-semibold truncate" title={inst.alias}>
                            {inst.alias || "Número WhatsApp"}
                          </CardTitle>
                          {isOnline ? (
                            <Badge variant="success" className="gap-1 text-[11px] py-0">
                              <span className="size-1.5 rounded-full bg-emerald-500 animate-pulse" />
                              Online
                            </Badge>
                          ) : inst.status === "pairing" ? (
                            <Badge variant="warning" className="gap-1 text-[11px] py-0">
                              Pareando
                            </Badge>
                          ) : (
                            <Badge variant="outline" className="text-[11px] text-muted-foreground py-0">
                              Offline
                            </Badge>
                          )}
                        </div>
                        <p className="font-mono text-xs text-muted-foreground mt-0.5">
                          {formatPhone(inst)}
                        </p>
                      </div>
                    </div>

                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-8 text-muted-foreground hover:text-foreground shrink-0"
                          disabled={isActionLoading}
                        >
                          {isActionLoading ? (
                            <Loader2 className="size-4 animate-spin" />
                          ) : (
                            <MoreHorizontal className="size-4" />
                          )}
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        {isOnline ? (
                          <DropdownMenuItem
                            onClick={() =>
                              runAction(
                                `disconnect-${inst.id}`,
                                () => api.disconnectInstance(inst.id),
                                "Aparelho desconectado.",
                                "Erro ao desconectar"
                              )
                            }
                            className="gap-2 cursor-pointer"
                          >
                            <PowerOff className="size-4 text-warning" />
                            Desconectar sessão
                          </DropdownMenuItem>
                        ) : (
                          <DropdownMenuItem
                            onClick={() =>
                              runAction(
                                `reconnect-${inst.id}`,
                                () => api.reconnectInstance(inst.id),
                                "Comando de reconexão enviado.",
                                "Erro ao reconectar"
                              )
                            }
                            className="gap-2 cursor-pointer"
                          >
                            <RefreshCw className="size-4 text-primary" />
                            Tentar reconectar
                          </DropdownMenuItem>
                        )}
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          onClick={() => setRemoveTarget(inst)}
                          className="gap-2 text-destructive focus:text-destructive cursor-pointer"
                        >
                          <Trash2 className="size-4" />
                          Remover aparelho
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </CardHeader>

                {/* Body */}
                <CardContent className="space-y-3.5 pt-0 mt-auto">
                  {/* Responsible employee select */}
                  <div className="grid gap-1.5 rounded-lg border bg-muted/20 p-2.5">
                    <Label className="text-[11px] font-medium text-muted-foreground flex items-center justify-between">
                      <span className="flex items-center gap-1">
                        <User className="size-3" />
                        Colaborador Responsável
                      </span>
                      {inst.department_name && (
                        <span className="text-[10px] text-muted-foreground">
                          Setor: {inst.department_name}
                        </span>
                      )}
                    </Label>
                    <Select
                      value={inst.employee_id ? inst.employee_id.toString() : NO_EMPLOYEE}
                      onValueChange={(v) => handleEmployeeChange(inst, v)}
                      disabled={inst.status === "pairing" || actionLoading === `employee-${inst.id}`}
                    >
                      <SelectTrigger className="h-8 text-xs bg-background">
                        <SelectValue placeholder="Sem colaborador" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={NO_EMPLOYEE}>Sem colaborador vinculado</SelectItem>
                        {employees.map((emp) => (
                          <SelectItem key={emp.id} value={emp.id.toString()}>
                            {emp.name}
                            {emp.department_name ? ` (${emp.department_name})` : ""}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>

                  {/* Send permission toggle */}
                  <div className="flex items-center justify-between gap-3 rounded-lg border p-2.5 bg-background">
                    <div className="flex items-center gap-2">
                      <div className={`p-1 rounded ${inst.allow_send ? "bg-amber-500/10 text-amber-600 dark:text-amber-400" : "bg-muted text-muted-foreground"}`}>
                        {inst.allow_send ? <Send className="size-3.5" /> : <Eye className="size-3.5" />}
                      </div>
                      <div>
                        <p className="text-xs font-semibold leading-tight">
                          {inst.allow_send ? "Disparo Habilitado" : "Somente Leitura"}
                        </p>
                        <p className="text-[10px] text-muted-foreground">
                          {inst.allow_send ? "IA e API podem enviar mensagens" : "Apenas captura de mensagens"}
                        </p>
                      </div>
                    </div>
                    <Switch
                      checked={inst.allow_send}
                      disabled={inst.status === "pairing" || actionLoading === `send-${inst.id}`}
                      onCheckedChange={(v) => handleAllowSend(inst, v)}
                      aria-label="Permitir envio"
                    />
                  </div>

                  {/* Compliance & LGPD badge/action */}
                  <div>
                    {inst.corporate_asset_confirmed ? (
                      <Badge variant="success" className="gap-1 text-[11px] font-normal py-0.5">
                        <CheckCircle2 className="size-3" />
                        Termo LGPD Confirmado
                      </Badge>
                    ) : (
                      <Button
                        variant="outline"
                        size="sm"
                        className="w-full h-8 gap-1.5 text-xs border-warning/50 text-warning hover:bg-warning/10"
                        onClick={() => {
                          setConfirmChecked(false);
                          setConfirmTarget(inst);
                        }}
                      >
                        <ShieldAlert className="size-3.5" />
                        Confirmar Termo de Ativo
                      </Button>
                    )}
                  </div>

                  {/* Footer metadata */}
                  <div className="flex items-center justify-between border-t border-border/50 pt-2.5 text-[11px] text-muted-foreground">
                    <span>
                      {inst.last_seen_at
                        ? `Visto: ${new Date(inst.last_seen_at).toLocaleDateString("pt-BR")} às ${new Date(
                            inst.last_seen_at
                          ).toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" })}`
                        : "Nunca conectado"}
                    </span>

                    {isOnline ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-6 px-2 text-[11px] text-muted-foreground hover:text-destructive"
                        disabled={actionLoading === `disconnect-${inst.id}`}
                        onClick={() =>
                          runAction(
                            `disconnect-${inst.id}`,
                            () => api.disconnectInstance(inst.id),
                            "Número desconectado.",
                            "Erro ao desconectar"
                          )
                        }
                      >
                        Desconectar
                      </Button>
                    ) : (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-6 px-2 text-[11px] text-primary hover:text-primary"
                        disabled={actionLoading === `reconnect-${inst.id}`}
                        onClick={() =>
                          runAction(
                            `reconnect-${inst.id}`,
                            () => api.reconnectInstance(inst.id),
                            "Comando de reconexão enviado.",
                            "Erro ao reconectar"
                          )
                        }
                      >
                        Reconectar
                      </Button>
                    )}
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      ) : (
        /* Table View */
        <Card className="overflow-hidden border-border/80 shadow-sm">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[280px]">Aparelho</TableHead>
                <TableHead className="w-[120px]">Status</TableHead>
                <TableHead className="w-[220px]">Responsável</TableHead>
                <TableHead className="w-[140px]">Permissão</TableHead>
                <TableHead className="w-[160px]">Compliance LGPD</TableHead>
                <TableHead className="w-[80px] text-right">Ações</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredInstances.map((inst) => {
                const isOnline = inst.live;
                return (
                  <TableRow key={inst.id} className="group">
                    <TableCell>
                      <div className="flex items-center gap-3">
                        <div
                          className={`flex size-8 items-center justify-center rounded-md shrink-0 ${
                            isOnline
                              ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                              : "bg-muted text-muted-foreground"
                          }`}
                        >
                          <Smartphone className="size-4" />
                        </div>
                        <div>
                          <p className="font-semibold text-foreground text-sm">{inst.alias}</p>
                          <p className="font-mono text-xs text-muted-foreground">{formatPhone(inst)}</p>
                        </div>
                      </div>
                    </TableCell>

                    <TableCell>
                      {isOnline ? (
                        <Badge variant="success" className="gap-1 text-[11px]">
                          <span className="size-1.5 rounded-full bg-emerald-500 animate-pulse" />
                          Online
                        </Badge>
                      ) : (
                        <Badge variant="outline" className="text-[11px] text-muted-foreground">
                          Offline
                        </Badge>
                      )}
                    </TableCell>

                    <TableCell>
                      <div className="flex items-center gap-2">
                        {inst.employee_name ? (
                          <div className="flex items-center gap-2">
                            <Avatar className="size-6 text-[10px]">
                              <AvatarFallback>{inst.employee_name.slice(0, 2).toUpperCase()}</AvatarFallback>
                            </Avatar>
                            <div>
                              <p className="text-xs font-medium text-foreground">{inst.employee_name}</p>
                              {inst.department_name && (
                                <p className="text-[10px] text-muted-foreground">{inst.department_name}</p>
                              )}
                            </div>
                          </div>
                        ) : (
                          <span className="text-xs text-muted-foreground italic">Sem responsável</span>
                        )}
                      </div>
                    </TableCell>

                    <TableCell>
                      {inst.allow_send ? (
                        <Badge variant="warning" className="gap-1 text-[11px]">
                          <Send className="size-3" />
                          Envio Ativo
                        </Badge>
                      ) : (
                        <Badge variant="outline" className="gap-1 text-[11px] text-muted-foreground">
                          <Eye className="size-3" />
                          Leitura
                        </Badge>
                      )}
                    </TableCell>

                    <TableCell>
                      {inst.corporate_asset_confirmed ? (
                        <Badge variant="success" className="gap-1 text-[11px] font-normal">
                          <CheckCircle2 className="size-3" />
                          Confirmado
                        </Badge>
                      ) : (
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 text-xs border-warning/50 text-warning hover:bg-warning/10"
                          onClick={() => {
                            setConfirmChecked(false);
                            setConfirmTarget(inst);
                          }}
                        >
                          Confirmar
                        </Button>
                      )}
                    </TableCell>

                    <TableCell className="text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon" className="size-8">
                            <MoreHorizontal className="size-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          {isOnline ? (
                            <DropdownMenuItem
                              onClick={() =>
                                runAction(
                                  `disconnect-${inst.id}`,
                                  () => api.disconnectInstance(inst.id),
                                  "Aparelho desconectado.",
                                  "Erro ao desconectar"
                                )
                              }
                              className="gap-2 cursor-pointer"
                            >
                              <PowerOff className="size-4 text-warning" />
                              Desconectar
                            </DropdownMenuItem>
                          ) : (
                            <DropdownMenuItem
                              onClick={() =>
                                runAction(
                                  `reconnect-${inst.id}`,
                                  () => api.reconnectInstance(inst.id),
                                  "Comando de reconexão enviado.",
                                  "Erro ao reconectar"
                                )
                              }
                              className="gap-2 cursor-pointer"
                            >
                              <RefreshCw className="size-4 text-primary" />
                              Reconectar
                            </DropdownMenuItem>
                          )}
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            onClick={() => setRemoveTarget(inst)}
                            className="gap-2 text-destructive focus:text-destructive cursor-pointer"
                          >
                            <Trash2 className="size-4" />
                            Remover
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </Card>
      )}

      {/* Pairing Dialog */}
      <Dialog open={pairingOpen} onOpenChange={(open) => !open && closePairing()}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Smartphone className="size-5 text-primary" />
              {pairing ? "Escaneie o QR Code no WhatsApp" : "Conectar Aparelho WhatsApp"}
            </DialogTitle>
            <DialogDescription>
              {pairing
                ? "Abra o WhatsApp no celular: Menu (⋮ ou Ajustes) → Aparelhos Conectados → Conectar Aparelho e aponte a câmera."
                : "Defina o identificador do aparelho, o operador responsável e aceite o termo de conformidade."}
            </DialogDescription>
          </DialogHeader>

          {!pairing ? (
            <form onSubmit={handleStartPairing} className="space-y-4 py-2">
              <div className="grid gap-1.5">
                <Label htmlFor="inst-alias">Nome de Identificação *</Label>
                <Input
                  id="inst-alias"
                  placeholder="Ex: Comercial 01, Suporte VIP, Diretoria"
                  value={alias}
                  onChange={(e) => setAlias(e.target.value)}
                  autoFocus
                />
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="inst-emp">Colaborador Responsável</Label>
                <Select value={employeeId} onValueChange={setEmployeeId}>
                  <SelectTrigger id="inst-emp">
                    <SelectValue placeholder="Selecione um colaborador" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NO_EMPLOYEE}>Sem colaborador vinculado</SelectItem>
                    {employees.map((emp) => (
                      <SelectItem key={emp.id} value={emp.id.toString()}>
                        {emp.name}
                        {emp.department_name ? ` · ${emp.department_name}` : ""}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="flex items-center justify-between gap-3 rounded-lg border p-3 bg-muted/20">
                <div className="space-y-0.5 text-xs">
                  <p className="font-semibold text-foreground">Permitir disparo de mensagens</p>
                  <p className="text-muted-foreground">Desativado, o aparelho operará em modo somente leitura.</p>
                </div>
                <Switch checked={allowSend} onCheckedChange={setAllowSend} aria-label="Permitir envio" />
              </div>

              <label className="flex cursor-pointer items-start gap-2.5 rounded-lg border border-warning/50 bg-warning/10 p-3 text-xs leading-relaxed">
                <input
                  type="checkbox"
                  className="mt-0.5 size-4 shrink-0 rounded accent-primary"
                  checked={confirmed}
                  onChange={(e) => setConfirmed(e.target.checked)}
                />
                <span className="text-muted-foreground">{CORPORATE_TERMS}</span>
              </label>

              <DialogFooter className="pt-2">
                <Button type="button" variant="outline" onClick={closePairing}>
                  Cancelar
                </Button>
                <Button type="submit" disabled={starting || !confirmed}>
                  {starting && <Loader2 className="mr-2 size-4 animate-spin" />}
                  Gerar QR Code
                </Button>
              </DialogFooter>
            </form>
          ) : (
            <div className="flex flex-col items-center space-y-4 py-4 text-center">
              {pairing.state === "expired" ? (
                <div className="space-y-3">
                  <p className="text-sm text-destructive font-medium">O código QR expirou sem ser lido.</p>
                  <Button onClick={openNewPairing} className="gap-2">
                    <RefreshCw className="size-4" />
                    Gerar Novo Código
                  </Button>
                </div>
              ) : pairing.qr ? (
                <>
                  <div className="relative rounded-2xl border-2 border-border bg-white p-4 shadow-lg">
                    <QRCodeSVG value={pairing.qr} size={230} level="M" />
                  </div>
                  <div className="space-y-1">
                    <p className="flex items-center justify-center gap-2 text-xs font-medium text-foreground">
                      <Loader2 className="size-3.5 animate-spin text-primary" />
                      Aguardando conexão no celular...
                    </p>
                    <p className="text-[11px] text-muted-foreground">
                      O código é atualizado periodicamente para sua segurança.
                    </p>
                  </div>
                </>
              ) : (
                <div className="flex h-56 flex-col items-center justify-center gap-3">
                  <Loader2 className="size-8 animate-spin text-primary" />
                  <p className="text-xs text-muted-foreground">Inicializando canal com o WhatsApp...</p>
                </div>
              )}
              <Button variant="outline" className="w-full" onClick={closePairing}>
                Cancelar Pareamento
              </Button>
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* Confirmation of legacy numbers */}
      <Dialog open={confirmTarget !== null} onOpenChange={(open) => !open && setConfirmTarget(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-warning">
              <ShieldCheck className="size-5" />
              Confirmar Ativo Corporativo & LGPD
            </DialogTitle>
            <DialogDescription>
              {confirmTarget?.alias || confirmTarget?.phone_number}
            </DialogDescription>
          </DialogHeader>
          <label className="flex cursor-pointer items-start gap-2.5 rounded-lg border border-warning/50 bg-warning/10 p-3 text-xs leading-relaxed">
            <input
              type="checkbox"
              className="mt-0.5 size-4 shrink-0 rounded accent-primary"
              checked={confirmChecked}
              onChange={(e) => setConfirmChecked(e.target.checked)}
            />
            <span className="text-muted-foreground">{CORPORATE_TERMS}</span>
          </label>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmTarget(null)}>
              Cancelar
            </Button>
            <Button disabled={!confirmChecked} onClick={handleConfirmAsset}>
              Confirmar Ativo
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Removal Alert */}
      <AlertDialog open={removeTarget !== null} onOpenChange={(open) => !open && setRemoveTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className="flex items-center gap-2 text-destructive">
              <Trash2 className="size-5" />
              Remover Aparelho WhatsApp
            </AlertDialogTitle>
            <AlertDialogDescription className="space-y-2">
              <p>
                Deseja realmente desconectar e remover o aparelho <strong>&ldquo;{removeTarget?.alias || removeTarget?.phone_number}&rdquo;</strong>?
              </p>
              <p className="text-xs text-muted-foreground">
                A sessão será encerrada no WhatsApp. Todas as mensagens já capturadas e auditadas serão mantidas no banco de dados com segurança.
              </p>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleRemove}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Desconectar e Remover
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </PageContainer>
  );
}
