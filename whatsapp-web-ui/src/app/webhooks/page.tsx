"use client";

import { useEffect } from "react";
import { PageContainer } from "@/components/layout/page";
import { WebhookList } from "@/components/webhooks";
import { useSettings } from "@/lib/store";

export default function WebhooksPage() {
  const { darkMode } = useSettings();

  useEffect(() => {
    if (darkMode) {
      document.documentElement.classList.add("dark");
    } else {
      document.documentElement.classList.remove("dark");
    }
  }, [darkMode]);

  return (
    <PageContainer>
      <WebhookList />
    </PageContainer>
  );
}
