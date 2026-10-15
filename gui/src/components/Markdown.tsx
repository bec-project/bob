import { memo } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { openExternal } from "../platform";
import { CopyButton } from "./CopyButton";

function nodeText(node: unknown): string {
  if (typeof node === "string") return node;
  if (Array.isArray(node)) return node.map(nodeText).join("");
  if (node && typeof node === "object" && "props" in node) {
    return nodeText((node as { props: { children?: unknown } }).props.children);
  }
  return "";
}

const components: Components = {
  pre({ children }) {
    return (
      <pre>
        {children}
        <CopyButton text={nodeText(children).replace(/\n$/, "")} className="copy" />
      </pre>
    );
  },
  a({ href, children }) {
    return (
      <a
        href={href}
        onClick={(event) => {
          if (!href || href.startsWith("#")) return;
          event.preventDefault();
          void openExternal(href);
        }}
      >
        {children}
      </a>
    );
  },
};

/** Model output as Markdown. Raw HTML in the text is shown as text, never rendered. */
export const Markdown = memo(function Markdown({ text }: { text: string }) {
  return (
    <div className="markdown">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {text}
      </ReactMarkdown>
    </div>
  );
});
