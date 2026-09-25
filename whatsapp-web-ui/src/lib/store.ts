"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";

interface SettingsState {
  darkMode: boolean;
  setDarkMode: (dark: boolean) => void;
}

// Only appearance is kept in the browser. Credentials are never stored client-side:
// the session lives on the server and reaches the browser as an HttpOnly cookie.
export const useSettings = create<SettingsState>()(
  persist(
    (set) => ({
      darkMode: false,
      setDarkMode: (darkMode) => set({ darkMode }),
    }),
    {
      name: "whatsapp-pairing-settings",
      version: 1,
      // v0 also persisted an `apiKey`; drop it so an old key doesn't linger in localStorage.
      migrate: (persisted) => ({ darkMode: Boolean((persisted as { darkMode?: boolean } | null)?.darkMode) }),
      partialize: (state) => ({ darkMode: state.darkMode }),
    }
  )
);

type AuthStatus = "checking" | "authed" | "anon";

interface AuthState {
  status: AuthStatus;
  username: string;
  setAuthed: (username: string) => void;
  setAnon: () => void;
}

// Mirrors what the server says about the current session. Holds no secret.
export const useAuth = create<AuthState>((set) => ({
  status: "checking",
  username: "",
  setAuthed: (username) => set({ status: "authed", username }),
  setAnon: () => set({ status: "anon", username: "" }),
}));

type PairingStep = "phone" | "code" | "dashboard";

interface PairingState {
  step: PairingStep;
  phoneNumber: string;
  pairingCode: string;
  expiresIn: number;
  jid: string;
  setStep: (step: PairingStep) => void;
  setPhoneNumber: (phone: string) => void;
  setPairingCode: (code: string, expiresIn: number) => void;
  setJid: (jid: string) => void;
  reset: () => void;
}

export const usePairing = create<PairingState>((set) => ({
  step: "phone",
  phoneNumber: "",
  pairingCode: "",
  expiresIn: 0,
  jid: "",
  setStep: (step) => set({ step }),
  setPhoneNumber: (phoneNumber) => set({ phoneNumber }),
  setPairingCode: (pairingCode, expiresIn) => set({ pairingCode, expiresIn }),
  setJid: (jid) => set({ jid }),
  reset: () =>
    set({
      step: "phone",
      phoneNumber: "",
      pairingCode: "",
      expiresIn: 0,
      jid: "",
    }),
}));
