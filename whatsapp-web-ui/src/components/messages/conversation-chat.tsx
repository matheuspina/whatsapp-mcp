"use client";

import {
  AudioLines,
  ChevronUp,
  FileText,
  Image as ImageIcon,
  MessageCircle,
  MessageSquare,
  Mic,
  Play,
  Smartphone,
  Users,
  Video,
} from "lucide-react";
import { Loader2 } from "lucide-react";
import { CardDescription } from "@/components/ui/card";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { MessageConversation, FeedMessage } from "@/lib/api";
import { cn } from "@/lib/utils";
import { whenLabel } from "@/components/audit/feed-message";

interface ConversationChatProps {
  conversations: MessageConversation[];
  selectedConversation: MessageConversation | null;
  onSelectConversation: (conversation: MessageConversation) => void;
  conversationsLoading: boolean;
  messages: FeedMessage[];
  messagesLoading: boolean;
  hasMore: boolean;
  onLoadOlder: () => void;
  loadingOlder: boolean;
}

function conversationKey(conversation: MessageConversation): string {
  return `${conversation.instance_jid || "legacy"}|${conversation.chat_jid}`;
}

function displayName(conversation: MessageConversation): string {
  return conversation.chat_name || conversation.chat_jid.split("@")[0] || "Conversa sem nome";
}

function initials(value: string): string {
  const words = value.trim().split(/\s+/).filter(Boolean);
  return (words.length > 1 ? `${words[0][0]}${words[words.length - 1][0]}` : value.slice(0, 2)).toUpperCase();
}

function mediaIcon(mediaType?: string) {
  const type = mediaType?.toLowerCase() || "";
  if (type.includes("audio") || type.includes("ptt") || type.includes("voz")) return <Mic className="size-3.5" />;
  if (type.includes("image") || type.includes("foto")) return <ImageIcon className="size-3.5" />;
  if (type.includes("video")) return <Video className="size-3.5" />;
  return <FileText className="size-3.5" />;
}

function isAudio(mediaType?: string): boolean {
  const type = mediaType?.toLowerCase() || "";
  return type.includes("audio") || type.includes("ptt") || type.includes("voz");
}

function dateKey(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString("pt-BR");
}

function dateLabel(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString("pt-BR", { day: "2-digit", month: "long", year: "numeric" });
}

function AudioPreview() {
  const bars = [3, 8, 5, 12, 7, 16, 10, 6, 14, 8, 4, 11, 6, 9, 4, 7, 3];
  return (
    <div className="flex items-center gap-2 rounded-lg bg-black/5 px-2.5 py-2 dark:bg-white/5">
      <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary" title="Áudio capturado">
        <Play className="ml-0.5 size-3 fill-current" />
      </span>
      <span className="flex h-5 items-end gap-px" aria-hidden="true">
        {bars.map((height, index) => (
          <span key={index} className="w-0.5 rounded-full bg-current opacity-60" style={{ height }} />
        ))}
      </span>
      <span className="text-[11px] font-medium">Áudio</span>
    </div>
  );
}

