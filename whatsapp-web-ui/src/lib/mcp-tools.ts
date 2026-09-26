export type McpToolGroup =
  | "Consultas"
  | "Pesquisa"
  | "Contatos e mídia"
  | "Ações de mensagens"
  | "Grupos e conta"
  | "Organização"
  | "Auditoria";

export interface McpToolDefinition {
  name: string;
  group: McpToolGroup;
  description: string;
  readOnly: boolean;
  destructive?: boolean;
}

export const MCP_TOOL_GROUPS: McpToolGroup[] = [
  "Consultas",
  "Pesquisa",
  "Contatos e mídia",
  "Ações de mensagens",
  "Grupos e conta",
  "Organização",
  "Auditoria",
];

/** Public catalogue of the tools registered in whatsapp-mcp-server/main.py. */
export const MCP_TOOLS: McpToolDefinition[] = [
  {
    name: "search_contacts",
    group: "Consultas",
    description: "Busca contatos por nome ou número de telefone.",
    readOnly: true,
  },
  {
    name: "list_messages",
    group: "Consultas",
    description: "Lista mensagens por período, conversa, termo e paginação.",
    readOnly: true,
  },
  {
    name: "list_chats",
    group: "Consultas",
    description: "Lista conversas monitoradas e suas últimas mensagens.",
    readOnly: true,
  },
  {
    name: "get_chat",
    group: "Consultas",
    description: "Consulta os detalhes de uma conversa.",
    readOnly: true,
  },
  {
    name: "get_message_context",
    group: "Consultas",
    description: "Retorna as mensagens ao redor de uma mensagem específica.",
    readOnly: true,
  },
  {
    name: "get_group_info",
    group: "Consultas",
    description: "Consulta participantes e metadados de um grupo.",
    readOnly: true,
  },
  {
    name: "get_profile_picture",
    group: "Consultas",
    description: "Obtém a imagem de perfil de uma pessoa ou grupo.",
    readOnly: true,
  },
  {
    name: "search_messages",
    group: "Pesquisa",
    description: "Pesquisa o histórico por palavras e significado.",
    readOnly: true,
  },
  {
    name: "search_department_conversations",
    group: "Pesquisa",
    description: "Pesquisa conversas vinculadas a um departamento.",
    readOnly: true,
  },
  {
    name: "index_status",
    group: "Pesquisa",
    description: "Mostra a cobertura e o estado do índice de pesquisa.",
    readOnly: true,
  },
  {
    name: "list_all_contacts",
    group: "Contatos e mídia",
    description: "Lista contatos do WhatsApp com seus dados disponíveis.",
    readOnly: true,
  },
  {
    name: "get_contact_context",
    group: "Contatos e mídia",
    description: "Consulta detalhes de contato, conversas e última interação.",
    readOnly: true,
  },
  {
    name: "get_direct_chat_by_contact",
    group: "Contatos e mídia",
    description: "Encontra a conversa direta associada a um telefone.",
    readOnly: true,
  },
  {
    name: "download_media",
    group: "Contatos e mídia",
    description: "Baixa a mídia anexada a uma mensagem.",
    readOnly: true,
  },
  {
    name: "manage_nickname",
    group: "Contatos e mídia",
    description: "Consulta ou altera apelidos locais de contatos.",
    readOnly: false,
  },
  {
    name: "send_message",
    group: "Ações de mensagens",
    description: "Envia uma mensagem para uma pessoa ou grupo.",
    readOnly: false,
  },
  {
    name: "send_file",
    group: "Ações de mensagens",
    description: "Envia imagem, vídeo, documento ou outro arquivo.",
    readOnly: false,
  },
  {
    name: "send_audio_message",
    group: "Ações de mensagens",
    description: "Envia um arquivo de áudio como mensagem de voz.",
    readOnly: false,
  },
  {
    name: "send_reaction",
    group: "Ações de mensagens",
    description: "Adiciona ou remove uma reação em uma mensagem.",
    readOnly: false,
  },
  {
    name: "edit_message",
    group: "Ações de mensagens",
    description: "Edita uma mensagem enviada anteriormente.",
    readOnly: false,
  },
  {
    name: "delete_message",
    group: "Ações de mensagens",
    description: "Revoga uma mensagem do WhatsApp.",
    readOnly: false,
    destructive: true,
  },
  {
    name: "mark_read",
    group: "Ações de mensagens",
    description: "Marca mensagens como lidas e envia a confirmação.",
    readOnly: false,
  },
  {
    name: "create_poll",
    group: "Ações de mensagens",
    description: "Cria e envia uma enquete em uma conversa.",
    readOnly: false,
  },
  {
    name: "request_history",
    group: "Ações de mensagens",
    description: "Solicita mensagens antigas disponíveis no telefone.",
    readOnly: false,
  },
  {
    name: "manage_group",
    group: "Grupos e conta",
    description: "Cria, atualiza e administra participantes de grupos.",
    readOnly: false,
    destructive: true,
  },
  {
    name: "set_presence",
    group: "Grupos e conta",
    description: "Define a presença da conta como disponível ou ausente.",
    readOnly: false,
  },
  {
    name: "subscribe_presence",
    group: "Grupos e conta",
    description: "Inscreve-se nas atualizações de presença de um contato.",
    readOnly: false,
  },
  {
    name: "get_blocklist",
    group: "Grupos e conta",
    description: "Lista os usuários bloqueados pela conta.",
    readOnly: true,
  },
  {
    name: "manage_blocklist",
    group: "Grupos e conta",
    description: "Bloqueia ou desbloqueia um usuário.",
    readOnly: false,
    destructive: true,
  },
  {
    name: "manage_newsletter",
    group: "Grupos e conta",
    description: "Segue, deixa de seguir ou cria canais do WhatsApp.",
    readOnly: false,
    destructive: true,
  },
  {
    name: "list_departments",
    group: "Organização",
    description: "Lista os departamentos monitorados.",
    readOnly: true,
  },
  {
    name: "list_employees",
    group: "Organização",
    description: "Lista colaboradores, funções e departamentos.",
    readOnly: true,
  },
  {
    name: "resolve_employee",
    group: "Organização",
    description: "Resolve um nome ou função para um colaborador.",
    readOnly: true,
  },
  {
    name: "list_instances",
    group: "Organização",
    description: "Lista os números monitorados, responsáveis e permissões.",
    readOnly: true,
  },
  {
    name: "get_employee_activity_summary",
    group: "Organização",
    description: "Resume a atividade de um colaborador em um período.",
    readOnly: true,
  },
  {
    name: "list_access_log",
    group: "Auditoria",
    description: "Lista acessos aos dados de mensagens pelo servidor ou painel.",
    readOnly: true,
  },
  {
    name: "get_audit_trail",
    group: "Auditoria",
    description: "Consulta o histórico de auditoria de uma conversa ou operação.",
    readOnly: true,
  },
  {
    name: "get_audit_deleted_messages",
    group: "Auditoria",
    description: "Lista mensagens revogadas e suas versões preservadas.",
    readOnly: true,
  },
];
