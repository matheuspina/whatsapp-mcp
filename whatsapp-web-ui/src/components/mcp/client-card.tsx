"use client";

import { ExternalLink } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { buttonVariants } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { CodeBlock } from "@/components/mcp/code-block";
import type { McpClient } from "@/lib/mcp-clients";
import { cn } from "@/lib/utils";

interface ClientCardProps {
  client: McpClient;
  /** True when the URL only works on this machine. */
  localUrl: boolean;
}

/** Setup instructions for one MCP client: a connect button when the client has one, then the steps and config to copy. */
export function ClientCard({ client, localUrl }: ClientCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{client.name}</CardTitle>
        <CardDescription>{client.kind}</CardDescription>
        {client.deepLink && (
          <CardAction>
            <a href={client.deepLink} className={cn(buttonVariants({ size: "sm" }))}>
              <ExternalLink className="mr-2 h-4 w-4" />
              Connect
            </a>
          </CardAction>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        {client.cloud && localUrl && (
          <Badge variant="warning">
            Needs a public https URL
          </Badge>
        )}
        {client.steps && (
          <ol className="list-decimal space-y-1 pl-5 text-sm">
            {client.steps.map((step) => (
              <li key={step}>{step}</li>
            ))}
          </ol>
        )}
        {client.snippets.map((snippet) => (
          <CodeBlock key={snippet.label} label={snippet.label} code={snippet.code} />
        ))}
        {client.note && <p className="text-xs text-muted-foreground">{client.note}</p>}
      </CardContent>
    </Card>
  );
}
