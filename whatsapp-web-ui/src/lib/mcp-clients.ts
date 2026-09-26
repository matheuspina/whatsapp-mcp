/** Name the server is registered under in every client. */
export const MCP_SERVER_NAME = "whatsapp";

/** Default address used by the public panel route. */
export const DEFAULT_MCP_URL = "http://localhost/mcp";

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
      kind: "Linha de comando",
      snippets: [
        { label: "Adicionar conexão", language: "bash", code: `claude mcp add --transport http ${name} ${url}` },
      ],
      steps: ["Execute o comando no terminal.", "No Claude Code, execute /mcp e faça login se for solicitado."],
    },
    {
      id: "claude",
      name: "Claude",
      kind: "Aplicativo web, desktop e celular",
      cloud: true,
      steps: [
        "Abra Configurações → Conectores.",
        "Escolha Adicionar conector personalizado.",
        `Informe ${name} como nome e cole o endereço acima.`,
        "Salve e faça login com seu usuário e senha quando for solicitado.",
      ],
      snippets: [
        {
          label: "Claude Desktop: configuração para uso neste computador",
          language: "json",
          code: mcpRemote(remoteArgs),
        },
      ],
    },
    {
      id: "cursor",
      name: "Cursor",
      kind: "Editor de código com IA",
      deepLink: `cursor://anysphere.cursor-deeplink/mcp/install?name=${name}&config=${encodeURIComponent(toBase64(json({ url })))}`,
      snippets: [
        { label: "Ou adicione nas configurações do Cursor", language: "json", code: json({ mcpServers: { [name]: { url } } }) },
      ],
    },
    {
      id: "vscode",
      name: "VS Code",
      kind: "Editor com assistente de IA",
      deepLink: `vscode:mcp/install?${encodeURIComponent(JSON.stringify({ name, type: "http", url }))}`,
      snippets: [
        {
          label: "Ou execute no terminal",
          language: "bash",
          code: `code --add-mcp '${JSON.stringify({ name, type: "http", url })}'`,
        },
        {
          label: "Ou adicione às configurações do VS Code",
          language: "json",
          code: json({ servers: { [name]: { type: "http", url } } }),
        },
      ],
    },
    {
      id: "chatgpt",
      name: "ChatGPT",
      kind: "Aplicativo de IA",
      cloud: true,
      steps: [
        "Abra Configurações → Conectores → Avançado e ative o modo de desenvolvedor.",
        "Volte aos Conectores e escolha Criar.",
        `Informe ${name} como nome, cole o endereço acima e escolha OAuth para autenticação.`,
        "Crie a conexão e faça login com seu usuário e senha do painel.",
      ],
      snippets: [],
    },
    {
      id: "codex",
      name: "Codex",
      kind: "Linha de comando e extensão para editor",
      snippets: [
        { label: "Adicionar conexão", language: "bash", code: `codex mcp add ${name} --url ${url}` },
        { label: "Fazer login (se solicitado)", language: "bash", code: `codex mcp login ${name}` },
        {
          label: "Ou adicione às configurações do Codex",
          language: "toml",
          code: `[mcp_servers.${name}]\nurl = "${url}"`,
        },
      ],
    },
    {
      id: "antigravity",
      name: "Antigravity",
      kind: "Editor com assistente de IA",
      steps: [
        "Abra o painel do agente, clique no menu ... e escolha MCP Servers → Manage MCP Servers → View raw config.",
        "Adicione o item abaixo e salve.",
      ],
      snippets: [
        {
          label: "mcp_config.json",
          language: "json",
          code: json({ mcpServers: { [name]: { serverUrl: url } } }),
        },
      ],
    },
    {
      id: "gemini-cli",
      name: "Gemini CLI",
      kind: "Linha de comando",
      snippets: [
        { label: "Adicionar conexão", language: "bash", code: `gemini mcp add --transport http ${name} ${url}` },
        {
          label: "Ou adicione às configurações do Gemini",
          language: "json",
          code: json({ mcpServers: { [name]: { httpUrl: url } } }),
        },
      ],
    },
    {
      id: "windsurf",
      name: "Windsurf",
      kind: "Editor de código com IA",
      snippets: [
        {
          label: "Adicionar às configurações do Windsurf",
          language: "json",
          code: json({ mcpServers: { [name]: { serverUrl: url } } }),
        },
      ],
    },
    {
      id: "grok",
      name: "Grok",
      kind: "Serviço de IA",
      cloud: true,
      snippets: [
        {
          label: "Solicitação de conexão",
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
    },
    {
      id: "other",
      name: "Other clients",
      kind: "Outro aplicativo compatível",
      snippets: [
        {
          label: "Aplicativos que aceitam um endereço",
          language: "json",
          code: json({ mcpServers: { [name]: { url } } }),
        },
        {
          label: "Aplicativos que usam comandos neste computador",
          language: "json",
          code: mcpRemote(remoteArgs),
        },
      ],
    },
  ];
}
