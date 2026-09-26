"use client";

import {
  Smartphone,
  MessageSquare,
  Building2,
  Users,
  Webhook,
  Plug,
} from "lucide-react";
import { FeatureCard } from "@/components/common/feature-card";
import { PageContainer, PageHeader } from "@/components/layout/page";

export default function Home() {
  return (
    <PageContainer>
      <PageHeader
        title="Central de Mensagens & Governança WhatsApp"
        description="Gestão de instâncias multi-aparelho, organização por departamentos e contexto para inteligência artificial."
      />
      <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
        <FeatureCard
          icon={Smartphone}
          title="Instâncias WhatsApp"
          description="Aparelhos conectados"
          href="/instances"
          cta="Gerenciar Instâncias"
        >
          Vincule e monitore múltiplos aparelhos corporativos via QR Code oficial, com operação passiva e baixo risco.
        </FeatureCard>

        <FeatureCard
          icon={MessageSquare}
          title="Mensagens & Auditoria"
          description="Feed centralizado e anti-delete"
          href="/messages"
          cta="Acessar Auditoria"
          variant="outline"
        >
          Consulte o histórico unificado de conversas com identificação de mensagens apagadas para conformidade legal.
        </FeatureCard>

        <FeatureCard
          icon={Building2}
          title="Setores da Empresa"
          description="Departamentos organizacionais"
          href="/departments"
          cta="Gerenciar Setores"
          variant="outline"
        >
          Cadastre setores como Comercial, Atendimento e Financeiro para segmentação de conversas e contexto de IA.
        </FeatureCard>

        <FeatureCard
          icon={Users}
          title="Colaboradores"
          description="Membros da equipe"
          href="/employees"
          cta="Gerenciar Equipe"
          variant="outline"
        >
          Vincule colaboradores aos seus respectivos setores e cargos para desambiguação de nomes pela IA.
        </FeatureCard>

        <FeatureCard
          icon={Webhook}
          title="Webhooks"
          description="Disparo de eventos em tempo real"
          href="/webhooks"
          cta="Configurar Webhooks"
          variant="outline"
        >
          Configure endpoints externos para receber notificações instantâneas com filtros por palavra-chave ou remetente.
        </FeatureCard>

        <FeatureCard
          icon={Plug}
          title="Clientes MCP & IA"
          description="Integração Claude & Cursor"
          href="/mcp-clients"
          cta="Ver Configurações"
          variant="outline"
        >
          Conecte ferramentas de IA como Claude Desktop, Cursor e agentes locais para consultar histórico e contexto.
        </FeatureCard>
      </div>
    </PageContainer>
  );
}
