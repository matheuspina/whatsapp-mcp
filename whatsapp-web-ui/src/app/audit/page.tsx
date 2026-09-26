"use client";

import { useEffect, useMemo, useState } from "react";
import { Eraser, Loader2, ScrollText, ShieldAlert, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { Notice } from "@/components/common/notice";
import { FeedMessageCard, whenLabel } from "@/components/audit/feed-message";
import { VersionsDialog } from "@/components/audit/versions-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
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
import { WhatsAppAPI, AccessLogEntry, FeedMessage, MessageVersion, PrivacyLogEntry } from "@/lib/api";

function errorText(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback;
}

function Loading() {
  return (
    <div className="flex h-32 items-center justify-center">
      <Loader2 className="size-6 animate-spin text-muted-foreground" />
    </div>
  );
}

/** Messages their sender revoked, with the text that was revoked kept as a version. */
function DeletedTab({ api }: { api: WhatsAppAPI }) {
  const [messages, setMessages] = useState<FeedMessage[] | null>(null);
  const [versions, setVersions] = useState<{ open: boolean; list: MessageVersion[] | null }>({ open: false, list: null });

  useEffect(() => {
    let ignore = false;
    api
      .getMessageFeed({ deleted_only: true, limit: 200 })
      .then((list) => {
        if (!ignore) setMessages(list);
      })
      .catch((err: unknown) => {
        if (!ignore) {
          toast.error("Erro ao carregar mensagens apagadas", { description: errorText(err, "Falha na comunicação") });
          setMessages([]);
        }
      });
    return () => {
      ignore = true;
    };
  }, [api]);

  const show = async (m: FeedMessage) => {
    setVersions({ open: true, list: null });
    try {
      setVersions({ open: true, list: await api.getMessageVersions(m.instance_jid ?? "", m.chat_jid, m.id) });
    } catch (err: unknown) {
      setVersions({ open: false, list: null });
      toast.error("Erro ao carregar o histórico", { description: errorText(err, "Falha na comunicação") });
    }
  };

  if (messages === null) return <Loading />;
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">
        Mensagens que o remetente apagou para todos. O sistema guarda o texto original. Só aparecem apagamentos
        recebidos enquanto o número estava conectado.
      </p>
      {messages.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">Nenhuma mensagem apagada.</CardContent>
        </Card>
      ) : (
        messages.map((m) => (
          <FeedMessageCard key={`${m.instance_jid}|${m.chat_jid}|${m.id}`} message={m} onShowVersions={show} />
        ))
      )}
      <VersionsDialog
        open={versions.open}
        versions={versions.list}
        onClose={() => setVersions({ open: false, list: null })}
      />
    </div>
  );
}

