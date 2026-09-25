import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import { Toaster } from "@/components/ui/sonner";
import { AuthGate } from "@/components/auth/auth-gate";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "WhatsApp MCP",
  description: "WhatsApp MCP by Matheus Pina: manage device pairing, sessions and webhooks",
  authors: [{ name: "Matheus Pina" }],
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className={geistSans.variable + " " + geistMono.variable + " antialiased"}>
        <AuthGate>{children}</AuthGate>
        <Toaster richColors position="top-right" />
      </body>
    </html>
  );
}
