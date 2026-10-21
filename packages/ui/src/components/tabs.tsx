import * as React from "react";
import { Tabs as T } from "radix-ui";
import { cn } from "../cn";

export const Tabs = T.Root;

export function TabsList({ className, ...props }: React.ComponentProps<typeof T.List>) {
  return <T.List className={cn("mx-auto my-3 inline-flex w-fit gap-0.5 rounded-pill bg-tag p-0.5", className)} {...props} />;
}

export function TabsTrigger({ className, ...props }: React.ComponentProps<typeof T.Trigger>) {
  return (
    <T.Trigger
      className={cn(
        "cursor-pointer rounded-pill px-3 py-1 text-caption text-fg-muted transition-colors hover:text-fg data-[state=active]:bg-bg data-[state=active]:text-fg data-[state=active]:shadow-card",
        className,
      )}
      {...props}
    />
  );
}

export function TabsContent({ className, ...props }: React.ComponentProps<typeof T.Content>) {
  return <T.Content className={cn("min-h-0 flex-1 overflow-y-auto", className)} {...props} />;
}
