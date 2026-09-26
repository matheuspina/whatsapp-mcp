import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const pageContainerVariants = cva("flex flex-col gap-6 p-4 md:p-6", {
  variants: {
    size: {
      full: "",
      narrow: "max-w-3xl",
    },
  },
  defaultVariants: { size: "full" },
});

/** Outer wrapper of every page: padding, vertical rhythm and (optionally) a readable max width. */
function PageContainer({
  className,
  size,
  ...props
}: React.ComponentProps<"div"> & VariantProps<typeof pageContainerVariants>) {
  return <div data-slot="page-container" className={cn(pageContainerVariants({ size }), className)} {...props} />;
}

interface PageHeaderProps extends Omit<React.ComponentProps<"div">, "title"> {
  title: React.ReactNode;
  description?: React.ReactNode;
  /** Buttons shown on the right of the title. */
  actions?: React.ReactNode;
}

/** Title, description and actions at the top of a page. The one place page typography is defined. */
function PageHeader({ title, description, actions, className, ...props }: PageHeaderProps) {
  return (
    <div data-slot="page-header" className={cn("flex items-center justify-between gap-4", className)} {...props}>
      <div className="space-y-1">
        <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 gap-2">{actions}</div>}
    </div>
  );
}

export { PageContainer, PageHeader };
