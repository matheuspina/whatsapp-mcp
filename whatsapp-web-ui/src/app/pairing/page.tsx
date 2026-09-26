"use client";

import { useState, useEffect } from "react";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { PhoneInput, CodeDisplay, Dashboard, SettingsDialog } from "@/components/pairing";
import { usePairing, useSettings } from "@/lib/store";
import { WhatsAppAPI } from "@/lib/api";

export default function PairingPage() {
  const { step, setStep, setJid } = usePairing();
  const { darkMode } = useSettings();
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [initialized, setInitialized] = useState(false);

  useEffect(() => {
    if (darkMode) {
      document.documentElement.classList.add("dark");
    } else {
      document.documentElement.classList.remove("dark");
    }
  }, [darkMode]);

  useEffect(() => {
    const checkExistingConnection = async () => {
      try {
        const api = new WhatsAppAPI();
        const status = await api.getConnectionStatus();
        if (status.success && status.linked && status.jid) {
          setJid(status.jid);
          setStep("dashboard");
        }
      } catch {
        console.log("No existing connection");
      } finally {
        setInitialized(true);
      }
    };

    checkExistingConnection();
  }, [setJid, setStep]);

  if (!initialized) {
    return (
      <div className="flex items-center justify-center min-h-[80vh]">
        <div className="text-muted-foreground">Carregando...</div>
      </div>
    );
  }

  return (
    <PageContainer>
        <PageHeader title="Conectar aparelho" />
        <div className="flex items-center justify-between rounded-lg border border-primary/20 bg-primary/5 p-3.5 text-xs text-foreground">
          <span>💡 <strong>Múltiplos números disponíveis:</strong> Você pode conectar e gerenciar múltiplos aparelhos simultâneos.</span>
          <a href="/instances" className="font-semibold text-primary underline ml-2 shrink-0">
            Ver aparelhos conectados &rarr;
          </a>
        </div>
        {step === "phone" && <PhoneInput />}
        {step === "code" && <CodeDisplay />}
        {step === "dashboard" && <Dashboard onOpenSettings={() => setSettingsOpen(true)} />}

        <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
    </PageContainer>
  );
}
