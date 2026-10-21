import * as React from "react";
import { Tabs as T } from "radix-ui";
import { cn } from "../cn";

export const Tabs = T.Root;

export function TabsList({ className, ...props }: React.ComponentProps<typeof T.List>) {
  return <T.List className={cn("flex gap-1 border-b border-border px-3", className)} {...props} />;
}

export function TabsTrigger({ className, ...props }: React.ComponentProps<typeof T.Trigger>) {
  return (
    <T.Trigger
      className={cn(
        "relative -mb-px cursor-pointer border-b-2 border-transparent px-2 py-2.5 text-body text-fg-muted hover:text-fg data-[state=active]:border-accent data-[state=active]:text-fg",
        className,
      )}
      {...props}
    />
  );
}

export function TabsContent({ className, ...props }: React.ComponentProps<typeof T.Content>) {
  return <T.Content className={cn("min-h-0 flex-1 overflow-y-auto", className)} {...props} />;
}
