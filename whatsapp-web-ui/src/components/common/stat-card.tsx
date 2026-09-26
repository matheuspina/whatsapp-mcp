import * as React from "react";
import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/lib/utils";

interface StatCardProps extends Omit<React.ComponentProps<typeof Card>, "title"> {
  icon: React.ReactNode;
  label: React.ReactNode;
  /** Set `large` for headline numbers, leave it off for longer values such as dates. */
  large?: boolean;
  value: React.ReactNode;
}

/** A labelled metric: small icon and label on top, the value below, optional extra content (a progress bar). */
function StatCard({ icon, label, value, large = true, className, children, ...props }: StatCardProps) {
  return (
    <Card className={cn("bg-muted/50", className)} {...props}>
      <CardContent>
        <div className="mb-2 flex items-center gap-2 text-sm font-medium [&>svg]:size-4">
          {icon}
          {label}
        </div>
        <div className={large ? "text-2xl font-semibold" : "text-sm font-medium"}>{value}</div>
        {children}
      </CardContent>
    </Card>
  );
}

export { StatCard };
