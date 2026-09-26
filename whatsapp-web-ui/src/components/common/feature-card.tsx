import * as React from "react";
import Link from "next/link";
import { ArrowRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

interface FeatureCardProps {
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  description: string;
  href: string;
  cta: string;
  /** Use `outline` for secondary destinations so only one card is emphasised. */
  variant?: "default" | "outline";
  children: React.ReactNode;
}

/** Overview card that links to a section of the panel. */
function FeatureCard({ icon: Icon, title, description, href, cta, variant = "default", children }: FeatureCardProps) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="rounded-lg bg-primary/10 p-2 text-primary">
            <Icon className="size-5" />
          </div>
          <div className="space-y-1">
            <CardTitle>{title}</CardTitle>
            <CardDescription>{description}</CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">{children}</p>
        <Button asChild variant={variant} className="w-full">
          <Link href={href}>
            {cta}
            <ArrowRight />
          </Link>
        </Button>
      </CardContent>
    </Card>
  );
}

export { FeatureCard };
