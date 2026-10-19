import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

// Readmes come from the library, so they are untrusted: raw HTML is never rendered
// (react-markdown's default), links open outside the app, and images never load
// (relative paths can't resolve, and remote ones would leave the network).
const components: Components = {
  a: ({ href, children }) => (
    <a href={href} target="_blank" rel="noopener noreferrer nofollow" className="text-accent underline underline-offset-2">
      {children}
    </a>
  ),
  img: ({ src, alt }) =>
    typeof src === "string" && /^https?:/i.test(src) ? (
      <a href={src} target="_blank" rel="noopener noreferrer nofollow" className="text-accent underline underline-offset-2">
        {alt || "Image"} (opens outside MeshManager)
      </a>
    ) : (
      <span className="text-muted">[{alt || "image"}]</span>
    ),
  h1: ({ children }) => <h1 className="mt-8 mb-3 text-2xl font-semibold first:mt-0">{children}</h1>,
  h2: ({ children }) => <h2 className="mt-7 mb-2.5 text-xl font-semibold first:mt-0">{children}</h2>,
  h3: ({ children }) => <h3 className="mt-6 mb-2 text-lg font-semibold first:mt-0">{children}</h3>,
  h4: ({ children }) => <h4 className="mt-5 mb-2 font-semibold first:mt-0">{children}</h4>,
  p: ({ children }) => <p className="my-3 leading-relaxed">{children}</p>,
  ul: ({ children }) => <ul className="my-3 list-disc space-y-1 pl-6">{children}</ul>,
  ol: ({ children }) => <ol className="my-3 list-decimal space-y-1 pl-6">{children}</ol>,
  blockquote: ({ children }) => <blockquote className="my-4 border-l border-border pl-4 text-muted">{children}</blockquote>,
  hr: () => <hr className="my-6 border-border" />,
  code: ({ children, className }) =>
    className ? (
      <code className={className}>{children}</code>
    ) : (
      <code className="rounded bg-surface-2 px-1 py-0.5 font-mono text-[0.9em]">{children}</code>
    ),
  pre: ({ children }) => (
    <pre className="my-4 overflow-x-auto rounded-lg bg-surface-2 p-3 font-mono text-[13px] leading-relaxed">{children}</pre>
  ),
  table: ({ children }) => (
    <div className="my-4 overflow-x-auto">
      <table className="w-full border-collapse text-sm">{children}</table>
    </div>
  ),
  th: ({ children }) => <th className="border-b border-border px-2 py-1.5 text-left font-medium">{children}</th>,
  td: ({ children }) => <td className="border-b border-border/60 px-2 py-1.5 align-top tabular-nums">{children}</td>,
};

export default function Markdown({ text }: { text: string }) {
  return (
    <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
      {text}
    </ReactMarkdown>
  );
}
