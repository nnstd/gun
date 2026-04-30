import { useEffect, useState, type ReactNode } from "react";
import { hc, type InferResponseType } from "hono/client";
import { ArrowRight, Check, ChevronDown, Copy, Github } from "lucide-react";
import { highlightCodeBlock } from "../lib/code-highlighter";
import type { AppType } from "../../server/app";

type PageKind = "home" | "docs" | "blog";
type InstallTabSize = "md" | "lg";
type NoteKind = "info" | "tip" | "warn";

function cx(...parts: Array<string | false | null | undefined>) {
  return parts.filter(Boolean).join(" ");
}

const NAV_ITEMS = [
  { label: "Docs", href: "/docs", page: "docs" as const },
  { label: "Benchmarks", href: "/#benchmarks", page: "home" as const },
  { label: "Examples", href: "/#examples", page: "home" as const },
  { label: "Blog", href: "/blog", page: "blog" as const },
];

const FOOTER_COLUMNS = [
  {
    title: "Product",
    links: [
      { label: "Docs", href: "/docs" },
      { label: "Benchmarks", href: "/#benchmarks" },
      { label: "Changelog", href: "#" },
      { label: "Roadmap", href: "#" },
    ],
  },
  {
    title: "Resources",
    links: [
      { label: "Examples", href: "/#examples" },
      { label: "Migration guide", href: "/docs" },
      { label: "Source maps", href: "/docs" },
      { label: "API reference", href: "/docs" },
    ],
  },
  {
    title: "Community",
    links: [
      { label: "GitHub", href: "https://github.com/nnstd/gun" },
      { label: "Discord", href: "#" },
      { label: "Twitter", href: "#" },
      { label: "Bluesky", href: "#" },
      { label: "npm", href: "#" },
    ],
  },
];

const githubClient = hc<AppType>("/");
type GithubStarsResponse = InferResponseType<
  typeof githubClient.api.github.stars.$get,
  200
>;

const INSTALL_COMMANDS = {
  curl: "curl -fsSL https://gun.nnstd.dev/install | bash",
  npm: "npm i -g gun-transpiler",
  bun: "bun add -g gun-transpiler",
  yarn: "yarn global add gun-transpiler",
  pnpm: "pnpm add -g gun-transpiler",
} as const;

function renderInstallCommand(command: string) {
  const parts = command.split(" ");

  return parts.map((part, index) => {
    const key = `${part}-${index}`;
    let className = "text-white/90";

    if (index === 0) {
      className = "text-amber-300";
    } else if (part.startsWith("-")) {
      className = "text-brand-200";
    } else if (part === "gun-transpiler" || part === "gun.nnstd.dev/install") {
      className = "text-green-300";
    } else if (
      part === "i" ||
      part === "add" ||
      part === "global" ||
      part === "|"
    ) {
      className = "text-cyan-300";
    } else if (part === "bash") {
      className = "text-amber-200";
    }

    return (
      <span key={key} className={className}>
        {index > 0 ? " " : ""}
        {part}
      </span>
    );
  });
}

