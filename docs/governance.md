# Governance: several numbers, people, audit and privacy

The bridge can monitor several WhatsApp numbers at once, attribute every message to the number that captured it and to the
person and department that held the number at that moment, keep an audit trail, and limit what each AI client may see.
This page describes how it works and where it stops.

It does not make monitoring lawful by itself. Recording employees' messages and the messages of customers who talk to
them needs a legal basis, a written policy and, usually, a data protection impact assessment. The panel helps with the
records (attestation, access log, anonymization, retention); the decisions are the company's.

## Numbers (instances)

Each number is a linked device (*Aparelho Conectado*) of a real phone: the person keeps using the phone normally. Add
numbers in **Números WhatsApp**. The panel asks for a name, the responsible person, whether the number may send, and the
corporate-asset attestation, then shows a QR code that renews itself until it is scanned. Closing the dialog before the
scan releases the pairing.

| Status | Meaning |
|--------|---------|
| `pairing` | QR code shown, not scanned yet. Pairings that are never scanned are dropped, and rows left by a restart are cleaned up. |
| `connected` / `disconnected` | Last connection state the bridge recorded. The panel also shows whether the number is online right now. |
| `logged_out` | WhatsApp ended the session (for example the phone was offline for about 14 days). Pair again. |
| `removed` | Retired by an operator. Credentials are deleted, the row and every captured message stay. |

At most `MAX_PENDING_PAIRINGS` (default 5) pairings can be in progress at once. An install that already had one number
keeps working unchanged; its earlier messages are attributed to that number the first time the new bridge starts.

Each number has its own send warm-up state (`store/antiban_warmup_<number>.json`; the first number keeps the configured
file), and one number dropping does not restart the container: the bridge only exits when **every** paired number has
been offline for over three minutes, as a single-number install always did.

## People, departments and who held a number

A department has a name and a description (context for the AI). An employee has a role, an e-mail and an active flag. A
number has one current owner, and every change of owner is kept in `instance_assignments` with the period it lasted.
Messages are attributed through that history, so when a number changes hands, the earlier conversations stay with the
earlier person. Filters by employee or department (in the panel, in the search tools and in the audit tools) follow the
same rule.

A number without an owner still records messages; they just carry no employee or department.

## Sending

By default the system does not send from a monitored number unless you allow it, per number:

- `allow_send` is off for numbers paired from the panel unless the box is ticked. Numbers that already existed when the
  migration ran keep sending allowed, so existing installs keep working. A device paired through the terminal or
  pairing-code flow, or loaded at startup without a row, follows `INSTANCE_ALLOW_SEND_DEFAULT` (default `true`); set it
  to `false` for a monitoring-only deployment.
- Every route that acts on the WhatsApp account (send, edit, delete, reactions, groups, presence, polls, blocklist,
  newsletters, typing, pin/mute/archive, disappearing timer, about) checks the flag and answers `403` when it is off.
- With more than one number paired, those routes require `?instance=` (an instance id, a JID or a phone number), so a
  message is never sent from a guessed phone. With one number, nothing changes. MCP tools that act on WhatsApp accept
  `instance_jid` for the same purpose.

Read-only routes fall back to a default number when none is named.

## Audit

- **Revoked messages.** When the sender deletes a message for everyone, the message stays, flagged with when and by whom
  (`deleted_at`, `deleted_by`), and the revoked text is kept in `message_versions`. Only revocations received while the
  number was connected are seen. "Delete for me" on the phone is not visible to the bridge.
- **Edits.** The message shows its latest text; every earlier text is a version. A history sync that redelivers the
  original never overwrites an edit.
- **The same message on two numbers** (a group both belong to) is stored once per number. Readers that only want to read
  a conversation see it once; the audit tools show every copy with its number and holder.
- **Panel:** *Mensagens* is the unified feed (filter by department, employee, number, text, dates, deleted only) and
  *Auditoria* lists revoked messages, the access log and the privacy operations.
- **MCP:** `get_audit_trail`, `get_audit_deleted_messages`, `get_employee_activity_summary` (totals, per day, busiest
  conversations and how fast the person answered, computed from the index without reading messages),
  `search_department_conversations`, and the `department_id`, `employee_id` and `instance_jid` filters of
  `search_messages`. Semantic search is available through MCP only; the panel searches by words.

