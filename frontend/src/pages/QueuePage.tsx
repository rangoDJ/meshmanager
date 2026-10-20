import {
  ArrowDown,
  ArrowUp,
  Check,
  Download,
  Ellipsis,
  ListPlus,
  Minus,
  Pencil,
  Play,
  Plus,
  Printer,
  RotateCcw,
  Trash2,
  Undo2,
} from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router";
import {
  type PrintItem,
  type PrintList,
  type PrintStatus,
  useClearDone,
  useCreatePrintList,
  useDeletePrintItem,
  useDeletePrintList,
  usePrintItems,
  usePrintLists,
  useUpdatePrintItem,
  useUpdatePrintList,
} from "../api/organize";
import { Dialog } from "../components/Dialog";
import { FileOverlays } from "../components/FileResults";
import { Thumbnail } from "../components/Thumbnail";
import { Button, ErrorText, Spinner } from "../components/ui";
import { formatDims, formatNumber } from "../lib/format";

// On a narrow screen the columns stack, with what is on the printer first.
const COLUMNS: { status: PrintStatus; title: string; empty: string; order: string }[] = [
  { status: "todo", title: "To print", empty: "Nothing waiting.", order: "order-2 @2xl:order-none" },
  { status: "printing", title: "Printing", empty: "Nothing on the printer.", order: "order-1 @2xl:order-none" },
  { status: "done", title: "Done", empty: "Nothing finished yet.", order: "order-3 @2xl:order-none" },
];

