"use client";

import {
  ExternalLink,
  Terminal,
  Code2,
  Cloud,
  Cpu,
  Sparkles,
  Bot,
  AlertTriangle,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { CodeBlock } from "@/components/mcp/code-block";
import type { McpClient } from "@/lib/mcp-clients";

interface ClientCardProps {
  client: McpClient;
  /** True when the URL only works on this machine. */
  localUrl: boolean;
}

function getClientIcon(id: string) {
  switch (id) {
    case "claude-code":
    case "gemini-cli":
    case "codex":
      return <Terminal className="size-5 text-amber-500" />;
    case "cursor":
    case "vscode":
    case "windsurf":
      return <Code2 className="size-5 text-sky-500" />;
    case "antigravity":
      return <Sparkles className="size-5 text-indigo-500" />;
    case "chatgpt":
    case "claude":
      return <Bot className="size-5 text-emerald-500" />;
    case "grok":
      return <Cpu className="size-5 text-rose-500" />;
    default:
      return <Cloud className="size-5 text-primary" />;
  }
}

export function ClientCard({ client, localUrl }: ClientCardProps) {
  const needsPublicUrl = client.cloud && localUrl;

  return (
    <Card className="flex flex-col justify-between overflow-hidden border-border/80 shadow-sm transition-all duration-200 hover:border-primary/40 hover:shadow-md">
      <CardHeader className="pb-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <div className="flex size-10 items-center justify-center rounded-xl bg-muted/80 border border-border/60 shrink-0">
              {getClientIcon(client.id)}
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <CardTitle className="text-base font-semibold">{client.name}</CardTitle>
                {client.cloud ? (
                  <Badge variant="outline" className="text-[10px] gap-1 py-0">
                    <Cloud className="size-2.5" />
                    Cloud
                  </Badge>
                ) : (
                  <Badge variant="secondary" className="text-[10px] py-0">
                    Local
                  </Badge>
                )}
              </div>
              <CardDescription className="text-xs text-muted-foreground mt-0.5 truncate">
                {client.kind}
              </CardDescription>
            </div>
          </div>

          {client.deepLink && (
            <Button asChild size="sm" className="gap-1.5 shrink-0 shadow-sm">
              <a href={client.deepLink}>
                <ExternalLink className="size-3.5" />
                Instalar
              </a>
            </Button>
          )}
        </div>
      </CardHeader>

      <CardContent className="space-y-4 pt-0">
        {needsPublicUrl && (
          <div className="flex items-start gap-2.5 rounded-lg border border-warning/50 bg-warning/10 p-3 text-xs text-warning">
            <AlertTriangle className="size-4 shrink-0 mt-0.5" />
            <div>
              <p className="font-semibold">Requer URL Pública (HTTPS)</p>
              <p className="text-[11px] opacity-90">
                Este cliente executa na nuvem e não alcança endereços localhost. Configure <code>MCP_PUBLIC_URL</code> no arquivo <code>.env</code>.
              </p>
            </div>
          </div>
        )}

        {client.steps && client.steps.length > 0 && (
          <div className="space-y-2">
            <p className="text-xs font-medium text-foreground">Passo a passo:</p>
            <div className="space-y-1.5 pl-1">
              {client.steps.map((step, idx) => (
                <div key={idx} className="flex items-start gap-2 text-xs text-muted-foreground">
                  <span className="flex size-4 shrink-0 items-center justify-center rounded-full bg-primary/10 text-[10px] font-bold text-primary">
                    {idx + 1}
                  </span>
                  <span className="leading-tight">{step}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {client.snippets.length > 0 && (
          <div className="space-y-3">
            {client.snippets.map((snippet, sIdx) => (
              <CodeBlock
                key={sIdx}
                label={snippet.label}
                language={snippet.language}
                code={snippet.code}
              />
            ))}
          </div>
        )}

        {client.note && (
          <div className="rounded-md bg-muted/40 border border-border/40 p-2.5 text-[11px] leading-relaxed text-muted-foreground">
            {client.note}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
