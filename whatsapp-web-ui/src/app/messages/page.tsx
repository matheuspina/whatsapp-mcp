"use client";

import { useEffect, useMemo, useState } from "react";
import {
  Building2,
  Calendar as CalendarIcon,
  Filter,
  History,
  Loader2,
  MessageSquare,
  RotateCcw,
  Search,
  SlidersHorizontal,
  Smartphone,
  Trash2,
  User,
  X,
} from "lucide-react";
import { format, subDays, startOfDay, endOfDay } from "date-fns";
import { ptBR } from "date-fns/locale";
import type { DateRange } from "react-day-picker";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { FeedMessageCard } from "@/components/audit/feed-message";
import { VersionsDialog } from "@/components/audit/versions-dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardDescription, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Calendar } from "@/components/ui/calendar";
import { WhatsAppAPI, Department, Employee, FeedFilters, FeedMessage, Instance, MessageVersion } from "@/lib/api";

const ALL = "all";
const PAGE_SIZE = 100;
const SEARCH_DEBOUNCE_MS = 350;

function errorText(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback;
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

  // 1. Basic filters (available directly in the bar)
  const [text, setText] = useState("");
  const [debouncedText, setDebouncedText] = useState("");
  const [employee, setEmployee] = useState(ALL);

  // 2. Calendar Popover filter
  const [dateRange, setDateRange] = useState<DateRange | undefined>(undefined);
  const [datePopoverOpen, setDatePopoverOpen] = useState(false);

  // 3. Other filters Popover (Setor, Número, Apagadas)
  const [instance, setInstance] = useState(ALL);
  const [department, setDepartment] = useState(ALL);
  const [deletedOnly, setDeletedOnly] = useState(false);
  const [moreFiltersOpen, setMoreFiltersOpen] = useState(false);

  // Deriving result and loading
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
        if (!ignore) toast.error("Erro ao carregar opções de filtro", { description: errorText(err, "Falha na comunicação") });
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
      since: dateRange?.from ? startOfDay(dateRange.from).toISOString() : undefined,
      until: dateRange?.to
        ? endOfDay(dateRange.to).toISOString()
        : dateRange?.from
        ? endOfDay(dateRange.from).toISOString()
        : undefined,
      limit: PAGE_SIZE,
    }),
    [instance, department, employee, debouncedText, deletedOnly, dateRange]
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
      toast.error("Erro ao carregar o histórico de versões", { description: errorText(err, "Falha na comunicação") });
    }
  };

  const employeeOptions = useMemo(
    () => (department === ALL ? employees : employees.filter((e) => e.department_id?.toString() === department)),
    [employees, department]
  );

  // Other filters count (in the Mais Filtros popover)
  const moreFiltersCount = useMemo(() => {
    let count = 0;
    if (department !== ALL) count++;
    if (instance !== ALL) count++;
    if (deletedOnly) count++;
    return count;
  }, [department, instance, deletedOnly]);

  // Overall active filters check
  const hasActiveFilters = useMemo(() => {
    return (
      instance !== ALL ||
      department !== ALL ||
      employee !== ALL ||
      text.trim() !== "" ||
      deletedOnly ||
      dateRange?.from !== undefined
    );
  }, [instance, department, employee, text, deletedOnly, dateRange]);

  const resetFilters = () => {
    setInstance(ALL);
    setDepartment(ALL);
    setEmployee(ALL);
    setText("");
    setDebouncedText("");
    setDeletedOnly(false);
    setDateRange(undefined);
  };

  const resetMoreFilters = () => {
    setInstance(ALL);
    setDepartment(ALL);
    setDeletedOnly(false);
  };

  // Quick date presets
  const applyDatePreset = (preset: "today" | "7d" | "30d") => {
    const today = new Date();
    if (preset === "today") {
      setDateRange({ from: startOfDay(today), to: endOfDay(today) });
      return;
    }
    if (preset === "7d") {
      setDateRange({ from: startOfDay(subDays(today, 7)), to: endOfDay(today) });
      return;
    }
    if (preset === "30d") {
      setDateRange({ from: startOfDay(subDays(today, 30)), to: endOfDay(today) });
      return;
    }
  };

  // Label for Date Picker button
  const dateButtonLabel = useMemo(() => {
    if (!dateRange?.from) return "Filtrar por data";
    if (!dateRange.to || dateRange.from.getTime() === dateRange.to.getTime()) {
      return format(dateRange.from, "dd/MM/yyyy", { locale: ptBR });
    }
    return `${format(dateRange.from, "dd/MM/yy", { locale: ptBR })} - ${format(dateRange.to, "dd/MM/yy", { locale: ptBR })}`;
  }, [dateRange]);

  const deletedCount = messages.filter((m) => m.is_deleted_remote).length;
  const editedCount = messages.filter((m) => m.is_edited).length;

  return (
    <PageContainer>
      <PageHeader
        title="Auditoria de Mensagens"
        description="Histórico unificado de conversas capturadas pelos números monitorados com rastreabilidade de operadores."
      />

      {/* KPI Stats */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Card className="p-4 shadow-sm border-border/70">
          <div className="flex items-center justify-between text-muted-foreground">
            <span className="text-xs font-medium uppercase tracking-wider">Carregadas</span>
            <MessageSquare className="size-4" />
          </div>
          <p className="mt-2 text-2xl font-bold tracking-tight text-foreground">
            {loading ? "..." : messages.length}
          </p>
          <span className="text-[11px] text-muted-foreground">no intervalo selecionado</span>
        </Card>

        <Card className="p-4 shadow-sm border-border/70">
          <div className="flex items-center justify-between text-muted-foreground">
            <span className="text-xs font-medium uppercase tracking-wider">Apagadas</span>
            <Trash2 className="size-4 text-destructive" />
          </div>
          <p className="mt-2 text-2xl font-bold tracking-tight text-destructive">
            {loading ? "..." : deletedCount}
          </p>
          <span className="text-[11px] text-muted-foreground">removidas pelo remetente</span>
        </Card>

        <Card className="p-4 shadow-sm border-border/70">
          <div className="flex items-center justify-between text-muted-foreground">
            <span className="text-xs font-medium uppercase tracking-wider">Editadas</span>
            <History className="size-4 text-warning" />
          </div>
          <p className="mt-2 text-2xl font-bold tracking-tight text-foreground">
            {loading ? "..." : editedCount}
          </p>
          <span className="text-[11px] text-muted-foreground">com versões no histórico</span>
        </Card>

        <Card className="p-4 shadow-sm border-border/70">
          <div className="flex items-center justify-between text-muted-foreground">
            <span className="text-xs font-medium uppercase tracking-wider">Aparelhos</span>
            <Smartphone className="size-4 text-primary" />
          </div>
          <p className="mt-2 text-2xl font-bold tracking-tight text-foreground">
            {instances.filter((i) => i.live).length}/{instances.length}
          </p>
          <span className="text-[11px] text-muted-foreground">instâncias ativas</span>
        </Card>
      </div>

      {/* Filter Card: Basic Filters directly visible + Calendar Popover + More Filters Popover */}
      <Card className="border-border/80 shadow-sm p-4">
        <div className="flex flex-col gap-3">
          <div className="flex flex-col md:flex-row items-center gap-2.5">
            {/* 1. Basic Filter: Text search */}
            <div className="relative flex-1 w-full">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id="feed-q"
                className="pl-9 pr-9"
                placeholder="Buscar por texto, palavra ou número..."
                value={text}
                onChange={(e) => setText(e.target.value)}
              />
              {text && (
                <button
                  type="button"
                  onClick={() => setText("")}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  <X className="size-4" />
                </button>
              )}
            </div>

            {/* 2. Basic Filter: Employee select */}
            <div className="w-full md:w-56 shrink-0">
              <Select value={employee} onValueChange={setEmployee}>
                <SelectTrigger className="w-full">
                  <User className="size-3.5 mr-2 text-muted-foreground shrink-0" />
                  <SelectValue placeholder="Todos os colaboradores" />
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

            {/* 3. Date Picker Popover (Shadcn Calendar in Popover) */}
            <div className="w-full md:w-auto shrink-0">
              <Popover open={datePopoverOpen} onOpenChange={setDatePopoverOpen}>
                <PopoverTrigger asChild>
                  <Button
                    variant={dateRange?.from ? "secondary" : "outline"}
                    className="h-9 w-full md:w-auto min-w-[190px] justify-start text-left font-normal text-xs gap-2 shadow-sm"
                  >
                    <CalendarIcon className="size-3.5 text-muted-foreground shrink-0" />
                    <span className="truncate">{dateButtonLabel}</span>
                    {dateRange?.from && (
                      <span
                        role="button"
                        tabIndex={0}
                        onClick={(e) => {
                          e.stopPropagation();
                          setDateRange(undefined);
                        }}
                        className="ml-auto hover:text-destructive rounded-full p-0.5"
                        title="Limpar data"
                      >
                        <X className="size-3" />
                      </span>
                    )}
                  </Button>
                </PopoverTrigger>

                <PopoverContent className="w-auto p-3 space-y-3" align="start">
                  <div className="flex items-center justify-between border-b pb-2 gap-2">
                    <span className="text-xs font-semibold text-foreground flex items-center gap-1.5">
                      <CalendarIcon className="size-3.5 text-primary" />
                      Período
                    </span>
                    <div className="flex items-center gap-1">
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-6 px-1.5 text-[10px]"
                        onClick={() => applyDatePreset("today")}
                      >
                        Hoje
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-6 px-1.5 text-[10px]"
                        onClick={() => applyDatePreset("7d")}
                      >
                        7 dias
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-6 px-1.5 text-[10px]"
                        onClick={() => applyDatePreset("30d")}
                      >
                        30 dias
                      </Button>
                      {dateRange?.from && (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-6 px-1.5 text-[10px] text-destructive hover:text-destructive"
                          onClick={() => setDateRange(undefined)}
                        >
                          Limpar
                        </Button>
                      )}
                    </div>
                  </div>

                  {/* Native shadcn Calendar */}
                  <Calendar
                    mode="range"
                    defaultMonth={dateRange?.from}
                    selected={dateRange}
                    onSelect={setDateRange}
                    numberOfMonths={1}
                    locale={ptBR}
                    className="rounded-md border bg-card p-1 shadow-xs"
                  />
                </PopoverContent>
              </Popover>
            </div>

            {/* 4. Other Options Popover (Setor, Número, Apagadas) */}
            <div className="flex items-center gap-2 self-end md:self-auto shrink-0">
              <Popover open={moreFiltersOpen} onOpenChange={setMoreFiltersOpen}>
                <PopoverTrigger asChild>
                  <Button
                    variant={moreFiltersCount > 0 ? "secondary" : "outline"}
                    size="default"
                    className="h-9 gap-2 shadow-sm font-medium text-xs"
                  >
                    <SlidersHorizontal className="size-3.5" />
                    <span>Mais Filtros</span>
                    {moreFiltersCount > 0 && (
                      <Badge variant="default" className="size-5 p-0 flex items-center justify-center rounded-full text-[10px]">
                        {moreFiltersCount}
                      </Badge>
                    )}
                  </Button>
                </PopoverTrigger>

                <PopoverContent align="end" className="w-80 space-y-4 p-4 shadow-xl">
                  <div className="flex items-center justify-between border-b pb-2.5">
                    <div className="flex items-center gap-2">
                      <SlidersHorizontal className="size-4 text-primary" />
                      <h4 className="font-semibold text-sm">Filtros Adicionais</h4>
                    </div>
                    {moreFiltersCount > 0 && (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-6 px-2 text-[11px] text-muted-foreground hover:text-foreground"
                        onClick={resetMoreFilters}
                      >
                        Redefinir
                      </Button>
                    )}
                  </div>

                  {/* Setor */}
                  <div className="grid gap-1.5">
                    <Label className="text-xs font-medium text-muted-foreground flex items-center gap-1">
                      <Building2 className="size-3.5" />
                      Setor / Departamento
                    </Label>
                    <Select
                      value={department}
                      onValueChange={(v) => {
                        setDepartment(v);
                        setEmployee(ALL);
                      }}
                    >
                      <SelectTrigger className="w-full">
                        <SelectValue placeholder="Todos os setores" />
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

                  {/* Número WhatsApp */}
                  <div className="grid gap-1.5">
                    <Label className="text-xs font-medium text-muted-foreground flex items-center gap-1">
                      <Smartphone className="size-3.5" />
                      Aparelho / Número
                    </Label>
                    <Select value={instance} onValueChange={setInstance}>
                      <SelectTrigger className="w-full">
                        <SelectValue placeholder="Todos os números" />
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

                  {/* Somente apagadas */}
                  <div className="flex items-center justify-between gap-2 rounded-lg border bg-muted/20 p-2.5">
                    <div className="flex items-center gap-2">
                      <Trash2 className="size-3.5 text-destructive" />
                      <Label htmlFor="feed-deleted-popover" className="text-xs font-medium cursor-pointer">
                        Apenas mensagens apagadas
                      </Label>
                    </div>
                    <Switch
                      id="feed-deleted-popover"
                      checked={deletedOnly}
                      onCheckedChange={setDeletedOnly}
                      aria-label="Somente apagadas"
                    />
                  </div>

                  <Button
                    variant="default"
                    size="sm"
                    className="w-full text-xs font-medium"
                    onClick={() => setMoreFiltersOpen(false)}
                  >
                    Concluído
                  </Button>
                </PopoverContent>
              </Popover>
            </div>
          </div>

          {/* Active Filter Chips */}
          {hasActiveFilters && (
            <div className="flex flex-wrap items-center gap-1.5 border-t pt-2.5">
              <span className="text-xs text-muted-foreground font-medium mr-1 flex items-center gap-1">
                <Filter className="size-3" />
                Filtros:
              </span>

              {text && (
                <Badge variant="secondary" className="gap-1 text-xs">
                  Texto: &ldquo;{text}&rdquo;
                  <button onClick={() => setText("")} className="hover:text-destructive">
                    <X className="size-3" />
                  </button>
                </Badge>
              )}

              {employee !== ALL && (
                <Badge variant="secondary" className="gap-1 text-xs">
                  Colaborador: {employees.find((e) => e.id.toString() === employee)?.name}
                  <button onClick={() => setEmployee(ALL)} className="hover:text-destructive">
                    <X className="size-3" />
                  </button>
                </Badge>
              )}

              {dateRange?.from && (
                <Badge variant="secondary" className="gap-1 text-xs">
                  Período: {format(dateRange.from, "dd/MM/yyyy", { locale: ptBR })}
                  {dateRange.to ? ` até ${format(dateRange.to, "dd/MM/yyyy", { locale: ptBR })}` : ""}
                  <button
                    onClick={() => setDateRange(undefined)}
                    className="hover:text-destructive"
                  >
                    <X className="size-3" />
                  </button>
                </Badge>
              )}

              {department !== ALL && (
                <Badge variant="secondary" className="gap-1 text-xs">
                  Setor: {departments.find((d) => d.id.toString() === department)?.name}
                  <button onClick={() => setDepartment(ALL)} className="hover:text-destructive">
                    <X className="size-3" />
                  </button>
                </Badge>
              )}

              {instance !== ALL && (
                <Badge variant="secondary" className="gap-1 text-xs">
                  Número: {instances.find((i) => i.phone_jid === instance)?.alias || instance}
                  <button onClick={() => setInstance(ALL)} className="hover:text-destructive">
                    <X className="size-3" />
                  </button>
                </Badge>
              )}

              {deletedOnly && (
                <Badge variant="destructive" className="gap-1 text-xs">
                  Somente apagadas
                  <button onClick={() => setDeletedOnly(false)} className="hover:opacity-80">
                    <X className="size-3" />
                  </button>
                </Badge>
              )}
            </div>
          )}
        </div>
      </Card>

      {/* Feed list */}
      {loading ? (
        <Card className="p-12">
          <div className="flex flex-col items-center justify-center gap-3 text-center">
            <Loader2 className="size-8 animate-spin text-primary" />
            <p className="text-sm text-muted-foreground font-medium">Buscando mensagens no banco de dados...</p>
          </div>
        </Card>
      ) : messages.length === 0 ? (
        <Card className="p-12 border-dashed">
          <div className="flex flex-col items-center justify-center gap-3 text-center">
            <div className="flex size-14 items-center justify-center rounded-full bg-muted">
              <MessageSquare className="size-7 text-muted-foreground" />
            </div>
            <CardTitle className="text-base font-semibold">Nenhuma mensagem encontrada</CardTitle>
            <CardDescription className="max-w-md text-xs">
              {hasActiveFilters
                ? "Nenhum registro corresponde aos filtros selecionados. Tente ajustar os parâmetros ou limpar os filtros."
                : "Ainda não há mensagens registradas para exibição no painel."}
            </CardDescription>
            {hasActiveFilters && (
              <Button variant="outline" size="sm" onClick={resetFilters} className="mt-2 gap-1.5">
                <RotateCcw className="size-3.5" />
                Limpar filtros
              </Button>
            )}
          </div>
        </Card>
      ) : (
        <div className="space-y-3">
          <div className="flex items-center justify-between text-xs text-muted-foreground px-1">
            <span>
              Exibindo <strong>{messages.length}</strong> mensagens
              {deletedCount > 0 ? ` (${deletedCount} apagadas pelo remetente)` : ""}
            </span>
            <span className="hidden sm:inline">Ordenado por data (mais recentes primeiro)</span>
          </div>

          <div className="space-y-3">
            {messages.map((m) => (
              <FeedMessageCard key={`${m.instance_jid}|${m.chat_jid}|${m.id}`} message={m} onShowVersions={openVersions} />
            ))}
          </div>

          {!result?.exhausted && (
            <div className="flex justify-center pt-4">
              <Button
                variant="outline"
                onClick={loadMore}
                disabled={loadingMore}
                className="gap-2 px-6 shadow-sm hover:bg-accent"
              >
                {loadingMore ? <Loader2 className="size-4 animate-spin" /> : <Filter className="size-4" />}
                Carregar mensagens anteriores
              </Button>
            </div>
          )}
        </div>
      )}

      <VersionsDialog open={versions !== null} versions={versions?.versions ?? null} onClose={() => setVersions(null)} />
    </PageContainer>
  );
}
