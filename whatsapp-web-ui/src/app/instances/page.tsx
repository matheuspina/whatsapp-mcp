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
} from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { Notice } from "@/components/common/notice";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import { WhatsAppAPI, Instance, Employee, PairingState } from "@/lib/api";

/** What the person responsible attests before a number is monitored. The bridge records who agreed and which wording. */
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
  return inst.phone_number ? `+${inst.phone_number}` : "Aguardando leitura do QR Code";
}

function statusBadge(inst: Instance): { variant: "success" | "warning" | "destructive" | "outline"; label: string } {
  if (inst.status === "pairing") return { variant: "warning", label: "Pareando" };
  if (inst.status === "logged_out") return { variant: "destructive", label: "Sessão encerrada" };
  if (inst.live) return { variant: "success", label: "Online" };
  return { variant: "outline", label: "Offline" };
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

  // Pairing dialog
  const [pairingOpen, setPairingOpen] = useState(false);
  const [alias, setAlias] = useState("");
  const [employeeId, setEmployeeId] = useState<string>(NO_EMPLOYEE);
  const [allowSend, setAllowSend] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const [starting, setStarting] = useState(false);
  const [pairing, setPairing] = useState<Pairing | null>(null);
  const pairingId = useRef<number | null>(null);

  // Confirmation of a number that was connected before the attestation existed
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

  // While a QR code is on screen: WhatsApp rotates it every ~20 seconds, so keep fetching the current one.
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
            toast.success("Número conectado.");
            setPairingOpen(false);
            setPairing(null);
            loadData();
          }
        })
        .catch(() => {
          /* the next tick tries again */
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

  // Closing the dialog before the QR code is scanned abandons the pairing: release it on the bridge.
  const closePairing = async () => {
    const abandoned = pairingId.current;
    pairingId.current = null;
    setPairingOpen(false);
    setPairing(null);
    if (abandoned !== null) {
      try {
        await api.removeInstance(abandoned);
      } catch {
        /* it expires on its own */
      }
      loadData();
    }
  };

  const handleStartPairing = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!alias.trim()) {
      toast.error("Informe um nome para o número.");
      return;
    }
    if (!confirmed) {
      toast.error("Confirme que o número é um ativo da empresa.");
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
      if (!res.qr_code) toast.info("O QR Code aparece em instantes.");
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
      "Responsável atualizado. As mensagens antigas continuam com quem tinha o número na época.",
      "Erro ao vincular colaborador"
    );

  const handleAllowSend = (inst: Instance, allow: boolean) =>
    runAction(
      `send-${inst.id}`,
      () => api.updateInstance(inst.id, { allow_send: allow }),
      allow ? "Envio habilitado para este número." : "Número agora é somente leitura.",
      "Erro ao alterar permissão de envio"
    );

  const handleConfirmAsset = async () => {
    if (!confirmTarget) return;
    const target = confirmTarget;
    setConfirmTarget(null);
    await runAction(
      `confirm-${target.id}`,
      () => api.updateInstance(target.id, { corporate_asset_confirmed: true }),
      "Ativo corporativo confirmado.",
      "Erro ao confirmar o ativo"
    );
  };

  const handleRemove = async () => {
    if (!removeTarget) return;
    const target = removeTarget;
    setRemoveTarget(null);
    await runAction(
      `remove-${target.id}`,
      () => api.removeInstance(target.id),
      "Número removido. As mensagens já capturadas foram mantidas.",
      "Erro ao remover número"
    );
  };

  const visible = instances.filter((i) => i.status !== "pairing" || i.id === pairing?.id);
  const unconfirmed = visible.filter((i) => !i.corporate_asset_confirmed && i.status !== "pairing");

  return (
    <PageContainer>
      <PageHeader
        title="Números WhatsApp"
        description="Aparelhos conectados que o sistema monitora, quem opera cada um e o que ele pode fazer."
        actions={
          <Button onClick={openNewPairing} className="gap-2">
            <Plus className="size-4" />
            Conectar novo número
          </Button>
        }
      />

      <Notice variant="warning" icon={ShieldCheck} title="Como funciona">
        <p className="text-xs leading-relaxed text-muted-foreground">
          Cada número é vinculado como <strong>Aparelho Conectado</strong>: o colaborador continua usando o celular
          normalmente e o sistema registra as mensagens para conformidade e para consulta por IA. Por padrão o sistema{" "}
          <strong>não envia nada</strong> por esses números; o envio é habilitado número a número. O WhatsApp
          desconecta aparelhos vinculados quando o celular fica cerca de 14 dias sem abrir o aplicativo, e o uso de
          clientes não oficiais pode levar a restrições da conta.
        </p>
      </Notice>

      {unconfirmed.length > 0 && (
        <Notice variant="destructive" icon={ShieldAlert} title="Ativo corporativo pendente">
          <p className="text-xs leading-relaxed text-muted-foreground">
            {unconfirmed.length === 1 ? "Um número" : `${unconfirmed.length} números`} foi conectado sem a confirmação de
            que é um ativo da empresa. Confirme em cada cartão; o servidor pode ser configurado para não registrar
            mensagens de números sem essa confirmação.
          </p>
        </Notice>
      )}

      {loading ? (
        <div className="flex h-48 items-center justify-center">
          <Loader2 className="size-8 animate-spin text-muted-foreground" />
        </div>
      ) : visible.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
            <Smartphone className="size-10 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">
              Nenhum número conectado. Use “Conectar novo número” e escaneie o QR Code pelo aplicativo do celular.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {visible.map((inst) => {
            const badge = statusBadge(inst);
            return (
              <Card key={inst.id} className="flex flex-col">
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <CardTitle className="truncate text-base font-semibold">
                          {inst.alias || "Número WhatsApp"}
                        </CardTitle>
                        <Badge variant={badge.variant}>{badge.label}</Badge>
                      </div>
                      <p className="mt-1 font-mono text-xs text-muted-foreground">{formatPhone(inst)}</p>
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
                      aria-label="Remover número"
                      onClick={() => setRemoveTarget(inst)}
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  </div>
                </CardHeader>

                <CardContent className="flex flex-1 flex-col gap-4 pt-0">
                  <div className="grid gap-1.5">
                    <Label className="text-xs text-muted-foreground">Colaborador responsável</Label>
                    <Select
                      value={inst.employee_id ? inst.employee_id.toString() : NO_EMPLOYEE}
                      onValueChange={(v) => handleEmployeeChange(inst, v)}
                      disabled={inst.status === "pairing" || actionLoading === `employee-${inst.id}`}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Sem colaborador" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={NO_EMPLOYEE}>Sem colaborador</SelectItem>
                        {employees.map((emp) => (
                          <SelectItem key={emp.id} value={emp.id.toString()}>
                            {emp.name}
                            {emp.department_name ? ` · ${emp.department_name}` : ""}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    {inst.department_name && (
                      <span className="flex items-center gap-1 text-xs text-muted-foreground">
                        <User className="size-3" />
                        Setor {inst.department_name}
                      </span>
                    )}
                  </div>

                  <div className="flex items-center justify-between gap-3 rounded-md border p-2.5">
                    <div className="flex items-center gap-2 text-xs">
                      {inst.allow_send ? (
                        <Send className="size-3.5 text-warning" />
                      ) : (
                        <Eye className="size-3.5 text-muted-foreground" />
                      )}
                      <div>
                        <p className="font-medium">{inst.allow_send ? "Envio habilitado" : "Somente leitura"}</p>
                        <p className="text-muted-foreground">
                          {inst.allow_send ? "A IA e a API podem enviar por este número." : "Nada é enviado por este número."}
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

                  {inst.status !== "pairing" &&
                    (inst.corporate_asset_confirmed ? (
                      <Badge variant="success" className="gap-1 self-start" title={`Termo ${inst.corporate_terms_version ?? ""}`}>
                        <ShieldCheck className="size-3" />
                        Ativo corporativo confirmado
                        {inst.corporate_confirmed_by ? ` por ${inst.corporate_confirmed_by}` : ""}
                      </Badge>
                    ) : (
                      <Button
                        variant="outline"
                        size="sm"
                        className="gap-1.5 self-start text-xs"
                        onClick={() => {
                          setConfirmChecked(false);
                          setConfirmTarget(inst);
                        }}
                      >
                        <ShieldAlert className="size-3.5 text-warning" />
                        Confirmar ativo corporativo
                      </Button>
                    ))}

                  <div className="mt-auto flex items-center justify-between border-t pt-3">
                    <span className="text-[11px] text-muted-foreground">
                      {inst.last_seen_at
                        ? `Visto: ${new Date(inst.last_seen_at).toLocaleString("pt-BR")}`
                        : "Nunca conectado"}
                    </span>
                    {inst.status !== "pairing" &&
                      (inst.live ? (
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-8 gap-1.5 text-xs text-muted-foreground hover:text-destructive"
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
                          {actionLoading === `disconnect-${inst.id}` ? (
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
                          {actionLoading === `reconnect-${inst.id}` ? (
                            <Loader2 className="size-3.5 animate-spin" />
                          ) : (
                            <RefreshCw className="size-3.5" />
                          )}
                          Reconectar
                        </Button>
                      ))}
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {/* Pairing */}
      <Dialog open={pairingOpen} onOpenChange={(open) => !open && closePairing()}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Conectar número WhatsApp</DialogTitle>
            <DialogDescription>
              {pairing
                ? "No celular: WhatsApp → Aparelhos conectados → Conectar um aparelho, e aponte a câmera para o código."
                : "Identifique o número, o responsável e confirme que ele é da empresa."}
            </DialogDescription>
          </DialogHeader>

          {!pairing ? (
            <form onSubmit={handleStartPairing} className="space-y-4 py-2">
              <div className="grid gap-2">
                <Label htmlFor="inst-alias">Nome do número *</Label>
                <Input
                  id="inst-alias"
                  placeholder="Ex: Celular Vendas 01"
                  value={alias}
                  onChange={(e) => setAlias(e.target.value)}
                  autoFocus
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="inst-emp">Colaborador responsável</Label>
                <Select value={employeeId} onValueChange={setEmployeeId}>
                  <SelectTrigger id="inst-emp">
                    <SelectValue placeholder="Selecione um colaborador" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NO_EMPLOYEE}>Sem colaborador</SelectItem>
                    {employees.map((emp) => (
                      <SelectItem key={emp.id} value={emp.id.toString()}>
                        {emp.name}
                        {emp.department_name ? ` · ${emp.department_name}` : ""}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="flex items-center justify-between gap-3 rounded-md border p-3">
                <div className="text-xs">
                  <p className="font-medium">Permitir envio por este número</p>
                  <p className="text-muted-foreground">Desligado, o número só é lido.</p>
                </div>
                <Switch checked={allowSend} onCheckedChange={setAllowSend} aria-label="Permitir envio" />
              </div>

              <label className="flex cursor-pointer items-start gap-2.5 rounded-md border border-warning/50 bg-warning/10 p-3 text-xs leading-relaxed">
                <input
                  type="checkbox"
                  className="mt-0.5 size-4 shrink-0 accent-primary"
                  checked={confirmed}
                  onChange={(e) => setConfirmed(e.target.checked)}
                />
                <span>{CORPORATE_TERMS}</span>
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
                  <p className="text-sm text-muted-foreground">O código expirou sem ser lido.</p>
                  <Button onClick={openNewPairing}>Tentar novamente</Button>
                </div>
              ) : pairing.qr ? (
                <>
                  <div className="rounded-xl border bg-white p-4 shadow-sm">
                    <QRCodeSVG value={pairing.qr} size={230} level="M" />
                  </div>
                  <p className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Loader2 className="size-3.5 animate-spin" />
                    Aguardando a leitura. O código é renovado automaticamente.
                  </p>
                </>
              ) : (
                <div className="flex h-56 items-center justify-center">
                  <Loader2 className="size-8 animate-spin text-muted-foreground" />
                </div>
              )}
              <Button variant="outline" className="w-full" onClick={closePairing}>
                Cancelar
              </Button>
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* Attestation for numbers connected before it existed */}
      <Dialog open={confirmTarget !== null} onOpenChange={(open) => !open && setConfirmTarget(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Confirmar ativo corporativo</DialogTitle>
            <DialogDescription>{confirmTarget?.alias || confirmTarget?.phone_number}</DialogDescription>
          </DialogHeader>
          <label className="flex cursor-pointer items-start gap-2.5 rounded-md border border-warning/50 bg-warning/10 p-3 text-xs leading-relaxed">
            <input
              type="checkbox"
              className="mt-0.5 size-4 shrink-0 accent-primary"
              checked={confirmChecked}
              onChange={(e) => setConfirmChecked(e.target.checked)}
            />
            <span>{CORPORATE_TERMS}</span>
          </label>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmTarget(null)}>
              Cancelar
            </Button>
            <Button disabled={!confirmChecked} onClick={handleConfirmAsset}>
              Confirmar
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Removal */}
      <AlertDialog open={removeTarget !== null} onOpenChange={(open) => !open && setRemoveTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remover número</AlertDialogTitle>
            <AlertDialogDescription>
              O aparelho é desconectado e a sessão é apagada. As mensagens já capturadas são mantidas e continuam
              atribuídas a este número. Para reconectar, será preciso escanear um novo QR Code.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleRemove}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Desconectar e remover
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </PageContainer>
  );
}
