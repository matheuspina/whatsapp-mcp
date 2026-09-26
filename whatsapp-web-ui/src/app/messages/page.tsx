"use client";

import { useEffect, useState, useMemo, useCallback } from "react";
import {
  MessageSquare,
  Search,
  Loader2,
  User,
  Users,
  Smartphone,
  RefreshCw,
  Clock,
  ShieldAlert,
} from "lucide-react";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { WhatsAppAPI, ChatItem, MessageItem } from "@/lib/api";

export default function MessagesPage() {
  const [chats, setChats] = useState<ChatItem[]>([]);
  const [selectedChat, setSelectedChat] = useState<ChatItem | null>(null);
  const [messages, setMessages] = useState<MessageItem[]>([]);
  const [loadingChats, setLoadingChats] = useState(true);
  const [loadingMessages, setLoadingMessages] = useState(false);
  const [chatSearch, setChatSearch] = useState("");
  const [messageSearch, setMessageSearch] = useState("");

  const api = useMemo(() => new WhatsAppAPI(), []);

  const loadChats = useCallback(async () => {
    try {
      const data = await api.getChats();
      data.sort((a, b) => new Date(b.last_message_time).getTime() - new Date(a.last_message_time).getTime());
      setChats(data);
      if (data.length > 0 && !selectedChat) {
        setSelectedChat(data[0]);
      }
    } catch (err: unknown) {
      toast.error("Erro ao carregar conversas", {
        description: err instanceof Error ? err.message : "Falha na comunicação",
      });
    } finally {
      setLoadingChats(false);
    }
  }, [api, selectedChat]);

  const loadMessages = useCallback(async (chatJid: string) => {
    try {
      setLoadingMessages(true);
      const data = await api.getMessages(chatJid, 150);
      setMessages(data);
    } catch (err: unknown) {
      toast.error("Erro ao carregar mensagens", {
        description: err instanceof Error ? err.message : "Falha na comunicação",
      });
    } finally {
      setLoadingMessages(false);
    }
  }, [api]);

  useEffect(() => {
    let ignore = false;
    api.getChats().then((data) => {
      if (!ignore) {
        data.sort((a, b) => new Date(b.last_message_time).getTime() - new Date(a.last_message_time).getTime());
        setChats(data);
        if (data.length > 0) {
          setSelectedChat(data[0]);
        }
        setLoadingChats(false);
      }
    }).catch(() => {
      if (!ignore) {
        setLoadingChats(false);
      }
    });
    return () => { ignore = true; };
  }, [api]);

  useEffect(() => {
    if (selectedChat) {
      let ignore = false;
      api.getMessages(selectedChat.jid, 150).then((data) => {
        if (!ignore) {
          setMessages(data);
          setLoadingMessages(false);
        }
      }).catch(() => {
        if (!ignore) {
          setLoadingMessages(false);
        }
      });
      return () => { ignore = true; };
    }
  }, [selectedChat, api]);

  // Filtered chat list
  const filteredChats = useMemo(() => {
    return chats.filter((c) => {
      const q = chatSearch.toLowerCase();
      return (
        c.jid.toLowerCase().includes(q) ||
        (c.name && c.name.toLowerCase().includes(q))
      );
    });
  }, [chats, chatSearch]);

  // Filtered messages list
  const filteredMessages = useMemo(() => {
    if (!messageSearch) return messages;
    const q = messageSearch.toLowerCase();
    return messages.filter(
      (m) =>
        (m.content && m.content.toLowerCase().includes(q)) ||
        (m.sender && m.sender.toLowerCase().includes(q)) ||
        (m.sender_name && m.sender_name.toLowerCase().includes(q))
    );
  }, [messages, messageSearch]);

  const deletedCount = useMemo(() => {
    return messages.filter((m) => m.is_deleted_remote).length;
  }, [messages]);

  return (
    <PageContainer>
      <PageHeader
        title="Mensagens & Auditoria"
        description="Monitoramento centralizado de mensagens capturadas em todas as instâncias ativas com governança anti-delete."
        actions={
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              loadChats();
              if (selectedChat) loadMessages(selectedChat.jid);
            }}
            className="gap-2"
          >
            <RefreshCw className="size-3.5" />
            Atualizar
          </Button>
        }
      />

      <div className="grid grid-cols-1 md:grid-cols-12 gap-4 h-[calc(100vh-210px)] min-h-[500px]">
        {/* Left column: Chats list */}
        <div className="md:col-span-4 lg:col-span-4 flex flex-col border rounded-lg bg-card overflow-hidden">
          <div className="p-3 border-b space-y-2">
            <div className="relative">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
              <Input
                placeholder="Buscar conversa ou telefone..."
                className="pl-8 text-xs h-9"
                value={chatSearch}
                onChange={(e) => setChatSearch(e.target.value)}
              />
            </div>
            <div className="flex items-center justify-between text-xs text-muted-foreground px-1">
              <span>{filteredChats.length} conversas</span>
            </div>
          </div>

          <div className="flex-1 overflow-y-auto divide-y">
            {loadingChats ? (
              <div className="flex h-32 items-center justify-center">
                <Loader2 className="size-6 animate-spin text-muted-foreground" />
              </div>
            ) : filteredChats.length === 0 ? (
              <div className="p-6 text-center text-xs text-muted-foreground">
                Nenhuma conversa encontrada.
              </div>
            ) : (
              filteredChats.map((chat) => {
                const isSelected = selectedChat?.jid === chat.jid;
                return (
                  <button
                    key={chat.jid}
                    onClick={() => setSelectedChat(chat)}
                    className={`w-full text-left p-3 transition-colors hover:bg-muted/50 flex items-start gap-3 ${
                      isSelected ? "bg-muted border-l-2 border-primary" : ""
                    }`}
                  >
                    <div className="flex size-9 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary mt-0.5">
                      {chat.is_group ? <Users className="size-4" /> : <User className="size-4" />}
                    </div>

                    <div className="flex-1 min-w-0">
                      <div className="flex items-center justify-between gap-1">
                        <span className="font-semibold text-xs truncate">
                          {chat.name || chat.jid.split("@")[0]}
                        </span>
                        <span className="text-[10px] text-muted-foreground shrink-0">
                          {new Date(chat.last_message_time).toLocaleDateString("pt-BR", {
                            day: "2-digit",
                            month: "2-digit",
                          })}
                        </span>
                      </div>

                      <p className="text-[11px] text-muted-foreground truncate font-mono mt-0.5">
                        {chat.jid}
                      </p>

                      {chat.instance_jid && (
                        <div className="mt-1 flex items-center gap-1">
                          <Badge variant="outline" className="text-[9px] px-1 py-0 gap-1 font-mono">
                            <Smartphone className="size-2.5" />
                            {chat.instance_jid.split("@")[0]}
                          </Badge>
                        </div>
                      )}
                    </div>
                  </button>
                );
              })
            )}
          </div>
        </div>

        {/* Right column: Message Thread */}
        <div className="md:col-span-8 lg:col-span-8 flex flex-col border rounded-lg bg-card overflow-hidden">
          {selectedChat ? (
            <>
              {/* Chat Thread Header */}
              <div className="p-3 border-b flex flex-col sm:flex-row sm:items-center justify-between gap-2 bg-muted/20">
                <div className="flex items-center gap-2.5 min-w-0">
                  <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
                    {selectedChat.is_group ? <Users className="size-4" /> : <User className="size-4" />}
                  </div>
                  <div className="min-w-0">
                    <h3 className="font-semibold text-sm truncate">
                      {selectedChat.name || selectedChat.jid}
                    </h3>
                    <p className="text-[11px] text-muted-foreground font-mono truncate">
                      {selectedChat.jid}
                    </p>
                  </div>
                </div>

                <div className="flex items-center gap-2">
                  {deletedCount > 0 && (
                    <Badge variant="destructive" className="gap-1 text-[11px] py-0.5">
                      <ShieldAlert className="size-3" />
                      {deletedCount} {deletedCount === 1 ? "apagada (auditada)" : "apagadas (auditadas)"}
                    </Badge>
                  )}
                  <div className="relative w-48">
                    <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground" />
                    <Input
                      placeholder="Filtrar mensagens..."
                      className="pl-8 text-xs h-8"
                      value={messageSearch}
                      onChange={(e) => setMessageSearch(e.target.value)}
                    />
                  </div>
                </div>
              </div>

              {/* Message Bubbles Container */}
              <div className="flex-1 overflow-y-auto p-4 space-y-3 bg-muted/5">
                {loadingMessages ? (
                  <div className="flex h-48 items-center justify-center">
                    <Loader2 className="size-6 animate-spin text-muted-foreground" />
                  </div>
                ) : filteredMessages.length === 0 ? (
                  <div className="flex flex-col items-center justify-center h-48 text-muted-foreground text-xs">
                    <MessageSquare className="size-8 mb-2 opacity-50" />
                    <span>Nenhuma mensagem registrada nesta conversa.</span>
                  </div>
                ) : (
                  filteredMessages.map((msg) => {
                    const isFromMe = msg.is_from_me;
                    const isDeleted = msg.is_deleted_remote;

                    return (
                      <div
                        key={msg.id}
                        className={`flex flex-col ${isFromMe ? "items-end" : "items-start"}`}
                      >
                        <div
                          className={`max-w-[85%] sm:max-w-[70%] rounded-xl px-3.5 py-2.5 text-xs shadow-xs transition-all ${
                            isDeleted
                              ? "border-2 border-destructive bg-destructive/10 text-destructive-foreground"
                              : isFromMe
                              ? "bg-primary text-primary-foreground"
                              : "bg-card border text-card-foreground"
                          }`}
                        >
                          {/* Deleted Anti-Delete Banner */}
                          {isDeleted && (
                            <div className="flex items-center gap-1.5 pb-1.5 mb-1.5 border-b border-destructive/20 text-destructive font-semibold text-[11px]">
                              <ShieldAlert className="size-3.5 shrink-0" />
                              <span>Mensagem apagada no WhatsApp (Preservada pelo Anti-Delete)</span>
                            </div>
                          )}

                          {/* Sender name in group */}
                          {!isFromMe && msg.sender_name && (
                            <p className="font-semibold text-[11px] text-primary mb-1">
                              {msg.sender_name}
                            </p>
                          )}

                          {/* Message Content */}
                          <p className="whitespace-pre-wrap leading-relaxed select-text font-normal">
                            {msg.content || (msg.media_type ? `[Arquivo de Mídia: ${msg.media_type}]` : "")}
                          </p>

                          {/* Footer: Time + Instance badge */}
                          <div
                            className={`flex items-center justify-end gap-1.5 mt-1.5 text-[10px] ${
                              isFromMe ? "text-primary-foreground/75" : "text-muted-foreground"
                            }`}
                          >
                            <Clock className="size-2.5" />
                            <span>
                              {new Date(msg.timestamp).toLocaleTimeString("pt-BR", {
                                hour: "2-digit",
                                minute: "2-digit",
                              })}
                            </span>
                            {msg.instance_jid && (
                              <span className="font-mono opacity-60">
                                • {msg.instance_jid.split("@")[0].slice(-4)}
                              </span>
                            )}
                          </div>
                        </div>
                      </div>
                    );
                  })
                )}
              </div>
            </>
          ) : (
            <div className="flex flex-col items-center justify-center h-full text-muted-foreground text-xs space-y-2">
              <MessageSquare className="size-10 opacity-30" />
              <p>Selecione uma conversa ao lado para visualizar as mensagens.</p>
            </div>
          )}
        </div>
      </div>
    </PageContainer>
  );
}
