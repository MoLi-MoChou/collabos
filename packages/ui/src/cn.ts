import { clsx, type ClassValue } from "clsx";

/** Joins class names. Only token classes are allowed (see themes/). */
export function cn(...v: ClassValue[]): string {
  return clsx(v);
}