/** The selected list lives in the URL: ?list=<id>, or ?list=new. */
export function QueuePage() {
  const [params, setParams] = useSearchParams();
  const { data: lists = [], isPending, error } = usePrintLists();
  const param = params.get("list");
  const choose = (v: string | null) => setParams(v ? { list: v } : {});

  const picked = param && /^\d+$/.test(param) ? lists.find((l) => l.id === Number(param)) : undefined;
  const busiest = lists.find((l) => l.todo + l.printing > 0) ?? lists[0];
  const current = param === "new" ? undefined : (picked ?? (param === null ? busiest : undefined));
  const mode = param === "new" ? "new" : current ? "list" : "intro";

  // Lists are grouped by the printer they are for.
  const groups = lists.reduce<Map<string, PrintList[]>>((m, l) => {
    const key = l.printer || "";
    m.set(key, [...(m.get(key) ?? []), l]);
    return m;
  }, new Map());

  return (
    <div className="space-y-6 print:space-y-0">
      <div className="print:hidden">
        <h1 className="text-2xl font-semibold">Print queue</h1>
        <p className="mt-1 text-sm text-muted">
          Lists of what to print next, for each printer or each run. Add models from the library or a file's panel.
        </p>
      </div>

      <div className="gap-6 lg:grid lg:grid-cols-[16rem_minmax(0,1fr)] print:block">
        <nav aria-label="Print lists" className="mb-4 lg:mb-0 print:hidden">
          <div className="flex gap-2 lg:hidden">
            <select
              value={current ? String(current.id) : (param ?? "")}
              onChange={(e) => choose(e.target.value || null)}
              aria-label="Print list"
              className="min-w-0 flex-1 rounded-lg border border-border bg-surface px-3 py-2.5 text-sm"
            >
              {!current && <option value="">Choose…</option>}
              {lists.map((l) => (
                <option key={l.id} value={l.id}>
                  {l.printer ? `${l.printer}: ` : ""}
                  {l.name}
                </option>
              ))}
            </select>
            <Button variant="secondary" onClick={() => choose("new")}>
              <ListPlus className="size-4" aria-hidden /> New
            </Button>
          </div>

          <div className="hidden space-y-4 lg:block">
            {isPending && (
              <div className="space-y-1.5 px-2" aria-label="Loading lists">
                {[80, 60].map((w) => (
                  <div key={w} className="h-4 animate-pulse rounded bg-surface-2" style={{ width: `${w}%` }} />
                ))}
              </div>
            )}
            {[...groups.entries()].map(([printer, ls]) => (
              <section key={printer || "none"} aria-label={printer || "Lists without a printer"}>
                <h2 className="px-2 pb-1 text-xs font-medium text-muted">{printer || "No printer"}</h2>
                <ul className="space-y-0.5 text-sm">
                  {ls.map((l) => {
                    const active = mode === "list" && current?.id === l.id;
                    return (
                      <li key={l.id}>
                        <button
                          onClick={() => choose(String(l.id))}
                          aria-current={active ? "page" : undefined}
                          className={`flex min-h-9 w-full items-center gap-2 rounded-md px-2 py-1 text-left transition focus-visible:outline-2 focus-visible:outline-accent ${
                            active ? "bg-accent/10 font-medium text-accent" : "text-muted hover:bg-surface-2 hover:text-fg"
                          }`}
                        >
                          <span className="truncate">{l.name}</span>
                          <span className="ml-auto pl-2 text-xs tabular-nums opacity-70" title="Waiting or printing">
                            {formatNumber(l.todo + l.printing)}
                          </span>
                        </button>
                      </li>
                    );
                  })}
                </ul>
              </section>
            ))}
            <Button variant="secondary" className="w-full" onClick={() => choose("new")}>
              <ListPlus className="size-4" aria-hidden /> New list
            </Button>
          </div>
        </nav>

        <div className="@container min-w-0">
          <ErrorText>{error?.message}</ErrorText>
          {isPending ? (
            <Spinner />
          ) : mode === "new" ? (
            <NewList printers={[...groups.keys()].filter(Boolean)} onCreated={(id) => choose(String(id))} onCancel={() => choose(null)} />
          ) : mode === "list" && current ? (
            <ListView key={current.id} list={current} printers={[...groups.keys()].filter(Boolean)} onGone={() => choose(null)} />
          ) : (
            <div className="rounded-xl border border-dashed border-border px-6 py-14 text-center">
              <p className="font-medium">No print lists yet</p>
              <p className="mx-auto mt-1 max-w-md text-sm text-muted">
                Make a list for a print run or a printer, then add models to it from the{" "}
                <Link to="/library" className="text-accent underline-offset-2 hover:underline">
                  library
                </Link>
                . Each model moves from To print to Printing to Done as you go.
              </p>
              <Button className="mt-4" onClick={() => choose("new")}>
                <ListPlus className="size-4" aria-hidden /> Make a list
              </Button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function PrinterField({ value, onChange, printers }: { value: string; onChange: (v: string) => void; printers: string[] }) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-sm font-medium">Printer (optional)</span>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        list="known-printers"
        placeholder="For example Bambu P1S"
        maxLength={100}
        className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-accent focus:ring-2 focus:ring-accent/25"
      />
      <datalist id="known-printers">
        {printers.map((p) => (
          <option key={p} value={p} />
        ))}
      </datalist>
    </label>
  );
}

function NewList({ printers, onCreated, onCancel }: { printers: string[]; onCreated: (id: number) => void; onCancel: () => void }) {
  const create = useCreatePrintList();
  const [name, setName] = useState("");
  const [printer, setPrinter] = useState("");
  return (
    <form
      className="max-w-md space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (name.trim()) create.mutate({ name, printer }, { onSuccess: (l) => onCreated(l.id) });
      }}
    >
      <h2 className="text-lg font-semibold">New print list</h2>
      <label className="block">
        <span className="mb-1.5 block text-sm font-medium">Name</span>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoFocus
          placeholder="For example Weekend run"
          maxLength={100}
          className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-accent focus:ring-2 focus:ring-accent/25"
        />
      </label>
      <PrinterField value={printer} onChange={setPrinter} printers={printers} />
      <ErrorText>{create.error?.message}</ErrorText>
      <div className="flex gap-2">
        <Button type="submit" disabled={!name.trim()} loading={create.isPending}>
          Create list
        </Button>
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

function ListView({ list, printers, onGone }: { list: PrintList; printers: string[]; onGone: () => void }) {
  const { data, isPending, error } = usePrintItems(list.id);
  const [openId, setOpenId] = useState<number | null>(null);
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const remove = useDeletePrintList();
  const clear = useClearDone();
  const items = data?.items ?? [];
  const live = data?.list ?? list;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start gap-x-3 gap-y-2 print:hidden">
        <div className="min-w-0 flex-1 basis-56">
          <h2 className="truncate text-lg font-semibold">{live.name}</h2>
          <p className="text-sm text-muted tabular-nums">
            {live.printer ? `${live.printer} · ` : ""}
            {formatNumber(live.todo)} to print, {formatNumber(live.printing)} printing, {formatNumber(live.done)} done
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <ToolbarButton onClick={() => setEditing(!editing)} active={editing} icon={<Pencil className="size-4" aria-hidden />}>
            Edit
          </ToolbarButton>
          <a
            href={`/api/print-lists/${list.id}/export.csv`}
            className="inline-flex h-10 items-center gap-1.5 rounded-lg border border-border bg-surface px-3 text-sm text-muted transition hover:text-fg focus-visible:outline-2 focus-visible:outline-accent"
          >
            <Download className="size-4" aria-hidden />
            <span>CSV</span>
          </a>
          <ToolbarButton onClick={() => window.print()} icon={<Printer className="size-4" aria-hidden />} disabled={items.length === 0}>
            Print or PDF
          </ToolbarButton>
          <ToolbarButton
            onClick={() => clear.mutate(list.id)}
            disabled={live.done === 0 || clear.isPending}
            icon={<RotateCcw className="size-4" aria-hidden />}
          >
            Clear done
          </ToolbarButton>
          <ToolbarButton onClick={() => setConfirming(true)} icon={<Trash2 className="size-4" aria-hidden />}>
            Delete
          </ToolbarButton>
        </div>
      </div>

      {editing && <EditList list={live} printers={printers} onDone={() => setEditing(false)} />}
      <ErrorText>{error?.message ?? clear.error?.message}</ErrorText>

      {isPending ? (
        <Spinner />
      ) : items.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border px-6 py-14 text-center print:hidden">
          <p className="font-medium">This list is empty</p>
          <p className="mx-auto mt-1 max-w-sm text-sm text-muted">
            In the{" "}
            <Link to="/library" className="text-accent underline-offset-2 hover:underline">
              library
            </Link>
            , choose Select, pick the models, then Print queue. Or open a file and add it from its panel.
          </p>
        </div>
      ) : (
        <div className="grid gap-4 @2xl:grid-cols-3 print:hidden">
          {COLUMNS.map((col) => (
            <Column
              key={col.status}
              title={col.title}
              empty={col.empty}
              all={items}
              status={col.status}
              order={col.order}
              onOpen={setOpenId}
            />
          ))}
        </div>
      )}

      <PrintSheet list={live} items={items} />
      <div className="print:hidden">
        <FileOverlays openId={openId} onClose={() => setOpenId(null)} />
      </div>

      {confirming && (
        <Dialog
          title={`Delete “${live.name}”?`}
          onClose={() => setConfirming(false)}
          footer={
            <>
              <Button variant="secondary" onClick={() => setConfirming(false)}>
                Cancel
              </Button>
              <Button className="bg-danger text-accent-fg" loading={remove.isPending} onClick={() => remove.mutate(list.id, { onSuccess: onGone })}>
                Delete list
              </Button>
            </>
          }
        >
          <p className="text-sm text-muted">
            This removes the list and its {formatNumber(items.length)} {items.length === 1 ? "entry" : "entries"}.{" "}
            <span className="text-fg">The models themselves are not touched.</span>
          </p>
          <ErrorText>{remove.error?.message}</ErrorText>
        </Dialog>
      )}
    </div>
  );
}

