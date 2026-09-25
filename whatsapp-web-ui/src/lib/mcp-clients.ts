/** Name the server is registered under in every client. */
export const MCP_SERVER_NAME = "whatsapp";

/** Where the MCP server listens when the panel is opened on this machine (docker-compose publishes it on 8081). */
export const DEFAULT_MCP_URL = "http://localhost:8081/mcp";

export type CodeLanguage = "bash" | "json" | "toml";

/** One thing to copy: a shell command or a config snippet, with a caption saying where it goes. */
export interface SnippetBlock {
  label: string;
  language: CodeLanguage;
  code: string;
}

export interface McpClient {
  id: string;
  name: string;
  /** One line under the name: what kind of client this is. */
  kind: string;
  /** Numbered steps for clients configured through a UI. */
  steps?: string[];
  snippets: SnippetBlock[];
  /** Opens the client and adds the server, for clients that register a URL handler. */
  deepLink?: string;
  /** True when the client's servers reach the MCP endpoint from the internet, so a localhost URL cannot work. */
  cloud?: boolean;
  note?: string;
}

const isHttp = (url: string) => url.startsWith("http://");

const json = (value: unknown) => JSON.stringify(value, null, 2);

const toBase64 = (text: string) => btoa(String.fromCharCode(...new TextEncoder().encode(text)));

/** Normalises what the user typed into the URL clients should use, or null when it is not an http(s) URL. */
export function parseMcpUrl(input: string): string | null {
  try {
    const url = new URL(input.trim());
    if (url.protocol !== "http:" && url.protocol !== "https:") return null;
    return url.toString().replace(/\/$/, "");
  } catch {
    return null;
  }
}

/** True for a URL only this machine can reach, which cloud clients (Claude web, ChatGPT, Grok) cannot use. */
export function isLocalUrl(url: string): boolean {
  try {
    const { hostname } = new URL(url);
    return hostname === "localhost" || hostname === "127.0.0.1" || hostname === "::1" || hostname === "[::1]";
  } catch {
    return false;
  }
}

