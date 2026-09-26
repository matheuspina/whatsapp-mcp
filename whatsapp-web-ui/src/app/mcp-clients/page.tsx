"use client";

import { useMemo, useState } from "react";
import {
  Cpu,
  Search,
  Server,
  Globe,
  Check,
  Copy,
  RotateCcw,
  X,
  Code2,
  Terminal,
  Cloud,
  Layers,
} from "lucide-react";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { StatCard, StatGrid } from "@/components/common/stat-card";
import { ClientCard } from "@/components/mcp/client-card";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { DEFAULT_MCP_URL, buildClients, isLocalUrl, parseMcpUrl } from "@/lib/mcp-clients";

/** The MCP server is published on port 8081 of the host that serves the panel. */
function guessMcpUrl(): string {
  if (typeof window === "undefined") return DEFAULT_MCP_URL;
  const { protocol, hostname } = window.location;
  return isLocalUrl(window.location.href) ? DEFAULT_MCP_URL : `${protocol}//${hostname}:8081/mcp`;
}

type ClientCategory = "all" | "ide" | "cli" | "cloud";

export default function McpPage() {
  const [input, setInput] = useState(guessMcpUrl);
  const [copied, setCopied] = useState(false);
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState<ClientCategory>("all");

  const url = parseMcpUrl(input);
  const clients = useMemo(() => (url ? buildClients(url) : []), [url]);
  const localUrl = url ? isLocalUrl(url) : true;

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

  // Categorize clients
  const categorizedClients = useMemo(() => {
    return clients.filter((c) => {
      // Category filter
      if (category === "ide") {
        if (!["cursor", "vscode", "windsurf", "antigravity"].includes(c.id)) return false;
      } else if (category === "cli") {
        if (!["claude-code", "gemini-cli", "codex"].includes(c.id)) return false;
      } else if (category === "cloud") {
        if (!c.cloud && !["chatgpt", "claude", "grok"].includes(c.id)) return false;
      }

      // Search query
      if (search.trim()) {
        const q = search.trim().toLowerCase();
        return (
          c.name.toLowerCase().includes(q) ||
          c.kind.toLowerCase().includes(q) ||
          c.id.toLowerCase().includes(q)
        );
      }

      return true;
    });
  }, [clients, category, search]);

  return (
    <PageContainer>
      <PageHeader
        title="Protocolo MCP (Model Context Protocol)"
        description="Conecte assistentes, agentes e IDEs inteligentes para ler e interagir com conversas do WhatsApp de forma segura."
      />

      <StatGrid columns={4}>
        <StatCard
          icon={<Server />}
          label="Protocolo"
          value="MCP SSE / HTTP"
          description="FastMCP Python 3.12"
          large={false}
        />
        <StatCard
          icon={localUrl ? <Globe className="text-muted-foreground" /> : <Cloud className="text-success" />}
          label="Status da URL"
          value={localUrl ? "Ambiente local" : "Acesso externo"}
          description={localUrl ? "Apenas conexões desta máquina" : "Compatível com conectores em nuvem"}
          large={false}
        />
        <StatCard
          icon={<Layers className="text-sky-600 dark:text-sky-400" />}
          label="Clientes suportados"
          value={clients.length}
          description="editores, CLIs e plataformas"
        />
        <StatCard
          icon={<Cpu className="text-emerald-600 dark:text-emerald-400" />}
          label="Ferramentas"
          value="27 MCP Tools"
          description="mensagens, contatos, chats, envio"
          large={false}
        />
      </StatGrid>

      {/* Server URL Configuration Card */}
      <Card className="border-border/80 shadow-sm">
        <CardHeader className="pb-3 border-b">
          <div className="flex items-center justify-between flex-wrap gap-2">
            <div className="flex items-center gap-2">
              <Server className="size-4 text-primary" />
              <CardTitle className="text-sm font-semibold">Endereço do Servidor MCP</CardTitle>
            </div>
            {input !== DEFAULT_MCP_URL && (
              <Button
                variant="ghost"
                size="sm"
                className="h-7 text-xs gap-1 text-muted-foreground hover:text-foreground"
                onClick={resetDefaultUrl}
              >
                <RotateCcw className="size-3" />
                Restaurar padrão
              </Button>
            )}
          </div>
          <CardDescription className="text-xs text-muted-foreground">
            Endereço HTTP onde o FastMCP expõe a interface MCP. Os snippets e instruções abaixo são atualizados dinamicamente conforme você altera esta URL.
          </CardDescription>
        </CardHeader>

        <CardContent className="space-y-4 pt-4">
          <div className="grid gap-1.5">
            <Label htmlFor="mcp-url" className="text-xs font-medium text-muted-foreground flex items-center justify-between">
              <span>URL Base do Endpoint MCP</span>
              {localUrl ? (
                <span className="text-[11px] text-muted-foreground">
                  (Para conectar Claude Web ou ChatGPT, defina <code>MCP_PUBLIC_URL</code> no <code>.env</code>)
                </span>
              ) : (
                <span className="text-[11px] text-emerald-600 dark:text-emerald-400 flex items-center gap-1">
                  <Globe className="size-3" />
                  URL externa com suporte a OAuth
                </span>
              )}
            </Label>
            <div className="flex items-center gap-2">
              <div className="relative flex-1">
                <Input
                  id="mcp-url"
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  aria-invalid={!url}
                  spellCheck={false}
                  className="font-mono text-xs pr-9"
                />
                {input && (
                  <button
                    type="button"
                    onClick={() => setInput("")}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  >
                    <X className="size-4" />
                  </button>
                )}
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-9 gap-1.5 text-xs shrink-0"
                onClick={copyUrl}
                disabled={!url}
              >
                {copied ? <Check className="size-3.5 text-emerald-500" /> : <Copy className="size-3.5" />}
                Copiar URL
              </Button>
            </div>
            {!url && (
              <p className="text-xs text-destructive font-medium">
                Informe uma URL HTTP ou HTTPS válida (ex: http://localhost:8081/mcp).
              </p>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Filter Toolbar for Clients */}
      {url && (
        <div className="space-y-4">
          <div className="flex flex-col sm:flex-row items-center justify-between gap-3">
            {/* Category tabs */}
            <div className="flex items-center gap-1.5 p-1 rounded-lg border bg-muted/40 w-full sm:w-auto overflow-x-auto">
              <Button
                variant={category === "all" ? "default" : "ghost"}
                size="sm"
                className="h-7 text-xs font-medium"
                onClick={() => setCategory("all")}
              >
                Todos ({clients.length})
              </Button>
              <Button
                variant={category === "ide" ? "default" : "ghost"}
                size="sm"
                className="h-7 text-xs font-medium gap-1"
                onClick={() => setCategory("ide")}
              >
                <Code2 className="size-3" />
                Editores & IDEs
              </Button>
              <Button
                variant={category === "cli" ? "default" : "ghost"}
                size="sm"
                className="h-7 text-xs font-medium gap-1"
                onClick={() => setCategory("cli")}
              >
                <Terminal className="size-3" />
                Linha de Comando (CLI)
              </Button>
              <Button
                variant={category === "cloud" ? "default" : "ghost"}
                size="sm"
                className="h-7 text-xs font-medium gap-1"
                onClick={() => setCategory("cloud")}
              >
                <Cloud className="size-3" />
                Cloud & Web
              </Button>
            </div>

            {/* Client search */}
            <div className="relative w-full sm:w-64">
              <Search className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Buscar cliente ou IDE..."
                className="h-8 pl-8 pr-8 text-xs"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
              {search && (
                <button
                  type="button"
                  onClick={() => setSearch("")}
                  className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  <X className="size-3.5" />
                </button>
              )}
            </div>
          </div>

          {/* Clients Grid */}
          {categorizedClients.length === 0 ? (
            <Card className="border-dashed p-8">
              <div className="flex flex-col items-center justify-center gap-2 text-center">
                <Search className="size-6 text-muted-foreground" />
                <p className="text-sm font-medium">Nenhum cliente encontrado para &ldquo;{search}&rdquo;</p>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setSearch("");
                    setCategory("all");
                  }}
                  className="mt-1 text-xs"
                >
                  Limpar filtros
                </Button>
              </div>
            </Card>
          ) : (
            <div className="grid gap-4 lg:grid-cols-2">
              {categorizedClients.map((client) => (
                <ClientCard key={client.id} client={client} localUrl={localUrl} />
              ))}
            </div>
          )}
        </div>
      )}
    </PageContainer>
  );
}
