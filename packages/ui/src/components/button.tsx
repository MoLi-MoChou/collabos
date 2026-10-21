import * as React from "react";
import { Slot } from "radix-ui";
import { cn } from "../cn";

const variants = {
  primary: "bg-accent text-accent-fg hover:opacity-90",
  secondary: "bg-surface text-fg border border-border hover:bg-surface-2",
  ghost: "bg-transparent text-fg-muted hover:bg-surface-2 hover:text-fg",
  danger: "bg-danger text-accent-fg hover:opacity-90",
  success: "bg-success text-accent-fg hover:opacity-90",
} as const;

const sizes = {
  sm: "h-7 px-2.5 text-caption gap-1",
  md: "h-9 px-3.5 text-body gap-1.5",
  icon: "h-8 w-8 justify-center",
} as const;

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: keyof typeof variants;
  size?: keyof typeof sizes;
  asChild?: boolean;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = "secondary", size = "md", asChild, className, ...props },
  ref,
) {
  const Comp = asChild ? Slot.Root : "button";
  return (
    <Comp
      ref={ref}
      className={cn(
        "inline-flex items-center rounded-control font-medium transition-colors disabled:pointer-events-none disabled:opacity-50 cursor-pointer select-none whitespace-nowrap",
        variants[variant],
        sizes[size],
        className,
      )}
      {...props}
    />
  );
});
