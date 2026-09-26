"use client";

import { useMemo, useState } from "react";
import {
  Check,
  CircleAlert,
  Cloud,
  Code2,
  Copy,
  Cpu,
  ExternalLink,
  Globe,
  Layers,
  RotateCcw,
  Search,
  Server,
  Terminal,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { StatCard, StatGrid } from "@/components/common/stat-card";
import { CodeBlock } from "@/components/mcp/code-block";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  DEFAULT_MCP_URL,
  buildClients,
  isLocalUrl,
  parseMcpUrl,
} from "@/lib/mcp-clients";
import { MCP_TOOL_GROUPS, MCP_TOOLS, type McpToolGroup } from "@/lib/mcp-tools";
import { cn } from "@/lib/utils";

/** The MCP server is published on port 8081 of the host that serves the panel. */
function guessMcpUrl(): string {
  if (typeof window === "undefined") return DEFAULT_MCP_URL;
  const { protocol, hostname } = window.location;
  return isLocalUrl(window.location.href) ? DEFAULT_MCP_URL : `${protocol}//${hostname}:8081/mcp`;
}

type McpTab = "install" | "tools";
type ToolFilter = "all" | "read" | "write";

export default function McpPage() {
  const [input, setInput] = useState(guessMcpUrl);
  const [copied, setCopied] = useState(false);
  const [activeTab, setActiveTab] = useState<McpTab>("install");
  const [selectedClientId, setSelectedClientId] = useState("claude-code");
  const [toolSearch, setToolSearch] = useState("");
  const [toolFilter, setToolFilter] = useState<ToolFilter>("all");

  const url = parseMcpUrl(input);
  const clients = useMemo(() => (url ? buildClients(url) : []), [url]);
  const selectedClient = clients.find((client) => client.id === selectedClientId) ?? clients[0];
  const localUrl = url ? isLocalUrl(url) : true;

  const filteredToolsByGroup = useMemo(() => {
    const query = toolSearch.trim().toLowerCase();

    return MCP_TOOL_GROUPS.map((group) => ({
      group,
      tools: MCP_TOOLS.filter((tool) => {
        if (tool.group !== group) return false;
        if (toolFilter === "read" && !tool.readOnly) return false;
        if (toolFilter === "write" && tool.readOnly) return false;
        if (!query) return true;
        return `${tool.name} ${tool.description} ${tool.group}`.toLowerCase().includes(query);
      }),
    })).filter(({ tools }) => tools.length > 0);
  }, [toolFilter, toolSearch]);

  const copyUrl = async () => {
    if (!url) return;
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      toast.success("URL do MCP copiada!");
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error("Erro ao copiar URL");
    }
  };

  const resetDefaultUrl = () => {
    setInput(DEFAULT_MCP_URL);
    toast.info("URL restaurada para o padrão local.");
  };

  return (
    <PageContainer>
      <PageHeader
        title="Servidor MCP"
        description="Configure uma conexão para seu cliente MCP e consulte as ferramentas disponíveis para o WhatsApp."
      />

      <StatGrid columns={4}>
        <StatCard
          icon={<Server />}
          label="Protocolo"
          value="MCP HTTP"
          description="FastMCP Python 3.12"
          large={false}
        />
        <StatCard
          icon={localUrl ? <Globe className="text-muted-foreground" /> : <Cloud className="text-success" />}
          label="Endpoint"
          value={localUrl ? "Local" : "Externo"}
          description={localUrl ? "Acessível nesta máquina" : "Compatível com clientes cloud"}
          large={false}
        />
        <StatCard
          icon={<Layers className="text-sky-600 dark:text-sky-400" />}
          label="Clientes"
          value={clients.length}
          description="editores, CLIs e cloud"
          large={false}
        />
        <StatCard
          icon={<Cpu className="text-emerald-600 dark:text-emerald-400" />}
          label="Tools"
          value={MCP_TOOLS.length}
          description="disponíveis no servidor"
          large={false}
        />
      </StatGrid>

      <Card className="border-border/80 shadow-sm">
        <CardContent className="p-4">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-end">
            <div className="min-w-0 flex-1 space-y-1.5">
              <div className="flex items-center justify-between gap-2">
                <Label htmlFor="mcp-url" className="text-xs font-medium">
                  URL do servidor MCP
                </Label>
                {localUrl ? (
                  <span className="text-[11px] text-muted-foreground">endpoint local</span>
                ) : (
                  <span className="flex items-center gap-1 text-[11px] text-emerald-600 dark:text-emerald-400">
                    <Globe className="size-3" />
                    endpoint externo
                  </span>
                )}
              </div>
              <div className="relative">
                <Input
                  id="mcp-url"
                  value={input}
                  onChange={(event) => setInput(event.target.value)}
                  aria-invalid={!url}
                  spellCheck={false}
                  className="h-9 pr-9 font-mono text-xs"
                  placeholder="https://seu-dominio/mcp"
                />
                {input && (
                  <button
                    type="button"
                    onClick={() => setInput("")}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    aria-label="Limpar URL"
                  >
                    <X className="size-4" />
                  </button>
                )}
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-9 gap-1.5 text-xs"
                onClick={copyUrl}
                disabled={!url}
              >
                {copied ? <Check className="size-3.5 text-emerald-500" /> : <Copy className="size-3.5" />}
                Copiar URL
              </Button>
              {input !== DEFAULT_MCP_URL && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-9 gap-1.5 text-xs text-muted-foreground"
                  onClick={resetDefaultUrl}
                >
                  <RotateCcw className="size-3" />
                  Restaurar
                </Button>
              )}
            </div>
          </div>
          {!url ? (
            <p className="mt-2 text-xs font-medium text-destructive">
              Informe uma URL HTTP ou HTTPS válida, como http://localhost:8081/mcp.
            </p>
          ) : (
            <p className="mt-2 text-[11px] text-muted-foreground">
              Para clientes cloud, use uma URL pública HTTPS definida por <code>MCP_PUBLIC_URL</code>.
            </p>
          )}
        </CardContent>
      </Card>

      <Tabs
        value={activeTab}
        onValueChange={(value) => setActiveTab(value as McpTab)}
        className="gap-4"
      >
        <TabsList className="h-10 w-full justify-start sm:w-fit">
          <TabsTrigger value="install" className="gap-2 px-4 text-xs sm:text-sm">
            <Terminal className="size-3.5" />
            Conectar cliente
          </TabsTrigger>
          <TabsTrigger value="tools" className="gap-2 px-4 text-xs sm:text-sm">
            <Code2 className="size-3.5" />
            Tools disponíveis
            <Badge variant="secondary" className="ml-0.5 px-1.5 py-0 text-[10px]">
              {MCP_TOOLS.length}
            </Badge>
          </TabsTrigger>
        </TabsList>

        <TabsContent value="install">
          <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_260px]">
            <Card className="border-border/80 shadow-sm">
              <CardHeader className="gap-3 border-b pb-4 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <CardTitle className="text-base">Instalação</CardTitle>
                  <CardDescription className="mt-1 text-xs">
                    Escolha o cliente e copie o comando ou configuração correspondente.
                  </CardDescription>
                </div>
                <div className="w-full space-y-1.5 sm:w-56">
                  <Label htmlFor="mcp-client" className="text-[11px] text-muted-foreground">
                    Cliente MCP
                  </Label>
                  <Select value={selectedClient?.id} onValueChange={setSelectedClientId} disabled={!url}>
                    <SelectTrigger id="mcp-client" className="h-9 w-full text-xs">
                      <SelectValue placeholder="Selecione um cliente" />
                    </SelectTrigger>
                    <SelectContent>
                      {clients.map((client) => (
                        <SelectItem key={client.id} value={client.id}>
                          {client.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </CardHeader>

              <CardContent className="space-y-5 pt-5">
                {!url || !selectedClient ? (
                  <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
                    Informe uma URL válida para gerar a configuração de instalação.
                  </div>
                ) : (
                  <>
                    <div className="flex flex-wrap items-center gap-2">
                      <h2 className="text-sm font-semibold">{selectedClient.name}</h2>
                      <span className="text-xs text-muted-foreground">{selectedClient.kind}</span>
                      {selectedClient.cloud && <Badge variant="outline">Cloud</Badge>}
                      {selectedClient.deepLink && (
                        <Button asChild size="sm" className="ml-auto h-8 gap-1.5 text-xs">
                          <a href={selectedClient.deepLink}>
                            Abrir no {selectedClient.name}
                            <ExternalLink className="size-3" />
                          </a>
                        </Button>
                      )}
                    </div>

                    {selectedClient.cloud && localUrl && (
                      <div className="flex gap-2 rounded-md border border-amber-500/30 bg-amber-500/5 px-3 py-2.5 text-xs text-amber-700 dark:text-amber-300">
                        <CircleAlert className="mt-0.5 size-3.5 shrink-0" />
                        <span>
                          Este cliente acessa o servidor pela internet. Troque o endpoint local por uma URL pública HTTPS antes de configurar.
                        </span>
                      </div>
                    )}

                    {selectedClient.steps && selectedClient.steps.length > 0 && (
                      <ol className="space-y-2 text-sm text-muted-foreground">
                        {selectedClient.steps.map((step, index) => (
                          <li key={step} className="flex gap-2.5">
                            <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-[11px] font-medium text-foreground">
                              {index + 1}
                            </span>
                            <span className="pt-0.5">{step}</span>
                          </li>
                        ))}
                      </ol>
                    )}

                    {selectedClient.snippets.length > 0 && (
                      <div className="space-y-4">
                        {selectedClient.snippets.map((snippet) => (
                          <CodeBlock
                            key={`${selectedClient.id}-${snippet.label}`}
                            code={snippet.code}
                            label={snippet.label}
                            language={snippet.language}
                          />
                        ))}
                      </div>
                    )}

                    {selectedClient.note && (
                      <p className="border-t pt-4 text-xs leading-relaxed text-muted-foreground">
                        {selectedClient.note}
                      </p>
                    )}
                  </>
                )}
              </CardContent>
            </Card>

            <div className="space-y-4">
              <div className="rounded-lg border bg-card p-4">
                <div className="mb-3 flex items-center gap-2">
                  <Server className="size-4 text-primary" />
                  <h2 className="text-sm font-semibold">Sobre a conexão</h2>
                </div>
                <dl className="space-y-3 text-xs">
                  <div className="flex items-center justify-between gap-3">
                    <dt className="text-muted-foreground">Transporte</dt>
                    <dd className="font-medium">HTTP</dd>
                  </div>
                  <div className="flex items-center justify-between gap-3">
                    <dt className="text-muted-foreground">Autenticação</dt>
                    <dd className="font-medium">OAuth / sessão</dd>
                  </div>
                  <div className="flex items-center justify-between gap-3">
                    <dt className="text-muted-foreground">Servidor</dt>
                    <dd className="font-medium">whatsapp</dd>
                  </div>
                </dl>
              </div>

              <div className="rounded-lg border bg-muted/20 p-4 text-xs leading-relaxed text-muted-foreground">
                <p className="font-medium text-foreground">Acesso seguro</p>
                <p className="mt-1.5">
                  O cliente deverá autenticar na primeira conexão. A URL e as credenciais não são armazenadas no navegador.
                </p>
              </div>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="tools">
          <div className="space-y-4">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <h2 className="text-base font-semibold">Tools disponíveis</h2>
                <p className="mt-1 text-xs text-muted-foreground">
                  Catálogo de operações expostas pelo servidor MCP. Esta tela apenas documenta as tools.
                </p>
              </div>
              <div className="relative w-full sm:w-64">
                <Search className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={toolSearch}
                  onChange={(event) => setToolSearch(event.target.value)}
                  placeholder="Buscar tool..."
                  className="h-8 pl-8 pr-8 text-xs"
                />
                {toolSearch && (
                  <button
                    type="button"
                    onClick={() => setToolSearch("")}
                    className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    aria-label="Limpar busca"
                  >
                    <X className="size-3.5" />
                  </button>
                )}
              </div>
            </div>

            <div className="flex w-fit items-center gap-1 rounded-lg border bg-muted/30 p-1">
              {([
                ["all", "Todas"],
                ["read", "Somente leitura"],
                ["write", "Ações"],
              ] as const).map(([value, label]) => (
                <Button
                  key={value}
                  type="button"
                  variant={toolFilter === value ? "default" : "ghost"}
                  size="sm"
                  className="h-7 px-2.5 text-xs"
                  onClick={() => setToolFilter(value)}
                >
                  {label}
                </Button>
              ))}
            </div>

            {filteredToolsByGroup.length === 0 ? (
              <div className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
                Nenhuma tool corresponde à busca atual.
              </div>
            ) : (
              <Card className="overflow-hidden border-border/80 shadow-sm">
                <CardContent className="p-0">
                  {filteredToolsByGroup.map(({ group, tools }, groupIndex) => (
                    <ToolGroupSection key={group} group={group} tools={tools} first={groupIndex === 0} />
                  ))}
                </CardContent>
              </Card>
            )}
          </div>
        </TabsContent>
      </Tabs>
    </PageContainer>
  );
}

function ToolGroupSection({
  group,
  tools,
  first,
}: {
  group: McpToolGroup;
  tools: typeof MCP_TOOLS;
  first: boolean;
}) {
  return (
    <section className={cn(!first && "border-t")}>
      <div className="flex items-center justify-between gap-3 bg-muted/20 px-4 py-3">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{group}</h3>
        <span className="text-[11px] text-muted-foreground">{tools.length}</span>
      </div>
      <div className="divide-y">
        {tools.map((tool) => (
          <div key={tool.name} className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <code className="text-xs font-medium text-foreground">{tool.name}</code>
              <p className="mt-1 text-xs text-muted-foreground">{tool.description}</p>
            </div>
            <div className="flex shrink-0 items-center gap-1.5">
              <Badge variant={tool.readOnly ? "success" : "outline"} className="text-[10px]">
                {tool.readOnly ? "Leitura" : "Ação"}
              </Badge>
              {tool.destructive && (
                <Badge variant="destructive" className="text-[10px]">
                  Sensível
                </Badge>
              )}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
