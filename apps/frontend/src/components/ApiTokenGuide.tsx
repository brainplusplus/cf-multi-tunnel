import { useState } from 'react';
import { AlertTriangle, ChevronDown, ExternalLink, ShieldCheck } from 'lucide-react';

const TOKENS_URL = 'https://dash.cloudflare.com/profile/api-tokens';

type Policy = 'Account' | 'Zone';

// The four permission groups the panel actually uses, grouped by the policy type
// Cloudflare requires. Kept in one place so the guide and the "why" list cannot
// drift from what the backend calls.
const PERMISSIONS: { policy: Policy; group: string; toggle: 'Read' | 'Edit'; usedFor: string }[] = [
  {
    policy: 'Account',
    group: 'Cloudflare Tunnel',
    toggle: 'Edit',
    usedFor: "list, read and rewrite each tunnel's ingress",
  },
  {
    policy: 'Account',
    group: 'Account Settings',
    toggle: 'Read',
    usedFor: 'verify the token and resolve the account name',
  },
  {
    policy: 'Zone',
    group: 'Zone',
    toggle: 'Read',
    usedFor: 'find the zone that owns a hostname',
  },
  {
    policy: 'Zone',
    group: 'DNS',
    toggle: 'Edit',
    usedFor: 'create and update the CNAME pointing at the tunnel',
  },
];

// Mistakes that are easy to make in the policy editor and produce a token that
// verifies fine but then fails on the first real call.
const TRAPS = [
  {
    wrong: 'Apps (Account & Billing)',
    why: 'deprecated and unrelated to tunnels. It is easy to tick while scrolling.',
  },
  {
    wrong: 'Cloudflare One Connector: cloudflared',
    why: 'governs Cloudflare-managed connectors, not tunnel configuration. Not needed here.',
  },
  {
    wrong: 'Account permissions inside a Zone policy',
    why: 'each policy has one scope, so the editor hides them. Use two policies.',
  },
];

function Step({ n, children }: { n: number; children: React.ReactNode }) {
  return (
    <li className="flex gap-2.5">
      <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-surface-800 text-[11px] font-semibold text-slate-300">
        {n}
      </span>
      <span className="min-w-0 flex-1 text-xs leading-relaxed text-slate-400">{children}</span>
    </li>
  );
}

