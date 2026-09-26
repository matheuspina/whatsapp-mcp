"use client";

import { useEffect, useMemo, useState } from "react";
import { Loader2, MessageSquare, Search } from "lucide-react";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { FeedMessageCard } from "@/components/audit/feed-message";
import { VersionsDialog } from "@/components/audit/versions-dialog";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { WhatsAppAPI, Department, Employee, FeedFilters, FeedMessage, Instance, MessageVersion } from "@/lib/api";

const ALL = "all";
const PAGE_SIZE = 100;
const SEARCH_DEBOUNCE_MS = 350;

function errorText(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback;
}

/** A day picked in a date input, as the start (inclusive) or end (exclusive, next midnight) instant. */
function dayBound(value: string, end: boolean): string | undefined {
  if (!value) return undefined;
  const date = new Date(`${value}T00:00:00`);
  if (end) date.setDate(date.getDate() + 1);
  return date.toISOString();
}

interface VersionsTarget {
  message: FeedMessage;
  versions: MessageVersion[] | null;
}

export default function MessagesPage() {
  const api = useMemo(() => new WhatsAppAPI(), []);

  const [instances, setInstances] = useState<Instance[]>([]);
  const [departments, setDepartments] = useState<Department[]>([]);
  const [employees, setEmployees] = useState<Employee[]>([]);

  const [instance, setInstance] = useState(ALL);
  const [department, setDepartment] = useState(ALL);
  const [employee, setEmployee] = useState(ALL);
  const [text, setText] = useState("");
  const [debouncedText, setDebouncedText] = useState("");
  const [deletedOnly, setDeletedOnly] = useState(false);
  const [since, setSince] = useState("");
  const [until, setUntil] = useState("");

  // The feed is stored with the query that produced it; "loading" is derived, not set.
  const [result, setResult] = useState<{ key: string; messages: FeedMessage[]; exhausted: boolean } | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [versions, setVersions] = useState<VersionsTarget | null>(null);

  useEffect(() => {
    let ignore = false;
    Promise.all([api.getInstances(), api.getDepartments(), api.getEmployees()])
      .then(([inst, dept, emp]) => {
        if (ignore) return;
        setInstances(inst);
        setDepartments(dept);
        setEmployees(emp);
      })
      .catch((err: unknown) => {
        if (!ignore) toast.error("Erro ao carregar filtros", { description: errorText(err, "Falha na comunicação") });
      });
    return () => {
      ignore = true;
    };
  }, [api]);

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedText(text.trim()), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [text]);

  const filters = useMemo<FeedFilters>(
    () => ({
      instance: instance !== ALL ? instance : undefined,
      department_id: department !== ALL ? parseInt(department, 10) : undefined,
      employee_id: employee !== ALL ? parseInt(employee, 10) : undefined,
      q: debouncedText || undefined,
      deleted_only: deletedOnly || undefined,
      since: dayBound(since, false),
      until: dayBound(until, true),
      limit: PAGE_SIZE,
    }),
    [instance, department, employee, debouncedText, deletedOnly, since, until]
  );
  const key = JSON.stringify(filters);

  useEffect(() => {
    let ignore = false;
    api
      .getMessageFeed(filters)
      .then((messages) => {
        if (!ignore) setResult({ key, messages, exhausted: messages.length < PAGE_SIZE });
      })
      .catch((err: unknown) => {
        if (!ignore) {
          toast.error("Erro ao carregar mensagens", { description: errorText(err, "Falha na comunicação") });
          setResult({ key, messages: [], exhausted: true });
        }
      });
    return () => {
      ignore = true;
    };
  }, [api, filters, key]);

  const loading = result === null || result.key !== key;
  const messages = loading ? [] : result.messages;

  const loadMore = async () => {
    if (loading || messages.length === 0) return;
    try {
      setLoadingMore(true);
      const older = await api.getMessageFeed({ ...filters, before: messages[messages.length - 1].timestamp });
      setResult({ key, messages: [...messages, ...older], exhausted: older.length < PAGE_SIZE });
    } catch (err: unknown) {
      toast.error("Erro ao carregar mais mensagens", { description: errorText(err, "Falha na comunicação") });
    } finally {
      setLoadingMore(false);
    }
  };

  const openVersions = async (message: FeedMessage) => {
    setVersions({ message, versions: null });
    try {
      const list = await api.getMessageVersions(message.instance_jid ?? "", message.chat_jid, message.id);
      setVersions({ message, versions: list });
    } catch (err: unknown) {
      setVersions(null);
      toast.error("Erro ao carregar o histórico", { description: errorText(err, "Falha na comunicação") });
    }
  };

  // Picking a department narrows the people in the employee filter.
  const employeeOptions = useMemo(
    () => (department === ALL ? employees : employees.filter((e) => e.department_id?.toString() === department)),
    [employees, department]
  );

  const deletedCount = messages.filter((m) => m.is_deleted_remote).length;

  return (
    <PageContainer>
      <PageHeader
        title="Mensagens"
        description="Tudo o que foi capturado pelos números monitorados, com quem operava cada número na época."
      />

      <Card>
        <CardContent className="grid gap-4 pt-6 sm:grid-cols-2 xl:grid-cols-4">
          <div className="grid gap-1.5">
            <Label className="text-xs text-muted-foreground">Setor</Label>
            <Select
              value={department}
              onValueChange={(v) => {
                setDepartment(v);
                setEmployee(ALL);
              }}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>Todos os setores</SelectItem>
                {departments.map((d) => (
                  <SelectItem key={d.id} value={d.id.toString()}>
                    {d.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="grid gap-1.5">
            <Label className="text-xs text-muted-foreground">Colaborador</Label>
            <Select value={employee} onValueChange={setEmployee}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>Todos os colaboradores</SelectItem>
                {employeeOptions.map((e) => (
                  <SelectItem key={e.id} value={e.id.toString()}>
                    {e.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="grid gap-1.5">
            <Label className="text-xs text-muted-foreground">Número</Label>
            <Select value={instance} onValueChange={setInstance}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>Todos os números</SelectItem>
                {instances
                  .filter((i) => i.phone_jid)
                  .map((i) => (
                    <SelectItem key={i.id} value={i.phone_jid as string}>
                      {i.alias || i.phone_number}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="feed-q" className="text-xs text-muted-foreground">
              Buscar texto
            </Label>
            <div className="relative">
              <Search className="absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
              <Input
                id="feed-q"
                className="pl-8"
                placeholder="Palavra ou frase"
                value={text}
                onChange={(e) => setText(e.target.value)}
              />
            </div>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="feed-since" className="text-xs text-muted-foreground">
              De
            </Label>
            <Input id="feed-since" type="date" value={since} onChange={(e) => setSince(e.target.value)} />
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="feed-until" className="text-xs text-muted-foreground">
              Até
            </Label>
            <Input id="feed-until" type="date" value={until} onChange={(e) => setUntil(e.target.value)} />
          </div>

          <div className="flex items-end sm:col-span-2">
            <label className="flex items-center gap-2.5 text-sm">
              <Switch checked={deletedOnly} onCheckedChange={setDeletedOnly} aria-label="Somente apagadas" />
              Somente mensagens apagadas pelo remetente
            </label>
          </div>
        </CardContent>
      </Card>

      {loading ? (
        <div className="flex h-48 items-center justify-center">
          <Loader2 className="size-8 animate-spin text-muted-foreground" />
        </div>
      ) : messages.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
            <MessageSquare className="size-10 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">Nenhuma mensagem para os filtros escolhidos.</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">
            {messages.length} mensagens carregadas
            {deletedCount > 0 ? `, ${deletedCount} apagadas pelo remetente` : ""}. A mesma mensagem aparece uma vez por
            número que a recebeu.
          </p>

          {messages.map((m) => (
            <FeedMessageCard key={`${m.instance_jid}|${m.chat_jid}|${m.id}`} message={m} onShowVersions={openVersions} />
          ))}

          {!result?.exhausted && (
            <div className="flex justify-center pt-2">
              <Button variant="outline" onClick={loadMore} disabled={loadingMore} className="gap-2">
                {loadingMore && <Loader2 className="size-4 animate-spin" />}
                Carregar mais antigas
              </Button>
            </div>
          )}
        </div>
      )}

      <VersionsDialog open={versions !== null} versions={versions?.versions ?? null} onClose={() => setVersions(null)} />
    </PageContainer>
  );
}