function MessageBubble({ message }: { message: FeedMessage }) {
  const sender = message.is_from_me ? "Você" : message.sender_name || message.sender || "Contato";
  const deleted = message.is_deleted_remote;

  return (
    <div className={cn("flex items-end gap-2", message.is_from_me ? "justify-end" : "justify-start")}>
      {!message.is_from_me && (
        <Avatar className="size-7 shrink-0 text-[10px]">
          <AvatarFallback>{initials(sender)}</AvatarFallback>
        </Avatar>
      )}
      <div
        className={cn(
          "max-w-[min(80%,38rem)] rounded-2xl px-3 py-2 shadow-xs",
          message.is_from_me
            ? "rounded-br-sm bg-primary text-primary-foreground"
            : "rounded-bl-sm border border-border/70 bg-card",
          deleted && "border-destructive/40 bg-destructive/5 text-muted-foreground"
        )}
      >
        {!message.is_from_me && <p className="mb-1 text-[11px] font-semibold text-primary">{sender}</p>}
        {message.media_type && isAudio(message.media_type) && <AudioPreview />}
        {message.content ? (
          <p className="whitespace-pre-wrap break-words text-sm leading-relaxed">{message.content}</p>
        ) : (
          <p className="flex items-center gap-1.5 text-xs italic opacity-80">
            {message.media_type ? mediaIcon(message.media_type) : <MessageSquare className="size-3.5" />}
            {deleted ? "Mensagem apagada pelo remetente" : `Mídia: ${message.media_type || "sem conteúdo textual"}`}
          </p>
        )}
        <div className={cn("mt-1 flex items-center justify-end gap-1.5 text-[10px]", message.is_from_me ? "text-primary-foreground/70" : "text-muted-foreground")}>
          {message.is_edited && <span>editada</span>}
          {deleted && <span>apagada</span>}
          <span>{whenLabel(message.timestamp).split(", ").at(-1)}</span>
        </div>
      </div>
    </div>
  );
}

