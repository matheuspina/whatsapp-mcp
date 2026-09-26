"use client";

import { Link2, Webhook } from "lucide-react";
import { FeatureCard } from "@/components/common/feature-card";
import { PageContainer, PageHeader } from "@/components/layout/page";

export default function Home() {
  return (
    <PageContainer>
      <PageHeader
        title="WhatsApp MCP"
        description="Manage your WhatsApp bridge, device pairing, and webhook configurations"
      />
      <div className="grid gap-6 md:grid-cols-2">
        <FeatureCard
          icon={Link2}
          title="Device Pairing"
          description="Link your WhatsApp device"
          href="/pairing"
          cta="Go to Pairing"
        >
          Connect a new WhatsApp device using phone number pairing. Get an 8-digit code to enter on your phone.
        </FeatureCard>
        <FeatureCard
          icon={Webhook}
          title="Webhook Manager"
          description="Configure webhook endpoints"
          href="/webhooks"
          cta="Manage Webhooks"
          variant="outline"
        >
          Create and manage webhooks to receive WhatsApp messages. Set up triggers, view logs, and test your
          endpoints.
        </FeatureCard>
      </div>
    </PageContainer>
  );
}