function PermissionTable({ policy }: { policy: Policy }) {
  const rows = PERMISSIONS.filter((p) => p.policy === policy);
  return (
    <div className="mt-2 overflow-hidden rounded-md border border-surface-800">
      <table className="w-full text-left text-[11px]">
        <thead className="bg-surface-900/60 text-slate-500">
          <tr>
            <th className="px-2 py-1.5 font-medium">Search</th>
            <th className="px-2 py-1.5 font-medium">Permission group</th>
            <th className="px-2 py-1.5 font-medium">Tick</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-surface-800">
          {rows.map((p) => (
            <tr key={p.group}>
              <td className="px-2 py-1.5 font-mono text-slate-500">{p.group.toLowerCase()}</td>
              <td className="px-2 py-1.5 text-slate-200">{p.group}</td>
              <td className="px-2 py-1.5 text-slate-200">{p.toggle}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/**
 * Step-by-step instructions for creating the scoped Cloudflare API token the
 * panel needs, matching Cloudflare's policy-based token editor (one policy per
 * scope: Account and Zone). Collapsed by default so the form stays short, but the
 * summary line makes it obvious the guide exists.
 */
export function ApiTokenGuide({ defaultOpen = false }: { defaultOpen?: boolean }) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <div className="rounded-lg border border-surface-700 bg-surface-950/50">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between gap-3 px-3 py-2.5 text-left"
      >
        <span className="flex items-center gap-2 text-xs font-medium text-slate-200">
          <ShieldCheck className="h-4 w-4 text-accent-400" />
          How to create this token
        </span>
        <ChevronDown className={`h-4 w-4 shrink-0 text-slate-500 transition-transform ${open ? 'rotate-180' : ''}`} />
      </button>

      {open && (
        <div className="space-y-3 border-t border-surface-800 px-3 pb-3 pt-3">
          <ol className="space-y-2.5">
            <Step n={1}>
              Open{' '}
              <a
                href={TOKENS_URL}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 font-medium text-accent-400 underline decoration-accent-400/40 hover:decoration-accent-400"
              >
                My Profile → API Tokens
                <ExternalLink className="h-3 w-3" />
              </a>{' '}
              and click <span className="text-slate-200">Create Token</span>.
            </Step>
            <Step n={2}>
              Scroll past the templates and choose{' '}
              <span className="text-slate-200">Create Custom Token</span>. The presets do not include tunnel access.
            </Step>
            <Step n={3}>
              <span className="text-slate-200">Token name</span>: anything you recognise, e.g.{' '}
              <code className="rounded bg-surface-800 px-1 py-0.5 font-mono text-[11px] text-slate-300">
                cf-multi-tunnel
              </code>
              .
            </Step>
            <Step n={4}>
              <span className="text-slate-200">Policy 1 — Account.</span> Click{' '}
              <span className="text-slate-200">+ Add new policy</span>, set its type to{' '}
              <span className="text-slate-200">Account</span>, then{' '}
              <span className="text-slate-200">Include → Specific account</span> and pick the account that owns your
              tunnels. In the permission list use the{' '}
              <span className="text-slate-200">“Search for permission groups…”</span> box and tick:
              <PermissionTable policy="Account" />
            </Step>
            <Step n={5}>
              <span className="text-slate-200">Policy 2 — Zone.</span> Click{' '}
              <span className="text-slate-200">+ Add new policy</span> again, set its type to{' '}
              <span className="text-slate-200">Zone</span>, then{' '}
              <span className="text-slate-200">Include → Specific zone</span> and pick the domain you publish to (add
              one more policy per domain, or use <span className="text-slate-200">All zones from an account</span>).
              Tick:
              <PermissionTable policy="Zone" />
            </Step>
            <Step n={6}>
              <span className="text-slate-200">Token expiration</span>: your choice —{' '}
              <span className="text-slate-200">No expiration</span> keeps the panel working until you roll a new token;
              a fixed date means you must remember to replace it. Leave{' '}
              <span className="text-slate-200">Client IP address filtering</span> empty unless the panel has a static
              egress IP.
            </Step>
            <Step n={7}>
              <span className="text-slate-200">Continue to summary → Create Token</span>, then copy the token and paste
              it above. <span className="text-amber-300/90">Cloudflare shows it only once</span> — if you lose it, roll a
              new one.
            </Step>
          </ol>

          <div className="space-y-1 rounded-md border border-amber-500/30 bg-amber-500/10 p-2.5 text-[11px] text-amber-200/90">
            <p className="flex items-center gap-1.5 font-medium text-amber-200">
              <AlertTriangle className="h-3.5 w-3.5" />
              Do not tick these
            </p>
            {TRAPS.map((t) => (
              <p key={t.wrong}>
                <span className="font-medium text-amber-100">{t.wrong}</span> — {t.why}
              </p>
            ))}
          </div>

          <div className="space-y-1 rounded-md border border-surface-800 bg-surface-900/40 p-2.5 text-[11px] text-slate-500">
            <p className="font-medium text-slate-400">What each permission is for</p>
            {PERMISSIONS.map((p) => (
              <p key={`why-${p.group}`}>
                <span className="text-slate-400">
                  {p.policy} · {p.group} · {p.toggle}
                </span>{' '}
                — {p.usedFor}
              </p>
            ))}
          </div>

          <div className="rounded-md border border-surface-800 bg-surface-900/40 p-2.5 text-[11px] text-slate-500">
            <p className="mb-1 font-medium text-slate-400">Where do I find the Account ID?</p>
            <p>
              It is in the URL of any dashboard page:{' '}
              <code className="font-mono text-slate-400">dash.cloudflare.com/&lt;account-id&gt;</code>, or in the right
              sidebar of a domain&apos;s Overview. The panel reads it from the token automatically; only fill the field
              when one token can see several accounts.
            </p>
          </div>
        </div>
      )}
    </div>
  );
}