export function SiteHeader({
  current,
  crumb,
}: {
  current: PageKind;
  crumb?: ReactNode;
}) {
  const [stars, setStars] = useState("--");

  useEffect(() => {
    let cancelled = false;

    async function loadStars() {
      const response = await githubClient.api.github.stars.$get();

      if (!response.ok) return;

      const data: GithubStarsResponse = await response.json();

      if (!cancelled) {
        setStars(formatGithubStars(data.stars));
      }
    }

    loadStars().catch(() => {
      if (!cancelled) setStars("--");
    });

    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <header className="sticky top-0 z-50 border-b border-white/10 bg-black/65 backdrop-blur-xl">
      <div className="mx-auto flex max-w-370 flex-col gap-4 px-5 py-4 sm:px-8 lg:px-12">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex items-center gap-4">
            <a
              href="/"
              className="inline-flex items-center gap-3 text-white no-underline"
            >
              <MascotMark size={32} />
              <span className="font-syne text-2xl font-extrabold tracking-[-0.04em]">
                gun
              </span>
            </a>
            {crumb ? (
              <div className="hidden items-center gap-3 text-sm text-white/45 lg:flex">
                <span>/</span>
                <span className="font-mono text-[13px] tracking-[0.08em] text-white/65">
                  {crumb}
                </span>
              </div>
            ) : null}
          </div>

          <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:gap-8">
            <nav className="flex flex-wrap gap-x-5 gap-y-2 text-sm text-white/60 lg:justify-center">
              {NAV_ITEMS.map((item) => {
                const active = current === item.page;
                return (
                  <a
                    key={item.label}
                    href={item.href}
                    className={cx(
                      "transition-colors hover:text-white",
                      active && "text-white",
                    )}
                  >
                    {item.label}
                  </a>
                );
              })}
            </nav>

            <div className="flex items-center gap-3">
              <a
                href="https://github.com/nnstd/gun"
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-2 text-sm text-white/60 transition-colors hover:text-white"
              >
                <Github className="h-4 w-4" />
                {stars}
              </a>
              <a
                href="/#install"
                className="inline-flex items-center gap-2 rounded-xl border border-brand-400/60 bg-brand-500 px-4 py-2 text-sm font-semibold text-white shadow-[0_0_24px_rgba(66,68,147,0.45)] transition hover:bg-brand-400"
              >
                Install
                <ArrowRight className="h-4 w-4" />
              </a>
            </div>
          </div>
        </div>

        {crumb ? (
          <div className="font-mono text-[12px] tracking-[0.08em] text-white/60 lg:hidden">
            {crumb}
          </div>
        ) : null}
      </div>
    </header>
  );
}

function formatGithubStars(stars: number) {
  if (stars >= 1000) {
    return `${(stars / 1000).toFixed(1)}k`;
  }

  return String(stars);
}

export function SiteFooter() {
  return (
    <footer className="border-t border-white/10">
      <div className="mx-auto max-w-370 px-5 py-12 sm:px-8 lg:px-12 lg:py-14">
        <div className="grid gap-10 border-b border-white/10 pb-10 lg:grid-cols-[1.4fr_1fr_1fr_1fr]">
          <div>
            <div className="mb-4 flex items-center gap-3">
              <MascotMark size={28} />
              <span className="font-syne text-2xl font-extrabold tracking-[-0.04em] text-white">
                gun
              </span>
            </div>
            <p className="max-w-xs text-sm leading-6 text-white/60">
              A JavaScript-to-Go compiler with a runtime that gets out of your
              way.
            </p>
          </div>

          {FOOTER_COLUMNS.map((column) => (
            <div key={column.title}>
              <div className="mb-4 font-mono text-[10px] font-bold uppercase tracking-[0.18em] text-brand-300">
                {column.title}
              </div>
              <ul className="space-y-2.5 text-sm text-white/60">
                {column.links.map((link) => (
                  <li key={link.label}>
                    <a
                      href={link.href}
                      className="transition-colors hover:text-white"
                    >
                      {link.label}
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        <div className="flex flex-col gap-3 pt-6 font-mono text-[11px] tracking-[0.12em] text-white/35 sm:flex-row sm:items-center sm:justify-between">
          <div>MIT / Copyright 2026 the gun project</div>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:gap-6">
            <span>
              <span className="mr-2 inline-block h-1.5 w-1.5 rounded-full bg-green-400 align-middle shadow-[0_0_6px_rgba(74,222,128,0.9)]" />
              all systems normal
            </span>
            <span>v1.0.2 / go1.22+</span>
          </div>
        </div>
      </div>
    </footer>
  );
}

export function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <div className="mb-4 font-mono text-[11px] font-bold uppercase tracking-[0.18em] text-brand-300">
      {children}
    </div>
  );
}

export function InstallTabs({ size = "md" }: { size?: InstallTabSize }) {
  const [activeTab, setActiveTab] =
    useState<keyof typeof INSTALL_COMMANDS>("curl");
  const [copied, setCopied] = useState(false);
  const command = INSTALL_COMMANDS[activeTab];
  const large = size === "lg";

  async function copyCommand() {
    if (typeof navigator !== "undefined" && navigator.clipboard) {
      await navigator.clipboard.writeText(command);
    }

    setCopied(true);
    window.setTimeout(() => setCopied(false), 1400);
  }

  return (
    <div
      id="install"
      className={cx(
        "inline-flex w-full max-w-105 flex-col overflow-hidden rounded-2xl border border-brand-400/50 bg-black/55 shadow-[0_0_36px_rgba(66,68,147,0.22)]",
        large && "max-w-115",
      )}
    >
      <div className="grid grid-cols-5 border-b border-white/10 bg-brand-500/10">
        {(
          Object.keys(INSTALL_COMMANDS) as Array<keyof typeof INSTALL_COMMANDS>
        ).map((tab) => (
          <button
            key={tab}
            type="button"
            onClick={() => setActiveTab(tab)}
            className={cx(
              "border-b-2 px-3 py-2 font-mono text-[11px] font-bold uppercase tracking-[0.12em] transition",
              activeTab === tab
                ? "border-brand-400 bg-brand-500/20 text-white"
                : "border-transparent text-white/45 hover:text-white/80",
            )}
          >
            {tab}
          </button>
        ))}
      </div>
      <div
        className={cx(
          "flex items-center gap-3 px-4 py-4",
          large && "px-5 py-5",
        )}
      >
        <span className="font-mono text-sm text-brand-300">$</span>
        <code
          className={cx(
            "flex-1 overflow-x-auto whitespace-nowrap font-mono text-sm",
            large && "text-[15px]",
          )}
        >
          {renderInstallCommand(command)}
        </code>
        <button
          type="button"
          onClick={copyCommand}
          className={cx(
            "inline-flex items-center gap-1 rounded-lg border px-2.5 py-1.5 font-mono text-[10px] font-bold uppercase tracking-[0.12em] transition",
            copied
              ? "border-green-400/50 bg-green-500/10 text-green-300"
              : "border-white/10 bg-white/5 text-white/65 hover:border-brand-300/40 hover:text-white",
          )}
        >
          {copied ? (
            <Check className="h-3.5 w-3.5" />
          ) : (
            <Copy className="h-3.5 w-3.5" />
          )}
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
    </div>
  );
}

export function InlineCode({ children }: { children: ReactNode }) {
  return (
    <code className="rounded-md border border-brand-400/25 bg-brand-500/10 px-2 py-1 font-mono text-[12px] text-brand-100">
      {children}
    </code>
  );
}

export function Note({
  type = "info",
  children,
}: {
  type?: NoteKind;
  children: ReactNode;
}) {
  const styles = {
    info: "border-brand-400/30 bg-brand-500/10 text-brand-50",
    tip: "border-green-400/30 bg-green-500/10 text-green-50",
    warn: "border-amber-400/30 bg-amber-500/10 text-amber-50",
  } as const;

  const icons = {
    info: "i",
    tip: "*",
    warn: "!",
  } as const;

  return (
    <div
      className={cx(
        "my-6 flex gap-4 rounded-2xl border px-4 py-4 text-sm leading-7",
        styles[type],
      )}
    >
      <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full border border-current/20 font-mono text-[11px] font-bold uppercase">
        {icons[type]}
      </div>
      <div className="text-white/80">{children}</div>
    </div>
  );
}

export function CodeBlock({
  lang = "bash",
  file,
  code,
}: {
  lang?: string;
  file?: string;
  code: string;
}) {
  const [copied, setCopied] = useState(false);
  const [highlighted, setHighlighted] = useState("");

  const accents: Record<string, string> = {
    bash: "border-green-400/30 bg-green-500/10 text-green-300",
    js: "border-amber-400/30 bg-amber-500/10 text-amber-300",
    ts: "border-cyan-400/30 bg-cyan-500/10 text-cyan-300",
    go: "border-sky-400/30 bg-sky-500/10 text-sky-300",
  };

  async function copyCode() {
    if (typeof navigator !== "undefined" && navigator.clipboard) {
      await navigator.clipboard.writeText(code);
    }

    setCopied(true);
    window.setTimeout(() => setCopied(false), 1400);
  }

  useEffect(() => {
    let cancelled = false;

    highlightCodeBlock(code, lang).then((html) => {
      if (!cancelled) setHighlighted(html);
    });

    return () => {
      cancelled = true;
    };
  }, [code, lang]);

  return (
    <div className="my-6 overflow-hidden rounded-2xl border border-white/10 bg-black/55 shadow-[0_12px_42px_rgba(0,0,0,0.35)]">
      <div className="flex items-center gap-3 border-b border-white/10 bg-white/5 px-4 py-3">
        <span
          className={cx(
            "rounded-md border px-2 py-1 font-mono text-[10px] font-bold uppercase tracking-[0.12em]",
            accents[lang] ?? accents.bash,
          )}
        >
          {lang}
        </span>
        {file ? (
          <span className="font-mono text-[12px] text-white/35">{file}</span>
        ) : null}
        <button
          type="button"
          onClick={copyCode}
          className="ml-auto inline-flex items-center gap-1 rounded-lg border border-white/10 px-2.5 py-1.5 font-mono text-[10px] font-bold uppercase tracking-[0.12em] text-white/55 transition hover:text-white"
        >
          {copied ? (
            <Check className="h-3.5 w-3.5" />
          ) : (
            <Copy className="h-3.5 w-3.5" />
          )}
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      {highlighted ? (
        <div
          className="overflow-x-auto px-4 py-5 text-[13px] [&_.code-shiki]:m-0 [&_.code-shiki]:bg-transparent [&_.code-shiki]:p-0 [&_.code-shiki]:font-mono [&_.code-shiki]:text-white/90 [&_.code-shiki]:text-[13px] [&_.code-shiki__code]:block [&_.code-shiki__code]:whitespace-pre [&_.line]:block [&_.line]:leading-0"
          dangerouslySetInnerHTML={{ __html: highlighted }}
        />
      ) : (
        <pre className="overflow-x-auto px-4 py-5 font-mono text-[13px] text-white/90">
          <code>{code}</code>
        </pre>
      )}
    </div>
  );
}

export function Tag({ label, color }: { label: string; color: string }) {
  return (
    <span
      className="inline-flex items-center rounded-full border px-3 py-1 font-mono text-[11px] font-bold uppercase tracking-[0.12em]"
      style={{
        borderColor: `${color}55`,
        backgroundColor: `${color}18`,
        color,
      }}
    >
      {label}
    </span>
  );
}

export function FAQItem({
  question,
  answer,
  open,
  onToggle,
  index,
}: {
  question: string;
  answer: string;
  open: boolean;
  onToggle: () => void;
  index: number;
}) {
  return (
    <div className="border-b border-white/10">
      <button
        type="button"
        onClick={onToggle}
        className="flex w-full items-center justify-between gap-5 py-6 text-left"
      >
        <span className="font-syne text-xl font-bold tracking-[-0.03em] text-white sm:text-2xl">
          <span className="mr-4 font-mono text-xs font-medium uppercase tracking-[0.16em] text-brand-300">
            {String(index + 1).padStart(2, "0")}
          </span>
          {question}
        </span>
        <span
          className={cx(
            "flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-brand-300/45 text-white transition",
            open && "rotate-180",
          )}
        >
          <ChevronDown className="h-4 w-4" />
        </span>
      </button>
      {open ? (
        <p className="max-w-3xl pb-6 pr-2 text-[15px] leading-7 text-white/65">
          {answer}
        </p>
      ) : null}
    </div>
  );
}

export function Mascot({
  size = 280,
  compact = false,
}: {
  size?: number;
  compact?: boolean;
}) {
  const glowSize = size + 72;

  return (
    <div
      className="relative flex items-center justify-center"
      style={{ width: glowSize, height: glowSize }}
    >
      {!compact ? (
        <div
          className="absolute rounded-full bg-[radial-gradient(circle,rgba(66,68,147,0.34)_0%,transparent_70%)] blur-sm"
          style={{ width: glowSize, height: glowSize }}
        />
      ) : null}
      <svg
        viewBox="0 0 200 200"
        fill="none"
        className="relative animate-float drop-shadow-[0_18px_48px_rgba(66,68,147,0.45)]"
        style={{ width: size, height: size }}
      >
        <defs>
          <radialGradient id="mascot-core" cx="40%" cy="35%" r="65%">
            <stop offset="0%" stopColor="#6666d4" />
            <stop offset="100%" stopColor="#2a2a72" />
          </radialGradient>
          <radialGradient id="mascot-shine" cx="35%" cy="30%" r="65%">
            <stop offset="0%" stopColor="white" stopOpacity="0.15" />
            <stop offset="100%" stopColor="white" stopOpacity="0" />
          </radialGradient>
        </defs>
        <ellipse cx="82" cy="54" rx="18" ry="22" fill="#3535a0" />
        <ellipse cx="118" cy="54" rx="18" ry="22" fill="#3535a0" />
        <circle cx="100" cy="96" r="80" fill="url(#mascot-core)" />
        <circle cx="100" cy="96" r="76" fill="url(#mascot-shine)" />
        <ellipse
          cx="78"
          cy="62"
          rx="28"
          ry="16"
          fill="white"
          opacity="0.08"
          transform="rotate(-15 78 62)"
        />
        <circle cx="78" cy="92" r="18" fill="white" />
        <circle cx="122" cy="92" r="18" fill="white" />
        <circle cx="80" cy="94" r="11" fill="#0f0f2e" />
        <circle cx="124" cy="94" r="11" fill="#0f0f2e" />
        <circle cx="85" cy="88" r="4.5" fill="white" />
        <circle cx="129" cy="88" r="4.5" fill="white" />
        <path
          d="M82 120 Q100 136 118 120"
          stroke="#0f0f2e"
          strokeWidth="3.5"
          fill="none"
          strokeLinecap="round"
        />
        <ellipse
          cx="56"
          cy="114"
          rx="16"
          ry="9"
          fill="#f472b6"
          opacity="0.22"
        />
        <ellipse
          cx="144"
          cy="114"
          rx="16"
          ry="9"
          fill="#f472b6"
          opacity="0.22"
        />
      </svg>
    </div>
  );
}

export function MascotMark({ size = 28 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 200 200"
      fill="none"
      className="shrink-0 drop-shadow-[0_0_10px_rgba(66,68,147,0.55)]"
    >
      <defs>
        <radialGradient id={`mascot-mark-${size}`} cx="40%" cy="35%" r="65%">
          <stop offset="0%" stopColor="#6666d4" />
          <stop offset="100%" stopColor="#2a2a72" />
        </radialGradient>
      </defs>
      <ellipse cx="82" cy="54" rx="18" ry="22" fill="#3535a0" />
      <ellipse cx="118" cy="54" rx="18" ry="22" fill="#3535a0" />
      <circle cx="100" cy="96" r="80" fill={`url(#mascot-mark-${size})`} />
      <circle cx="78" cy="92" r="18" fill="white" />
      <circle cx="122" cy="92" r="18" fill="white" />
      <circle cx="80" cy="94" r="11" fill="#0f0f2e" />
      <circle cx="124" cy="94" r="11" fill="#0f0f2e" />
      <circle cx="85" cy="88" r="4.5" fill="white" />
      <circle cx="129" cy="88" r="4.5" fill="white" />
      <path
        d="M82 120 Q100 136 118 120"
        stroke="#0f0f2e"
        strokeWidth="6"
        fill="none"
        strokeLinecap="round"
      />
    </svg>
  );
}

export function MascotNotFound({ size = 240 }: { size?: number }) {
  return (
    <svg
      viewBox="0 0 200 200"
      fill="none"
      className="relative animate-float drop-shadow-[0_16px_48px_rgba(66,68,147,0.5)]"
      style={{ width: size, height: size }}
      aria-hidden="true"
    >
      <defs>
        <radialGradient id={`mascot-not-found-${size}`} cx="40%" cy="35%" r="65%">
          <stop offset="0%" stopColor="#6666d4" />
          <stop offset="100%" stopColor="#2a2a72" />
        </radialGradient>
        <radialGradient id={`mascot-not-found-shine-${size}`} cx="35%" cy="30%" r="65%">
          <stop offset="0%" stopColor="white" stopOpacity="0.15" />
          <stop offset="100%" stopColor="white" stopOpacity="0" />
        </radialGradient>
      </defs>
      <circle cx="100" cy="96" r="80" fill={`url(#mascot-not-found-${size})`} />
      <ellipse cx="82" cy="54" rx="18" ry="22" fill="#3535a0" />
      <ellipse cx="118" cy="54" rx="18" ry="22" fill="#3535a0" />
      <circle cx="100" cy="96" r="76" fill={`url(#mascot-not-found-${size})`} />
      <circle cx="100" cy="96" r="76" fill={`url(#mascot-not-found-shine-${size})`} />
      <ellipse
        cx="78"
        cy="62"
        rx="28"
        ry="16"
        fill="white"
        opacity="0.08"
        transform="rotate(-15 78 62)"
      />
      <circle cx="78" cy="92" r="18" fill="white" />
      <circle cx="122" cy="92" r="18" fill="white" />
      <circle cx="86" cy="94" r="11" fill="#0f0f2e" />
      <circle cx="116" cy="94" r="11" fill="#0f0f2e" />
      <circle cx="91" cy="88" r="4.5" fill="white" opacity="0.95" />
      <circle cx="121" cy="88" r="4.5" fill="white" opacity="0.95" />
      <ellipse cx="100" cy="128" rx="5" ry="6" fill="#0f0f2e" />
      <ellipse cx="56" cy="114" rx="16" ry="9" fill="#f472b6" opacity="0.22" />
      <ellipse cx="144" cy="114" rx="16" ry="9" fill="#f472b6" opacity="0.22" />
      <text
        x="148"
        y="44"
        fontFamily="Syne, sans-serif"
        fontSize="36"
        fontWeight="800"
        fill="#a0a0ff"
        opacity="0.85"
      >
        ?
      </text>
      <text
        x="40"
        y="58"
        fontFamily="Syne, sans-serif"
        fontSize="22"
        fontWeight="800"
        fill="#a0a0ff"
        opacity="0.5"
      >
        ?
      </text>
    </svg>
  );
}
