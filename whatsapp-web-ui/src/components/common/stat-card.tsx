import * as React from "react";
import { cn } from "@/lib/utils";

type StatColumns = 2 | 3 | 4;

interface StatCardProps extends React.ComponentProps<"div"> {
  icon: React.ReactNode;
  label: React.ReactNode;
  description?: React.ReactNode;
  /** Set `large` to false for longer values such as dates. */
  large?: boolean;
  valueClassName?: string;
  value: React.ReactNode;
}

interface StatGridProps extends React.ComponentProps<"div"> {
  columns?: StatColumns;
}

const gridVariants: Record<StatColumns, string> = {
  2: "grid-cols-2 [&>*:nth-child(odd)]:border-r [&>*:nth-child(-n+2)]:border-b",
  3: "grid-cols-1 sm:grid-cols-3 [&>*:not(:last-child)]:border-b sm:[&>*]:border-b-0 sm:[&>*:not(:nth-child(3n))]:border-r",
  4: "grid-cols-2 [&>*:nth-child(odd)]:border-r [&>*:nth-child(-n+2)]:border-b sm:grid-cols-4 sm:[&>*]:border-b-0 sm:[&>*:not(:nth-child(4n))]:border-r",
};

/** Compact, consistent container for page-level metrics. */
function StatGrid({ columns = 4, className, ...props }: StatGridProps) {
  return (
    <div
      className={cn(
        "grid overflow-hidden rounded-lg border border-border/70 bg-card [&>*]:border-border/70",
        gridVariants[columns],
        className
      )}
      {...props}
    />
  );
}

/** A compact metric with an icon, value and optional supporting text. */
function StatCard({
  icon,
  label,
  value,
  description,
  large = true,
  valueClassName,
  className,
  children,
  ...props
}: StatCardProps) {
  return (
    <div className={cn("flex min-w-0 items-center gap-2.5 px-3 py-2.5", className)} {...props}>
      <span className="flex shrink-0 items-center justify-center text-muted-foreground [&>svg]:size-4">
        {icon}
      </span>
      <div className="min-w-0">
        <p className="truncate text-[11px] text-muted-foreground">{label}</p>
        <p className={cn(large ? "text-lg font-semibold leading-none" : "text-xs font-medium leading-tight", valueClassName)}>
          {value}
        </p>
        {description && <p className="mt-1 truncate text-[11px] text-muted-foreground">{description}</p>}
        {children}
      </div>
    </div>
  );
}

export { StatCard, StatGrid };
