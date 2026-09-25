"use client";

import { useMemo, useState } from "react";
import { ClientCard } from "@/components/mcp/client-card";
import { CodeBlock } from "@/components/mcp/code-block";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { DEFAULT_MCP_URL, buildClients, isLocalUrl, parseMcpUrl } from "@/lib/mcp-clients";

/** The MCP server is published on port 8081 of the host that serves the panel. */
function guessMcpUrl(): string {
  const { protocol, hostname } = window.location;
  return isLocalUrl(window.location.href) ? DEFAULT_MCP_URL : `${protocol}//${hostname}:8081/mcp`;
}

export default function McpPage() {
  // The page only renders after the auth gate resolves in the browser, so reading `window` here is safe.
  const [input, setInput] = useState(guessMcpUrl);
  const url = parseMcpUrl(input);
  const clients = useMemo(() => (url ? buildClients(url) : []), [url]);
  const localUrl = url ? isLocalUrl(url) : true;

  return (
    <div className="p-8">
      <div className="mx-auto max-w-6xl space-y-6">
        <div>
          <h1 className="text-2xl font-bold">MCP</h1>
          <p className="text-muted-foreground">Connect an AI client to your WhatsApp account</p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Server URL</CardTitle>
            <CardDescription>
              The address clients use to reach the MCP server. Change it if you expose the server on a domain
              (set MCP_PUBLIC_URL in .env to turn on sign-in for it).
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="mcp-url">MCP URL</Label>
              <Input
                id="mcp-url"
                value={input}
                onChange={(e) => setInput(e.target.value)}
                aria-invalid={!url}
                spellCheck={false}
              />
              {!url && <p className="text-sm text-destructive">Enter a full http:// or https:// URL.</p>}
            </div>
            {url && <CodeBlock code={url} />}
            <p className="text-xs text-muted-foreground">
              Without MCP_PUBLIC_URL the endpoint has no login and only answers on this machine. With it, clients
              sign you in with your panel username and password. See docs/mcp-oauth.md.
            </p>
          </CardContent>
        </Card>

        {url && (
          <div className="grid gap-6 lg:grid-cols-2">
            {clients.map((client) => (
              <ClientCard key={client.id} client={client} localUrl={localUrl} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
