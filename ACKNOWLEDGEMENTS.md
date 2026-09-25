# Acknowledgements

WhatsApp MCP stands on the work of many people. Thank you.

## Project lineage

- [Luke Harries](https://github.com/lharries), author of [lharries/whatsapp-mcp](https://github.com/lharries/whatsapp-mcp), the original MCP server.
- [Adam Russak](https://github.com/AdamRussak) ([AdamRussak/whatsapp-mcp](https://github.com/AdamRussak/whatsapp-mcp)), who added webhooks, the container split and a webhook UI.
- [Felix Isaac](https://github.com/FelixIsaac) ([FelixIsaac/whatsapp-mcp-extended](https://github.com/FelixIsaac/whatsapp-mcp-extended)), who added reactions, message editing and deletion, group management, polls, presence, newsletters and the curated tool surface, and did the security hardening this project builds on.

## Community contributions

The upstream project credits these community forks for ideas and fixes that reached this codebase:

| Contributor | What they worked out |
|---|---|
| [simonseifert](https://github.com/simonseifert) | Optional on-device voice transcription and multilingual semantic search; `direct_path` column tracking for CDN fallback; inline image content blocks |
| [bitterdev](https://github.com/bitterdev) | WhatsApp LID addressing resolution (`GetAltJID()`), mapping `<id>@lid` recipients to phone JIDs to prevent server error 463 |
| [domdomegg](https://github.com/domdomegg) | Security scan workflow fixes and the `.gitleaks.toml` allowlist |
| [laudite](https://github.com/laudite) | Media captions in `ExtractTextContent()`, quoted/reply context in webhooks, `@mention` auto-detection |
| [kasperpeulen](https://github.com/kasperpeulen) | Contact name resolution priority chain (`FullName > PushName > FirstName > Business`) |
| [Coriatel](https://github.com/Coriatel) | The first working `/api/download` implementation, with manual HKDF/AES-CBC decryption |
| [jedijashwa](https://github.com/jedijashwa) | Fix for reactions silently failing (wrong sender JID lookup); extended MIME type support |
| [slarrain](https://github.com/slarrain) | LID JID normalization, and the diagnosis of the conversation-splitting bug |

## Libraries

- [whatsmeow](https://github.com/tulir/whatsmeow) by Tulir Asokan, the Go implementation of the WhatsApp Web multi-device protocol. Everything here rests on it.
- The [Model Context Protocol](https://modelcontextprotocol.io) and its [Python SDK](https://github.com/modelcontextprotocol/python-sdk).
- [Next.js](https://nextjs.org), [React](https://react.dev), [Tailwind CSS](https://tailwindcss.com), [shadcn/ui](https://ui.shadcn.com), [Radix UI](https://www.radix-ui.com) and [lucide](https://lucide.dev) for the web panel.

See [`NOTICE.md`](NOTICE.md) for the license of each component.
