"use client";

import {
  ExternalLink,
  Cloud,
  AlertTriangle,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { CodeBlock } from "@/components/mcp/code-block";
import { ClientIcon } from "@/components/mcp/client-icon";
import type { McpClient } from "@/lib/mcp-clients";

interface ClientCardProps {
  client: McpClient;
  /** True when the URL only works on this machine. */
  localUrl: boolean;
}

export function ClientCard({ client, localUrl }: ClientCardProps) {
  const needsPublicUrl = client.cloud && localUrl;

  return (
    <Card className="flex flex-col justify-between overflow-hidden border-border/80 shadow-sm transition-all duration-200 hover:border-primary/40 hover:shadow-md">
      <CardHeader className="pb-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <div className="flex size-10 items-center justify-center rounded-xl bg-muted/80 border border-border/60 shrink-0">
              <ClientIcon id={client.id} className="size-5" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <CardTitle className="text-base font-semibold">{client.name}</CardTitle>
                {client.cloud ? (
                  <Badge variant="outline" className="text-[10px] gap-1 py-0">
                    <Cloud className="size-2.5" />
                    Online
                  </Badge>
                ) : (
                  <Badge variant="secondary" className="text-[10px] py-0">
                    Neste computador
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
              <p className="font-semibold">É necessário um endereço público (HTTPS)</p>
              <p className="text-[11px] opacity-90">
                Este aplicativo acessa o WhatsApp pela internet. Use o endereço público mostrado acima antes de configurá-lo.
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