### Access log

Every tool call by an MCP client and every read of the feed by the panel is recorded (`access_log`): who, which tool or
route, the parameters (values only for tools that read; tools that act on WhatsApp record parameter names, never message
bodies), and how many results. Denied attempts are recorded too. `MCP_ACCESS_LOG=false` turns off the MCP side. The
log is shown in **Auditoria → Acessos** and by the `list_access_log` tool (unrestricted clients only).

## Who may see what through MCP

Without a policy the MCP server behaves as before: every client sees everything. A policy file limits that. Set
`MCP_ACCESS_POLICY` to a JSON file (in Docker, put it in `./store` and use `/app/store/access-policy.json`):

```json
{
  "default": { "read_only": true },
  "clients": {
    "<oauth client id>": {
      "departments": [2],
      "employees": [5],
      "instances": ["5511900000001@s.whatsapp.net"],
      "read_only": true,
      "deny_tools": ["get_audit_deleted_messages"],
      "allow_tools": null
    }
  }
}
```

- The client id is the OAuth client the AI connected with (see the *Acessos* tab or the log for the ids in use). Without
  OAuth every caller is `local`, which can only be given the `default` rule.
- `departments` and `employees` grant the numbers those people held, **for the periods they held them**; `instances`
  grants a number for good. Grants add up. A client with no data grant sees everything (subject to `read_only` and the
  tool lists).
- `read_only` blocks every tool that acts on WhatsApp. `MCP_READ_ONLY=true` makes it the default for everyone.
- `allow_tools` (when a list) and `deny_tools` restrict individual tools.
- The file is re-read when it changes. A file that is missing, not valid JSON or malformed **denies everything** instead
  of opening everything, and so does a rule that names people or departments when the organization tables cannot be read.

The restriction is applied where data is read, not tool by tool: database connections opened by the server show only the
allowed messages, chats and address books, and search filters by the same windows, so tools added later inherit it. A
restricted client also sees only the people, departments and numbers that its scope touches, and its filters can narrow
the scope but never widen it.

This limits the AI client, not someone who can open `store/` directly, and it does not replace protecting the volume and
the backups.

## Corporate-asset attestation

Pairing from the panel requires ticking a statement that the number is a company asset and that the responsible person was
told the conversations are recorded. The bridge stores who confirmed, when, and which version of the wording. Numbers
connected before this existed show as pending and can be confirmed on their card.

Set `REQUIRE_CORPORATE_CONFIRMATION=true` to make the bridge stop recording messages (live and history sync) of numbers
without the attestation.

## Privacy (LGPD)

In **Auditoria → Privacidade**, or through the API:

- **Anonymize a person** (`POST /api/privacy/anonymize {"subject": "<phone or JID>"}`): their messages in the direct chat
  and what they wrote in groups lose text, name and media references; the direct chat moves to a made-up identifier;
  nicknames, earlier versions and webhook delivery logs of that chat are deleted; media files saved by the download
  endpoint (`store/media/`) are removed; the search index forgets the old identifier at its next pass. Other people's messages in a group stay.
- **Retention** (`RETENTION_DAYS`, or `POST /api/privacy/purge {"days": N}`): deletes messages older than N days from every
  number, with their versions and webhook logs. `RETENTION_DAYS` runs it at startup and then daily.
- Both are recorded in `privacy_log` with who asked, and cannot be undone.

What this does not reach: backups of `store/` (including the copy the migration writes next to `messages.db`),
`whatsapp.db` (managed by whatsmeow: contacts, keys), media saved outside `store/media/` (for example by automatic
download) or already fetched by a webhook consumer, text the AI client
already received, and any copy outside the system. Encryption at rest is not provided; protect the disk or volume.

## Known limits

- The bridge relies on an unofficial client library. WhatsApp can restrict accounts that use unofficial clients, and
  linked devices are logged out when the phone stays offline for about two weeks.
- Attribution of a message to a holder is exact to the moment of the assignment change; the panel shows the owner
  registered at that moment, not who physically typed.
- Daily counters of `get_employee_activity_summary` use `DISPLAY_TZ` days. A period with more than 200,000 messages is
  cut and says so.
- The summary is computed from indexed metadata on demand; there are no scheduled summaries and no language-model
  summaries (nothing is sent to a hosted model to produce them).
