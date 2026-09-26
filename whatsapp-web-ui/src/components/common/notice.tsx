import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

/** Tinted surface for a status: success, warning or destructive. Colors come from the theme tokens. */
const noticeVariants = cva("rounded-lg border p-4", {
  variants: {
    variant: {
      success: "border-success/50 bg-success/10",
      warning: "border-warning/50 bg-warning/10",
      destructive: "border-destructive/50 bg-destructive/10",
    },
  },
  defaultVariants: { variant: "warning" },
});

/** Text color that matches a notice variant, for its icon or bullet. */
const noticeToneVariants = cva("", {
  variants: {
    variant: {
      success: "text-success",
      warning: "text-warning",
      destructive: "text-destructive",
    },
  },
  defaultVariants: { variant: "warning" },
});

interface NoticeProps extends Omit<React.ComponentProps<"div">, "title">, VariantProps<typeof noticeVariants> {
  icon?: React.ComponentType<{ className?: string }>;
  title?: React.ReactNode;
}

function Notice({ variant, icon: Icon, title, className, children, ...props }: NoticeProps) {
  return (
    <div data-slot="notice" className={cn(noticeVariants({ variant }), className)} {...props}>
      {title && (
        <div className="mb-2 flex items-center gap-2 text-sm font-medium">
          {Icon && <Icon className={cn("size-4", noticeToneVariants({ variant }))} />}
          {title}
        </div>
      )}
      {children}
    </div>
  );
}

export { Notice, noticeVariants, noticeToneVariants };