/** Builds the setup instructions for every supported client from the MCP server URL. */
export function buildClients(url: string): McpClient[] {
  const name = MCP_SERVER_NAME;
  const mcpRemote = (extra: string[] = []) =>
    json({ mcpServers: { [name]: { command: "npx", args: ["-y", "mcp-remote", url, ...extra] } } });
  // mcp-remote refuses plain http unless told otherwise.
  const remoteArgs = isHttp(url) ? ["--allow-http"] : [];

  return [
    {
      id: "claude-code",
      name: "Claude Code",
      kind: "Anthropic CLI",
      snippets: [
        { label: "Add the server", language: "bash", code: `claude mcp add --transport http ${name} ${url}` },
      ],
      steps: ["Run the command in your terminal.", "Inside Claude Code, run /mcp and sign in if the server asks for it."],
    },
    {
      id: "claude",
      name: "Claude",
      kind: "claude.ai, desktop and mobile",
      cloud: true,
      steps: [
        "Open Settings → Connectors.",
        "Choose Add custom connector.",
        `Enter ${name} as the name and paste the URL above.`,
        "Save, then sign in with your panel username and password when asked.",
      ],
      snippets: [
        {
          label: "Claude Desktop with a local URL: add to claude_desktop_config.json",
          language: "json",
          code: mcpRemote(remoteArgs),
        },
      ],
      note: "Custom connectors are called from Anthropic's servers, so they need a public https URL (MCP_PUBLIC_URL). For a localhost URL, use the config file below, which runs on your machine.",
    },
    {
      id: "cursor",
      name: "Cursor",
      kind: "AI code editor",
      deepLink: `cursor://anysphere.cursor-deeplink/mcp/install?name=${name}&config=${encodeURIComponent(toBase64(json({ url })))}`,
      snippets: [
        { label: "Or add to ~/.cursor/mcp.json", language: "json", code: json({ mcpServers: { [name]: { url } } }) },
      ],
    },
    {
      id: "vscode",
      name: "VS Code",
      kind: "GitHub Copilot agent mode",
      deepLink: `vscode:mcp/install?${encodeURIComponent(JSON.stringify({ name, type: "http", url }))}`,
      snippets: [
        {
          label: "Or run in a terminal",
          language: "bash",
          code: `code --add-mcp '${JSON.stringify({ name, type: "http", url })}'`,
        },
        {
          label: "Or add to .vscode/mcp.json",
          language: "json",
          code: json({ servers: { [name]: { type: "http", url } } }),
        },
      ],
    },
    {
      id: "chatgpt",
      name: "ChatGPT",
      kind: "OpenAI",
      cloud: true,
      steps: [
        "Open Settings → Connectors → Advanced and turn on Developer mode.",
        "Back in Connectors, choose Create.",
        `Enter ${name} as the name, paste the URL above and pick OAuth for authentication.`,
        "Create it and sign in with your panel username and password.",
      ],
      snippets: [],
      note: "ChatGPT calls the server from OpenAI's infrastructure, so it needs a public https URL (MCP_PUBLIC_URL). Menu names change between releases and plans.",
    },
    {
      id: "codex",
      name: "Codex",
      kind: "OpenAI CLI and IDE extension",
      snippets: [
        { label: "Add the server", language: "bash", code: `codex mcp add ${name} --url ${url}` },
        { label: "Sign in (when OAuth is on)", language: "bash", code: `codex mcp login ${name}` },
        {
          label: "Or add to ~/.codex/config.toml",
          language: "toml",
          code: `[mcp_servers.${name}]\nurl = "${url}"`,
        },
      ],
    },
    {
      id: "antigravity",
      name: "Antigravity",
      kind: "Google agentic IDE",
      steps: [
        "Open the agent panel, click the ... menu and choose MCP Servers → Manage MCP Servers → View raw config.",
        "Add the entry below and save.",
      ],
      snippets: [
        {
          label: "mcp_config.json",
          language: "json",
          code: json({ mcpServers: { [name]: { serverUrl: url } } }),
        },
      ],
      note: "Antigravity only reads serverUrl for remote servers; url and httpUrl are ignored.",
    },
    {
      id: "gemini-cli",
      name: "Gemini CLI",
      kind: "Google CLI",
      snippets: [
        { label: "Add the server", language: "bash", code: `gemini mcp add --transport http ${name} ${url}` },
        {
          label: "Or add to ~/.gemini/settings.json",
          language: "json",
          code: json({ mcpServers: { [name]: { httpUrl: url } } }),
        },
      ],
    },
    {
      id: "windsurf",
      name: "Windsurf",
      kind: "AI code editor",
      snippets: [
        {
          label: "Add to ~/.codeium/windsurf/mcp_config.json",
          language: "json",
          code: json({ mcpServers: { [name]: { serverUrl: url } } }),
        },
      ],
    },
    {
      id: "grok",
      name: "Grok",
      kind: "xAI API",
      cloud: true,
      snippets: [
        {
          label: "Responses API request",
          language: "bash",
          code: [
            "curl https://api.x.ai/v1/responses \\",
            '  -H "Content-Type: application/json" \\',
            '  -H "Authorization: Bearer $XAI_API_KEY" \\',
            "  -d '" +
              json({
                model: "grok-4.7",
                input: "List my most recent WhatsApp chats",
                tools: [{ type: "mcp", server_url: url, server_label: name }],
              }) +
              "'",
          ].join("\n"),
        },
      ],
      note: "Grok reaches MCP servers through the xAI API, from xAI's servers, so it needs a public https URL (MCP_PUBLIC_URL). The grok.com app has no custom connector option that we could confirm.",
    },
    {
      id: "other",
      name: "Other clients",
      kind: "Any MCP client",
      snippets: [
        {
          label: "Clients that accept a URL",
          language: "json",
          code: json({ mcpServers: { [name]: { url } } }),
        },
        {
          label: "Clients that only run local commands (stdio)",
          language: "json",
          code: mcpRemote(remoteArgs),
        },
      ],
      note: "Any client that follows the MCP authorization spec can sign in on its own once OAuth is on. Scripts can send API_KEY as a bearer token instead.",
    },
  ];
}
