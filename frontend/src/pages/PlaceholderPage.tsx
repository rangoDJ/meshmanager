import { Construction } from "lucide-react";

export function PlaceholderPage({
  title,
  text,
  milestone,
}: {
  title: string;
  text: string;
  milestone: number;
}) {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">{title}</h1>
      <div className="flex flex-col items-center rounded-xl border border-dashed border-border px-6 py-16 text-center">
        <Construction className="mb-3 size-8 text-muted" aria-hidden />
        <p className="max-w-md text-sm text-muted">{text}</p>
        <p className="mt-2 text-xs text-muted">Planned for milestone {milestone}.</p>
      </div>
    </div>
  );
}
