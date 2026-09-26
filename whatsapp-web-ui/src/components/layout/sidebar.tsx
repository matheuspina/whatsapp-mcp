"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import {
  ChevronsUpDown,
  LayoutDashboard,
  Link2,
  LogOut,
  MessageSquare,
  Plug,
  Settings,
  User,
  Webhook,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { WhatsAppAPI } from "@/lib/api";
import { useAuth } from "@/lib/store";
import {
  Sidebar as SidebarPrimitive,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const navItems = [
  {
    title: "Overview",
    href: "/",
    icon: LayoutDashboard,
    description: "System overview",
  },
  {
    title: "Pairing",
    href: "/pairing",
    icon: Link2,
    description: "Link WhatsApp device",
  },
  {
    title: "Webhooks",
    href: "/webhooks",
    icon: Webhook,
    description: "Manage webhook endpoints",
  },
  {
    title: "MCP Clients",
    href: "/mcp-clients",
    icon: Plug,
    description: "Connect AI clients",
  },
];

type ConnectionDotStatus = "connected" | "disconnected" | "unknown";

export function Sidebar({ ...props }: React.ComponentProps<typeof SidebarPrimitive>) {
  const pathname = usePathname();
  const { username, setAnon } = useAuth();
  const [connStatus, setConnStatus] = useState<ConnectionDotStatus>("unknown");

  useEffect(() => {
    let cancelled = false;
    const checkStatus = () => {
      new WhatsAppAPI()
        .getConnectionStatus()
        .then((status) => {
          if (!cancelled) {
            setConnStatus(status.connected ? "connected" : "disconnected");
          }
        })
        .catch(() => {
          if (!cancelled) {
            setConnStatus("unknown");
          }
        });
    };

    checkStatus();
    const interval = setInterval(checkStatus, 10000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, []);

  const handleLogout = async () => {
    try {
      await new WhatsAppAPI().logout();
    } finally {
      setAnon();
    }
  };

  const dotColor = {
    connected: "bg-green-500",
    disconnected: "bg-red-500",
    unknown: "bg-yellow-500",
  }[connStatus];

  const statusLabel = {
    connected: "Connected",
    disconnected: "Disconnected",
    unknown: "Checking...",
  }[connStatus];

  return (
    <SidebarPrimitive collapsible="icon" {...props}>
      <SidebarHeader className="h-14 justify-center border-b px-2 py-0">
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild tooltip="WhatsApp MCP">
              <Link href="/">
                <div className="relative flex aspect-square size-8 items-center justify-center rounded-lg bg-green-500/10 text-green-600 dark:text-green-500">
                  <MessageSquare className="size-5" />
                  <span
                    className={cn(
                      "absolute -top-0.5 -right-0.5 size-2.5 rounded-full border-2 border-sidebar",
                      dotColor
                    )}
                    title={statusLabel}
                  />
                </div>
                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-semibold">WhatsApp MCP</span>
                  <span className="truncate text-xs text-muted-foreground">by Matheus Pina</span>
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Menu</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {navItems.map((item) => {
                const isActive =
                  item.href === "/"
                    ? pathname === "/"
                    : pathname === item.href || pathname.startsWith(item.href + "/");
                return (
                  <SidebarMenuItem key={item.href}>
                    <SidebarMenuButton asChild isActive={isActive} tooltip={item.title}>
                      <Link href={item.href}>
                        <item.icon />
                        <span>{item.title}</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              asChild
              isActive={pathname === "/settings" || pathname.startsWith("/settings/")}
              tooltip="Settings"
            >
              <Link href="/settings">
                <Settings />
                <span>Settings</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>

          <SidebarMenuItem>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <SidebarMenuButton
                  size="lg"
                  className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
                >
                  <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                    <User className="size-4" />
                  </div>
                  <div className="grid flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-semibold">{username || "User"}</span>
                    <span className="truncate text-xs text-muted-foreground flex items-center gap-1.5">
                      <span className={cn("inline-block size-1.5 rounded-full", dotColor)} />
                      {statusLabel}
                    </span>
                  </div>
                  <ChevronsUpDown className="ml-auto size-4" />
                </SidebarMenuButton>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                className="w-(--radix-dropdown-menu-trigger-width) min-w-56 rounded-lg"
                side="top"
                align="end"
                sideOffset={4}
              >
                <DropdownMenuLabel className="p-0 font-normal">
                  <div className="flex items-center gap-2 px-1 py-1.5 text-left text-sm">
                    <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                      <User className="size-4" />
                    </div>
                    <div className="grid flex-1 text-left text-sm leading-tight">
                      <span className="truncate font-semibold">{username || "User"}</span>
                      <span className="truncate text-xs text-muted-foreground flex items-center gap-1.5">
                        <span className={cn("inline-block size-1.5 rounded-full", dotColor)} />
                        {statusLabel}
                      </span>
                    </div>
                  </div>
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem asChild>
                  <Link href="/settings" className="cursor-pointer">
                    <Settings className="mr-2 size-4" />
                    Settings
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onClick={handleLogout}
                  className="cursor-pointer text-destructive focus:text-destructive"
                >
                  <LogOut className="mr-2 size-4" />
                  Sign out
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>

      <SidebarRail />
    </SidebarPrimitive>
  );
}
