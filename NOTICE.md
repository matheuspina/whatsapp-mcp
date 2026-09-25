# Notice

**WhatsApp MCP** — Copyright (c) 2026 Matheus Pina

This project is a derivative work. Its parts are under different terms, described below.

## Licensing at a glance

| Part | Author | License |
|------|--------|---------|
| Modifications and additions made in this repository (web panel login and sessions, panel redesign and rebranding, deployment and documentation changes, and any new code added here) | Matheus Pina | [PolyForm Noncommercial 1.0.0](LICENSE) |
| Code inherited from the projects listed under [Upstream projects](#upstream-projects) | Their respective authors | MIT License, reproduced [below](#mit-license-upstream-notice) |

The [`LICENSE`](LICENSE) file is the license that applies to this project as distributed by Matheus Pina.
Noncommercial use is free. **Commercial use requires a separate license**: see [`COMMERCIAL.md`](COMMERCIAL.md)
or write to <mathpinab@gmail.com>.

The MIT License is permissive, so the inherited portions stay available under it as their authors published them;
nothing in this repository takes that away. The commercial restriction applies to the work added here.
Where inherited and new code are combined in one file, treat the file as governed by the `LICENSE`
unless you can identify the inherited portion and use it separately under the MIT License.

## Upstream projects

This repository descends from a chain of open source forks. Their history is preserved in the git log.

1. [lharries/whatsapp-mcp](https://github.com/lharries/whatsapp-mcp), Copyright (c) 2025 Luke Harries. The original MCP server.
2. [AdamRussak/whatsapp-mcp](https://github.com/AdamRussak/whatsapp-mcp). Added webhooks, the container split and a webhook UI.
3. [FelixIsaac/whatsapp-mcp-extended](https://github.com/FelixIsaac/whatsapp-mcp-extended). Added reactions, editing and deleting messages,
   group management, polls, presence, newsletters, the curated MCP tool surface and the security hardening this project builds on.

Ideas and fixes from the wider community that reached this codebase through those projects are credited in
[`ACKNOWLEDGEMENTS.md`](ACKNOWLEDGEMENTS.md).

## MIT License (upstream notice)

The following notice comes from the original project and must be kept with its code:

```
MIT License

Copyright (c) 2025 Luke Harries

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## Third-party dependencies

This project depends on other open source software, each under its own license and not affected by the terms above.
The main ones:

| Component | Used for | License |
|-----------|----------|---------|
| [whatsmeow](https://github.com/tulir/whatsmeow) | WhatsApp Web multi-device protocol (Go) | MPL-2.0 |
| [MCP Python SDK / FastMCP](https://github.com/modelcontextprotocol/python-sdk) | MCP server | MIT |
| [Next.js](https://github.com/vercel/next.js), [React](https://github.com/facebook/react) | Web panel | MIT |
| [shadcn/ui](https://github.com/shadcn-ui/ui), [Radix UI](https://github.com/radix-ui/primitives), [Tailwind CSS](https://github.com/tailwindlabs/tailwindcss) | Web panel UI | MIT |
| [lucide](https://github.com/lucide-icons/lucide) | Icons | ISC |
| [SQLite](https://www.sqlite.org/copyright.html) via [go-sqlite3](https://github.com/mattn/go-sqlite3) | Storage | Public domain / MIT |

Check each project for the exact terms before redistributing. This list is a guide, not legal advice.

## Trademarks

WhatsApp is a trademark of Meta Platforms, Inc. This project is independent. It is **not affiliated with, endorsed by
or sponsored by** WhatsApp or Meta. All other names are the property of their owners.