export function ConversationChat({
  conversations,
  selectedConversation,
  onSelectConversation,
  conversationsLoading,
  messages,
  messagesLoading,
  hasMore,
  onLoadOlder,
  loadingOlder,
}: ConversationChatProps) {
  return (
    <div className="grid h-[min(700px,calc(100vh-300px))] min-h-[520px] grid-rows-[minmax(10rem,0.38fr)_minmax(0,1fr)] overflow-hidden rounded-lg border border-border/80 bg-card lg:grid-cols-[18rem_minmax(0,1fr)] lg:grid-rows-1">
      <aside className="flex min-h-0 flex-col border-b border-border/70 lg:border-b-0 lg:border-r">
        <div className="flex items-center justify-between border-b border-border/70 px-3.5 py-3">
          <div>
            <p className="text-sm font-semibold">Conversas</p>
            <p className="text-[11px] text-muted-foreground">{conversations.length} encontradas</p>
          </div>
          <MessageCircle className="size-4 text-muted-foreground" />
        </div>
        <ScrollArea className="min-h-0 flex-1">
          <div className="space-y-0.5 p-1.5">
            {conversationsLoading ? (
              <div className="flex items-center justify-center gap-2 py-10 text-xs text-muted-foreground">
                <Loader2 className="size-4 animate-spin" /> Carregando conversas
              </div>
            ) : conversations.length === 0 ? (
              <div className="px-4 py-10 text-center">
                <MessageCircle className="mx-auto mb-2 size-5 text-muted-foreground" />
                <p className="text-xs text-muted-foreground">Nenhuma conversa encontrada.</p>
              </div>
            ) : (
              conversations.map((conversation) => {
                const selected = selectedConversation && conversationKey(selectedConversation) === conversationKey(conversation);
                return (
                  <button
                    key={conversationKey(conversation)}
                    type="button"
                    onClick={() => onSelectConversation(conversation)}
                    className={cn(
                      "flex w-full items-start gap-2.5 rounded-md px-2.5 py-2.5 text-left transition-colors",
                      selected ? "bg-accent text-accent-foreground" : "hover:bg-muted/60"
                    )}
                  >
                    <Avatar className="size-8 shrink-0 text-[10px]">
                      <AvatarFallback>{initials(displayName(conversation))}</AvatarFallback>
                    </Avatar>
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center justify-between gap-2">
                        <span className="truncate text-xs font-semibold">{displayName(conversation)}</span>
                        <span className="shrink-0 text-[10px] text-muted-foreground">
                          {new Date(conversation.last_message_time).toLocaleDateString("pt-BR", { day: "2-digit", month: "2-digit" })}
                        </span>
                      </span>
                      <span className="mt-0.5 flex items-center gap-1 text-[10px] text-muted-foreground">
                        {conversation.is_group && <Users className="size-3" />}
                        <span className="truncate">{conversation.last_is_from_me ? "Você: " : ""}{conversation.last_message || "Mídia"}</span>
                      </span>
                      <span className="mt-1 flex items-center gap-1 truncate text-[10px] text-muted-foreground/80">
                        <Smartphone className="size-3 shrink-0" />
                        {conversation.instance_alias || conversation.instance_jid || "Número não identificado"}
                      </span>
                    </span>
                  </button>
                );
              })
            )}
          </div>
        </ScrollArea>
      </aside>

      <section className="flex min-h-0 flex-col bg-muted/10">
        {selectedConversation ? (
          <>
            <header className="flex items-center gap-3 border-b border-border/70 bg-card px-4 py-3">
              <Avatar className="size-9 shrink-0">
                <AvatarFallback>{initials(displayName(selectedConversation))}</AvatarFallback>
              </Avatar>
              <div className="min-w-0">
                <p className="truncate text-sm font-semibold">{displayName(selectedConversation)}</p>
                <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
                  {selectedConversation.is_group && <span className="inline-flex items-center gap-1"><Users className="size-3" /> Grupo</span>}
                  <span className="truncate">{selectedConversation.instance_alias || selectedConversation.instance_jid}</span>
                  {selectedConversation.employee_name && <span className="hidden truncate sm:inline">· {selectedConversation.employee_name}</span>}
                </div>
              </div>
              <span className="ml-auto shrink-0 text-[11px] text-muted-foreground">
                {selectedConversation.message_count} mensagens
              </span>
            </header>

            <ScrollArea className="min-h-0 flex-1">
              <div className="mx-auto flex max-w-3xl flex-col gap-2 p-4">
                {hasMore && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={onLoadOlder}
                    disabled={loadingOlder}
                    className="mx-auto h-7 gap-1.5 text-xs text-muted-foreground"
                  >
                    {loadingOlder ? <Loader2 className="size-3.5 animate-spin" /> : <ChevronUp className="size-3.5" />}
                    Carregar mensagens anteriores
                  </Button>
                )}
                {messagesLoading ? (
                  <div className="flex flex-1 items-center justify-center gap-2 py-20 text-xs text-muted-foreground">
                    <Loader2 className="size-4 animate-spin" /> Carregando histórico
                  </div>
                ) : messages.length === 0 ? (
                  <div className="py-20 text-center text-xs text-muted-foreground">Esta conversa ainda não possui mensagens exibíveis.</div>
                ) : (
                  messages.map((message, index) => (
                    <div key={`${message.instance_jid}|${message.chat_jid}|${message.id}`}>
                      {(index === 0 || dateKey(messages[index - 1].timestamp) !== dateKey(message.timestamp)) && (
                        <div className="my-3 flex items-center justify-center">
                          <span className="rounded-full border border-border/70 bg-card px-2.5 py-1 text-[10px] text-muted-foreground">
                            {dateLabel(message.timestamp)}
                          </span>
                        </div>
                      )}
                      <MessageBubble message={message} />
                    </div>
                  ))
                )}
              </div>
            </ScrollArea>
            <footer className="flex items-center gap-2 border-t border-border/70 bg-card px-4 py-2.5 text-[11px] text-muted-foreground">
              <AudioLines className="size-3.5" />
              Visualização somente leitura. O envio de mensagens não está disponível nesta tela.
            </footer>
          </>
        ) : (
          <div className="flex flex-1 flex-col items-center justify-center px-6 text-center">
            <div className="mb-3 flex size-11 items-center justify-center rounded-full bg-muted text-muted-foreground">
              <MessageCircle className="size-5" />
            </div>
            <p className="text-sm font-medium">Selecione uma conversa</p>
            <CardDescription className="mt-1 max-w-xs text-xs">As mensagens capturadas serão exibidas em uma timeline agrupada por chat.</CardDescription>
          </div>
        )}
      </section>
    </div>
  );
}