function ToolbarButton({
  onClick,
  active,
  disabled,
  icon,
  children,
}: {
  onClick: () => void;
  active?: boolean;
  disabled?: boolean;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      aria-pressed={active}
      className={`inline-flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm transition focus-visible:outline-2 focus-visible:outline-accent disabled:cursor-not-allowed disabled:opacity-50 ${
        active ? "border-accent bg-accent/10 text-accent" : "border-border bg-surface text-muted hover:text-fg"
      }`}
    >
      {icon}
      <span>{children}</span>
    </button>
  );
}

function EditList({ list, printers, onDone }: { list: PrintList; printers: string[]; onDone: () => void }) {
  const update = useUpdatePrintList();
  const [name, setName] = useState(list.name);
  const [printer, setPrinter] = useState(list.printer);
  return (
    <form
      className="max-w-md space-y-3 rounded-xl border border-border bg-surface p-4 print:hidden"
      onSubmit={(e) => {
        e.preventDefault();
        if (name.trim()) update.mutate({ id: list.id, name, printer }, { onSuccess: onDone });
      }}
    >
      <label className="block">
        <span className="mb-1.5 block text-sm font-medium">Name</span>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoFocus
          maxLength={100}
          className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-accent focus:ring-2 focus:ring-accent/25"
        />
      </label>
      <PrinterField value={printer} onChange={setPrinter} printers={printers} />
      <ErrorText>{update.error?.message}</ErrorText>
      <div className="flex gap-2">
        <Button type="submit" disabled={!name.trim()} loading={update.isPending}>
          Save
        </Button>
        <Button type="button" variant="ghost" onClick={onDone}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

function Column({
  title,
  empty,
  status,
  all,
  order,
  onOpen,
}: {
  title: string;
  empty: string;
  status: PrintStatus;
  all: PrintItem[];
  order: string;
  onOpen: (id: number) => void;
}) {
  const items = all.filter((i) => i.status === status);
  return (
    <section aria-label={title} className={`min-w-0 space-y-2 ${order}`}>
      <h3 className="flex items-baseline gap-2 text-sm font-medium">
        {title}
        <span className="text-xs font-normal text-muted tabular-nums">{formatNumber(items.length)}</span>
      </h3>
      {items.length === 0 ? (
        <p className="rounded-lg border border-dashed border-border px-3 py-6 text-center text-sm text-muted">{empty}</p>
      ) : (
        <ul className="space-y-2">
          {items.map((it, i) => (
            <Card key={it.id} item={it} neighbours={items} index={i} onOpen={onOpen} />
          ))}
        </ul>
      )}
    </section>
  );
}

const NEXT: Record<PrintStatus, { to: PrintStatus; label: string; icon: React.ReactNode } | null> = {
  todo: { to: "printing", label: "Start printing", icon: <Play className="size-4 text-accent" aria-hidden /> },
  printing: { to: "done", label: "Mark done", icon: <Check className="size-4 text-accent" aria-hidden /> },
  done: { to: "todo", label: "Print again", icon: <RotateCcw className="size-4" aria-hidden /> },
};

function Card({ item, neighbours, index, onOpen }: { item: PrintItem; neighbours: PrintItem[]; index: number; onOpen: (id: number) => void }) {
  const update = useUpdatePrintItem();
  const remove = useDeletePrintItem();
  const [note, setNote] = useState(item.note);
  const [more, setMore] = useState(false);
  useEffect(() => setNote(item.note), [item.note]);
  const f = item.file;
  const next = NEXT[item.status];

  function move(by: -1 | 1) {
    const target = neighbours[index + by];
    const beyond = neighbours[index + by * 2];
    if (!target) return;
    const position = beyond ? (target.position + beyond.position) / 2 : target.position + by;
    update.mutate({ id: item.id, position });
  }

  const stepper =
    "grid size-8 place-items-center rounded-lg text-muted hover:bg-surface-2 hover:text-fg disabled:opacity-30 pointer-coarse:size-10";
  const moreItem =
    "flex min-h-10 w-full items-center gap-2 rounded-lg px-2.5 text-left text-sm text-muted hover:bg-surface-2 hover:text-fg disabled:opacity-40";

  return (
    <li className="space-y-2 rounded-xl border border-border bg-surface p-3">
      <div className="flex items-start gap-2.5">
        <button
          onClick={() => onOpen(f.id)}
          aria-label={`Open ${f.name}`}
          className="size-11 shrink-0 overflow-hidden rounded-lg bg-surface-2 focus-visible:outline-2 focus-visible:outline-accent"
        >
          <Thumbnail file={f} />
        </button>
        <div className="min-w-0 flex-1">
          <button onClick={() => onOpen(f.id)} className="line-clamp-2 text-left text-sm font-medium break-all hover:underline">
            {f.name}
          </button>
          <p className="truncate text-xs text-muted">
            <span className="uppercase">{f.format}</span>
            {f.dimensions && <> · {formatDims(f.dimensions)}</>}
            {f.missing && " · missing from disk"}
          </p>
        </div>
        <button
          onClick={() => setMore(!more)}
          aria-expanded={more}
          aria-label={`More for ${f.name}`}
          title="More"
          className={`grid size-9 shrink-0 place-items-center rounded-lg focus-visible:outline-2 focus-visible:outline-accent ${
            more ? "bg-accent/10 text-accent" : "text-muted hover:bg-surface-2 hover:text-fg"
          }`}
        >
          <Ellipsis className="size-4" aria-hidden />
        </button>
      </div>

      <input
        value={note}
        onChange={(e) => setNote(e.target.value)}
        onBlur={() => note !== item.note && update.mutate({ id: item.id, note })}
        onKeyDown={(e) => e.key === "Enter" && e.currentTarget.blur()}
        placeholder="Note: filament, settings…"
        aria-label={`Note for ${f.name}`}
        maxLength={1000}
        className="w-full rounded-lg border border-transparent bg-surface-2 px-2.5 py-1.5 text-sm outline-none placeholder:text-muted focus:border-accent focus:bg-surface focus:ring-2 focus:ring-accent/25"
      />

      <div className="flex flex-wrap items-center gap-2">
        {next && (
          <Button
            variant="secondary"
            className="h-10 min-w-[7.5rem] flex-1 px-3 whitespace-nowrap"
            loading={update.isPending && update.variables?.status === next.to}
            onClick={() => update.mutate({ id: item.id, status: next.to })}
          >
            {next.icon} {next.label}
          </Button>
        )}
        <div className="flex shrink-0 items-center" role="group" aria-label={`Quantity of ${f.name}`}>
          <button
            onClick={() => update.mutate({ id: item.id, quantity: item.quantity - 1 })}
            disabled={item.quantity <= 1}
            aria-label="One fewer"
            className={stepper}
          >
            <Minus className="size-4" aria-hidden />
          </button>
          <span className="w-8 text-center text-sm font-medium tabular-nums" aria-live="polite">
            ×{item.quantity}
          </span>
          <button
            onClick={() => update.mutate({ id: item.id, quantity: item.quantity + 1 })}
            disabled={item.quantity >= 999}
            aria-label="One more"
            className={stepper}
          >
            <Plus className="size-4" aria-hidden />
          </button>
        </div>
      </div>

      {more && (
        <div className="border-t border-border pt-1">
          {item.status === "printing" && (
            <button onClick={() => update.mutate({ id: item.id, status: "todo" })} className={moreItem}>
              <Undo2 className="size-4" aria-hidden /> Back to To print
            </button>
          )}
          {item.status !== "done" && (
            <>
              <button onClick={() => move(-1)} disabled={index === 0} className={moreItem}>
                <ArrowUp className="size-4" aria-hidden /> Move up
              </button>
              <button onClick={() => move(1)} disabled={index === neighbours.length - 1} className={moreItem}>
                <ArrowDown className="size-4" aria-hidden /> Move down
              </button>
            </>
          )}
          <button onClick={() => remove.mutate(item.id)} className={`${moreItem} hover:text-danger`}>
            <Trash2 className="size-4" aria-hidden /> Take off the list
          </button>
        </div>
      )}
      {(update.error || remove.error) && <p role="alert" className="text-sm text-danger">{(update.error ?? remove.error)?.message}</p>}
    </li>
  );
}

const statusWords: Record<PrintStatus, string> = { todo: "To print", printing: "Printing", done: "Done" };

/** The list as a plain sheet for paper or a PDF: only shown when printing. */
function PrintSheet({ list, items }: { list: PrintList; items: PrintItem[] }) {
  const open = items.filter((i) => i.status !== "done");
  const finished = items.filter((i) => i.status === "done");
  return (
    <div className="hidden text-black print:block">
      <h1 className="text-xl font-semibold">{list.name}</h1>
      <p className="mb-4 text-sm">
        {list.printer ? `${list.printer} · ` : ""}
        {new Date().toLocaleDateString(undefined, { dateStyle: "long" })}
      </p>
      <table className="w-full border-collapse text-left text-sm">
        <thead>
          <tr className="border-b border-black">
            <th className="w-8 py-1 pr-2" scope="col">
              <span className="sr-only">Printed</span>
            </th>
            <th className="w-12 py-1 pr-2" scope="col">Qty</th>
            <th className="py-1 pr-2" scope="col">Model</th>
            <th className="py-1 pr-2" scope="col">Size</th>
            <th className="py-1 pr-2" scope="col">Status</th>
            <th className="py-1" scope="col">Note</th>
          </tr>
        </thead>
        <tbody>
          {[...open, ...finished].map((it) => (
            <tr key={it.id} className="border-b border-neutral-400 align-top">
              <td className="py-1.5 pr-2">
                <span className="inline-block size-4 border border-black" />
              </td>
              <td className="py-1.5 pr-2 tabular-nums">×{it.quantity}</td>
              <td className="py-1.5 pr-2">
                {it.file.name}
                <span className="block text-xs break-all text-neutral-600">{it.file.host_path ?? `${it.file.source_name} / ${it.file.rel_path}`}</span>
              </td>
              <td className="py-1.5 pr-2 tabular-nums">{it.file.dimensions ? formatDims(it.file.dimensions) : ""}</td>
              <td className="py-1.5 pr-2">{statusWords[it.status]}</td>
              <td className="py-1.5">{it.note}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