/** Who read message data, through the AI server or this panel. */
function AccessTab({ api }: { api: WhatsAppAPI }) {
  const [entries, setEntries] = useState<AccessLogEntry[] | null>(null);

  useEffect(() => {
    let ignore = false;
    api
      .getAccessLog(200)
      .then((list) => {
        if (!ignore) setEntries(list);
      })
      .catch((err: unknown) => {
        if (!ignore) {
          toast.error("Erro ao carregar o log de acessos", { description: errorText(err, "Falha na comunicação") });
          setEntries([]);
        }
      });
    return () => {
      ignore = true;
    };
  }, [api]);

  if (entries === null) return <Loading />;
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">
        Cada consulta a mensagens feita por um cliente de IA (ferramentas do MCP) ou por esta tela fica registrada.
        Ferramentas que enviam mensagens registram só os nomes dos parâmetros, não o conteúdo.
      </p>
      {entries.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">Nenhum acesso registrado.</CardContent>
        </Card>
      ) : (
        <Card>
          <CardContent className="divide-y p-0">
            {entries.map((e) => (
              <div key={e.id} className="grid gap-1 px-4 py-2.5 text-xs sm:grid-cols-[11rem_1fr_auto] sm:items-center">
                <span className="text-muted-foreground">{whenLabel(e.ts)}</span>
                <div className="min-w-0">
                  <span className="font-medium">{e.action}</span>
                  <span className="text-muted-foreground"> · {e.actor || "desconhecido"}</span>
                  {e.params && <p className="truncate font-mono text-[11px] text-muted-foreground">{e.params}</p>}
                </div>
                {typeof e.result_count === "number" && <Badge variant="outline">{e.result_count} resultados</Badge>}
              </div>
            ))}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

type Confirmation = { kind: "anonymize"; subject: string } | { kind: "purge"; days: number } | null;

/** LGPD: remove one person's data on request, apply a retention window, and keep a record of both. */
function PrivacyTab({ api }: { api: WhatsAppAPI }) {
  const [subject, setSubject] = useState("");
  const [days, setDays] = useState("");
  const [confirmation, setConfirmation] = useState<Confirmation>(null);
  const [busy, setBusy] = useState(false);
  const [log, setLog] = useState<PrivacyLogEntry[] | null>(null);
  const [refresh, setRefresh] = useState(0);

  useEffect(() => {
    let ignore = false;
    api
      .getPrivacyLog(50)
      .then((list) => {
        if (!ignore) setLog(list);
      })
      .catch((err: unknown) => {
        if (!ignore) {
          toast.error("Erro ao carregar o registro de privacidade", { description: errorText(err, "Falha na comunicação") });
          setLog([]);
        }
      });
    return () => {
      ignore = true;
    };
  }, [api, refresh]);

  const daysNumber = parseInt(days, 10);
  const daysValid = Number.isInteger(daysNumber) && daysNumber >= 1;

  const run = async () => {
    if (!confirmation) return;
    const action = confirmation;
    setConfirmation(null);
    try {
      setBusy(true);
      if (action.kind === "anonymize") {
        const res = await api.anonymizeSubject(action.subject);
        toast.success(`Titular anonimizado: ${res.messages} mensagens, ${res.chats} conversa(s), ${res.media_removed} arquivo(s) removido(s).`);
        setSubject("");
      } else {
        const res = await api.purgeOlderThan(action.days);
        toast.success(`${res.removed} mensagens com mais de ${action.days} dias foram apagadas.`);
        setDays("");
      }
      setRefresh((n) => n + 1);
    } catch (err: unknown) {
      toast.error("Não foi possível concluir a operação", { description: errorText(err, "Falha na comunicação") });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      <Notice variant="warning" icon={ShieldAlert} title="Operações irreversíveis">
        <p className="text-xs leading-relaxed text-muted-foreground">
          Anonimizar e apagar por prazo removem dados do banco e do índice de busca, e não podem ser desfeitos. O
          backup do banco (<code>store/</code>) e cópias fora do sistema não são alcançados: trate-os pela política de
          retenção de backups da empresa. Ao anonimizar, a conversa direta do titular passa a usar um identificador
          fictício e suas mensagens em grupos perdem texto e nome.
        </p>
      </Notice>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Eraser className="size-4" />
              Anonimizar um titular
            </CardTitle>
            <CardDescription>Atende a um pedido do titular (LGPD, art. 18).</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid gap-1.5">
              <Label htmlFor="privacy-subject">Telefone ou JID</Label>
              <Input
                id="privacy-subject"
                placeholder="5511999998888"
                value={subject}
                onChange={(e) => setSubject(e.target.value)}
              />
            </div>
            <Button
              variant="destructive"
              disabled={busy || !subject.trim()}
              onClick={() => setConfirmation({ kind: "anonymize", subject: subject.trim() })}
            >
              Anonimizar
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Trash2 className="size-4" />
              Apagar mensagens antigas
            </CardTitle>
            <CardDescription>
              Retenção por prazo. Para rodar todo dia sem intervenção, defina <code>RETENTION_DAYS</code> na bridge.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid gap-1.5">
              <Label htmlFor="privacy-days">Apagar mensagens com mais de (dias)</Label>
              <Input
                id="privacy-days"
                type="number"
                min={1}
                placeholder="365"
                value={days}
                onChange={(e) => setDays(e.target.value)}
              />
            </div>
            <Button
              variant="destructive"
              disabled={busy || !daysValid}
              onClick={() => setConfirmation({ kind: "purge", days: daysNumber })}
            >
              Apagar
            </Button>
          </CardContent>
        </Card>
      </div>

      <div className="space-y-2">
        <h2 className="flex items-center gap-2 text-sm font-medium">
          <ScrollText className="size-4" />
          Registro de operações
        </h2>
        {log === null ? (
          <Loading />
        ) : log.length === 0 ? (
          <Card>
            <CardContent className="py-8 text-center text-sm text-muted-foreground">Nenhuma operação registrada.</CardContent>
          </Card>
        ) : (
          <Card>
            <CardContent className="divide-y p-0">
              {log.map((e) => (
                <div key={e.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 py-2.5 text-xs">
                  <span className="text-muted-foreground">{whenLabel(e.ts)}</span>
                  <Badge variant={e.action === "anonymize" ? "warning" : "destructive"}>
                    {e.action === "anonymize" ? "Anonimização" : "Retenção"}
                  </Badge>
                  <span>{e.subject}</span>
                  <span className="text-muted-foreground">por {e.actor || "desconhecido"}</span>
                </div>
              ))}
            </CardContent>
          </Card>
        )}
      </div>

      <AlertDialog open={confirmation !== null} onOpenChange={(open) => !open && setConfirmation(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmation?.kind === "anonymize" ? "Anonimizar este titular?" : "Apagar mensagens antigas?"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmation?.kind === "anonymize"
                ? `Todas as mensagens de ${confirmation.subject} perdem texto, nome e mídia. Isso não pode ser desfeito.`
                : confirmation?.kind === "purge"
                  ? `Todas as mensagens com mais de ${confirmation.days} dias serão apagadas de todos os números. Isso não pode ser desfeito.`
                  : ""}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction onClick={run} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
              Confirmar
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

export default function AuditPage() {
  const api = useMemo(() => new WhatsAppAPI(), []);

  return (
    <PageContainer>
      <PageHeader
        title="Auditoria"
        description="Mensagens apagadas, quem consultou os dados e o atendimento a pedidos de privacidade."
      />

      <Notice variant="success" icon={ScrollText} title="Busca por significado">
        <p className="text-xs leading-relaxed text-muted-foreground">
          A busca por palavras está em <strong>Mensagens</strong>. A busca semântica (por significado, filtrada por
          setor ou colaborador) é feita pelos clientes de IA conectados ao servidor MCP, com{" "}
          <code>search_messages</code> e <code>search_department_conversations</code>.
        </p>
      </Notice>

      <Tabs defaultValue="deleted">
        <TabsList>
          <TabsTrigger value="deleted">Apagadas</TabsTrigger>
          <TabsTrigger value="access">Acessos</TabsTrigger>
          <TabsTrigger value="privacy">Privacidade (LGPD)</TabsTrigger>
        </TabsList>
        <TabsContent value="deleted">
          <DeletedTab api={api} />
        </TabsContent>
        <TabsContent value="access">
          <AccessTab api={api} />
        </TabsContent>
        <TabsContent value="privacy">
          <PrivacyTab api={api} />
        </TabsContent>
      </Tabs>
    </PageContainer>
  );
}
