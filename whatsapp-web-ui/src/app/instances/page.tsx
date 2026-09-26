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
} from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { StatCard, StatGrid } from "@/components/common/stat-card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardDescription, CardTitle } from "@/components/ui/card";
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
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
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
        description="Conecte e acompanhe os números de WhatsApp usados pela sua equipe."
        actions={
          <>
            <Popover>
              <PopoverTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-9 text-muted-foreground hover:text-foreground"
                  aria-label="Como funciona o monitoramento de aparelhos WhatsApp"
                  title="Como funciona o monitoramento"
                >
                  <Info className="size-4" />
                </Button>
              </PopoverTrigger>
              <PopoverContent align="end" className="w-[min(22rem,calc(100vw-2rem))] p-3">
                <p className="mb-2 text-xs font-semibold text-foreground">Sobre os aparelhos</p>
                <div className="space-y-2 text-xs leading-relaxed text-muted-foreground">
                  <p><strong className="text-foreground">Conectado:</strong> as mensagens ficam disponíveis para consulta e acompanhamento.</p>
                  <p><strong className="text-foreground">Somente leitura:</strong> o envio de mensagens fica bloqueado até ser liberado.</p>
                </div>
              </PopoverContent>
            </Popover>
            <Button onClick={openNewPairing} className="gap-2 shadow-sm">
              <Plus className="size-4" />
              Conectar Novo Aparelho
            </Button>
          </>
        }
      />

      <StatGrid columns={4}>
        <StatCard
          icon={<Smartphone />}
          label="Aparelhos"
          value={loading ? "..." : visible.length}
        />
        <StatCard
          icon={<span className="size-2 rounded-full bg-emerald-500" />}
          label="Online"
          value={loading ? "..." : onlineCount}
          valueClassName="text-emerald-600 dark:text-emerald-400"
        />
        <StatCard
          icon={<Send />}
          label="Envio liberado"
          value={loading ? "..." : sendAllowedCount}
        />
        <StatCard
          icon={<ShieldCheck />}
          label="LGPD confirmado"
          value={loading ? "..." : `${visible.length - unconfirmed.length}/${visible.length}`}
        />
      </StatGrid>

      {/* Pending attestation alert */}
      {unconfirmed.length > 0 && (
        <div className="rounded-lg border border-warning/40 bg-warning/5 px-3 py-2.5">
          <div className="flex flex-col items-start justify-between gap-2 sm:flex-row sm:items-center">
            <div className="flex items-center gap-3">
              <ShieldAlert className="size-4 shrink-0 text-warning" />
              <div>
                <p className="text-xs font-medium text-foreground">
                  {unconfirmed.length === 1
                    ? "1 número com termo corporativo pendente"
                    : `${unconfirmed.length} números com termo corporativo pendente`}
                </p>
                <p className="text-[11px] text-muted-foreground">
                  Confirme o termo corporativo para concluir o cadastro desses números.
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
        </div>
      )}

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
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {filteredInstances.map((inst) => {
            const isOnline = inst.live;
            const isActionLoading = actionLoading?.includes(`-${inst.id}`);
            const statusLabel = inst.status === "pairing" ? "Pareando" : isOnline ? "Online" : "Offline";

            return (
              <Card
                key={inst.id}
                className="group border-border/80 py-0 shadow-none transition-colors hover:border-primary/40"
              >
                <CardContent className="p-3.5">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-3 min-w-0">
                      <div
                        className={`relative flex size-9 shrink-0 items-center justify-center rounded-md ${
                          isOnline
                            ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                            : "bg-muted text-muted-foreground"
                        }`}
                        title={statusLabel}
                      >
                        <Smartphone className="size-4" />
                        <span
                          className={`absolute -bottom-0.5 -right-0.5 size-2.5 rounded-full border-2 border-card ${
                            inst.status === "pairing"
                              ? "bg-amber-500"
                              : isOnline
                                ? "bg-emerald-500"
                                : "bg-muted-foreground/50"
                          }`}
                          aria-label={statusLabel}
                        />
                      </div>
                      <div className="min-w-0">
                        <div className="flex min-w-0 items-center gap-2">
                          <CardTitle className="truncate text-sm font-semibold" title={inst.alias}>
                            {inst.alias || "Número WhatsApp"}
                          </CardTitle>
                        </div>
                        <p className="mt-0.5 font-mono text-[11px] text-muted-foreground">
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

                  {/* Assignment stays inline with the rest of the device metadata. */}
                  <div className="mt-3 flex items-center gap-2 border-t border-border/60 pt-3">
                    <User className="size-3.5 shrink-0 text-muted-foreground" />
                    <span className="shrink-0 text-[11px] text-muted-foreground">Responsável</span>
                    <Select
                      value={inst.employee_id ? inst.employee_id.toString() : NO_EMPLOYEE}
                      onValueChange={(v) => handleEmployeeChange(inst, v)}
                      disabled={inst.status === "pairing" || actionLoading === `employee-${inst.id}`}
                    >
                      <SelectTrigger className="ml-auto h-7 w-full max-w-[12rem] border-0 bg-muted/40 px-2 text-xs shadow-none hover:bg-muted/70 focus-visible:ring-1">
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

                  <div className="mt-2 flex min-h-7 items-center justify-between gap-3 text-[11px]">
                    <label className="flex items-center gap-1.5 text-muted-foreground">
                      {inst.allow_send ? <Send className="size-3.5 text-amber-500" /> : <Eye className="size-3.5" />}
                      <span>{inst.allow_send ? "Envio permitido" : "Somente leitura"}</span>
                    </label>
                    <Switch
                      checked={inst.allow_send}
                      className="scale-90"
                      disabled={inst.status === "pairing" || actionLoading === `send-${inst.id}`}
                      onCheckedChange={(v) => handleAllowSend(inst, v)}
                      aria-label="Permitir envio"
                    />
                  </div>

                  <div className="mt-2 flex items-center justify-between gap-2 text-[11px]">
                    {inst.corporate_asset_confirmed ? (
                      <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
                        <CheckCircle2 className="size-3.5" />
                        LGPD confirmado
                      </span>
                    ) : (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-6 px-1.5 text-[11px] text-warning hover:bg-warning/10 hover:text-warning"
                        onClick={() => {
                          setConfirmChecked(false);
                          setConfirmTarget(inst);
                        }}
                      >
                        <ShieldAlert className="size-3.5" />
                        Confirmar LGPD
                      </Button>
                    )}
                    <div className="ml-auto flex min-w-0 items-center gap-2">
                      <span className="hidden truncate text-[10px] text-muted-foreground sm:inline">
                        {inst.last_seen_at
                          ? `Visto ${new Date(inst.last_seen_at).toLocaleTimeString("pt-BR", {
                              hour: "2-digit",
                              minute: "2-digit",
                            })}`
                          : "Nunca conectado"}
                      </span>
                      {isOnline ? (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-6 px-1.5 text-[11px] text-muted-foreground hover:text-destructive"
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
                          className="h-6 px-1.5 text-[11px] text-primary hover:text-primary"
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
                const statusLabel = inst.status === "pairing" ? "Pareando" : isOnline ? "Online" : "Offline";
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
                      <span
                        className={`inline-block size-2 rounded-full ${
                          inst.status === "pairing"
                            ? "bg-amber-500"
                            : isOnline
                              ? "bg-emerald-500"
                              : "bg-muted-foreground/50"
                        }`}
                        title={statusLabel}
                        aria-label={statusLabel}
                      />
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
                      <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
                        {inst.allow_send ? (
                          <Send className="size-3.5 text-amber-500" />
                        ) : (
                          <Eye className="size-3.5" />
                        )}
                        {inst.allow_send ? "Envio ativo" : "Leitura"}
                      </span>
                    </TableCell>

                    <TableCell>
                      {inst.corporate_asset_confirmed ? (
                        <span className="inline-flex items-center gap-1.5 text-xs text-emerald-600 dark:text-emerald-400">
                          <CheckCircle2 className="size-3.5" />
                          Confirmado
                        </span>
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
                A conexão será encerrada no WhatsApp. As mensagens já capturadas continuarão disponíveis para consulta.
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
