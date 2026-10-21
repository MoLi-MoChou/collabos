import * as React from "react";
import { cn } from "../cn";

const field =
  "w-full rounded-card border-0 bg-surface px-3 text-body text-fg shadow-card placeholder:text-fg-subtle focus:outline-none focus:shadow-ring";

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(function Input(
  { className, ...props },
  ref,
) {
  return <input ref={ref} className={cn(field, "h-9", className)} {...props} />;
});

export const Textarea = React.forwardRef<HTMLTextAreaElement, React.TextareaHTMLAttributes<HTMLTextAreaElement>>(
  function Textarea({ className, ...props }, ref) {
    return <textarea ref={ref} className={cn(field, "py-2 resize-none", className)} {...props} />;
  },
);

export function Label({ className, ...props }: React.LabelHTMLAttributes<HTMLLabelElement>) {
  return <label className={cn("text-caption font-medium text-fg-muted", className)} {...props} />;
}
