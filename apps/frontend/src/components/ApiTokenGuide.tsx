import { useState } from 'react';
import { ChevronDown, ExternalLink, ShieldCheck } from 'lucide-react';

const TOKENS_URL = 'https://dash.cloudflare.com/profile/api-tokens';

// The four scopes the panel actually uses. Kept in one place so the guide and
// the "why" list cannot drift from the backend's requirements.
const PERMISSIONS: { scope: string; resource: string; level: string; usedFor: string }[] = [
  {
    scope: 'Account',
    resource: 'Cloudflare Tunnel',
    level: 'Edit',
    usedFor: 'list, read, and rewrite each tunnel\'s ingress',
  },
  {
    scope: 'Account',
    resource: 'Account Settings',
    level: 'Read',
    usedFor: 'verify the token and resolve the account name',
  },
  {
    scope: 'Zone',
    resource: 'Zone',
    level: 'Read',
    usedFor: 'find the zone that owns a hostname',
  },
  {
    scope: 'Zone',
    resource: 'DNS',
    level: 'Edit',
    usedFor: 'create and update the CNAME pointing at the tunnel',
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

/**
 * Step-by-step instructions for creating the scoped Cloudflare API token the
 * panel needs. Collapsed by default so the form stays short, but the summary
 * line makes it obvious the guide exists.
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
              <span className="text-slate-200">Create Custom Token → Get started</span>. The presets do not include
              tunnel access.
            </Step>
            <Step n={3}>
              <span className="text-slate-200">Token name</span>: anything you recognise, e.g.{' '}
              <code className="rounded bg-surface-800 px-1 py-0.5 font-mono text-[11px] text-slate-300">
                cf-tunnel-panel
              </code>
              .
            </Step>
            <Step n={4}>
              Add these <span className="text-slate-200">Permissions</span> — each row is three dropdowns (group,
              resource, level):
              <div className="mt-2 overflow-hidden rounded-md border border-surface-800">
                <table className="w-full text-left text-[11px]">
                  <thead className="bg-surface-900/60 text-slate-500">
                    <tr>
                      <th className="px-2 py-1.5 font-medium">Group</th>
                      <th className="px-2 py-1.5 font-medium">Resource</th>
                      <th className="px-2 py-1.5 font-medium">Level</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-surface-800">
                    {PERMISSIONS.map((p) => (
                      <tr key={`${p.scope}-${p.resource}`}>
                        <td className="px-2 py-1.5 text-slate-400">{p.scope}</td>
                        <td className="px-2 py-1.5 text-slate-200">{p.resource}</td>
                        <td className="px-2 py-1.5 text-slate-200">{p.level}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Step>
            <Step n={5}>
              <span className="text-slate-200">Account Resources</span>: Include → the account that owns your tunnels.
              <span className="text-slate-200"> Zone Resources</span>: Include → All zones from that account (or only
              the zones you publish to).
            </Step>
            <Step n={6}>
              Leave <span className="text-slate-200">Client IP filtering</span> and{' '}
              <span className="text-slate-200">TTL</span> empty unless you need them, then{' '}
              <span className="text-slate-200">Continue to summary → Create Token</span>.
            </Step>
            <Step n={7}>
              Copy the token and paste it above.{' '}
              <span className="text-amber-300/90">Cloudflare shows it only once</span> — if you lose it, roll a new
              one.
            </Step>
          </ol>

          <div className="rounded-md border border-surface-800 bg-surface-900/40 p-2.5 text-[11px] text-slate-500">
            <p className="mb-1 font-medium text-slate-400">Where do I find the Account ID?</p>
            <p>
              It is in the URL of any dashboard page:{' '}
              <code className="font-mono text-slate-400">dash.cloudflare.com/&lt;account-id&gt;</code>, or in the right
              sidebar of a domain&apos;s Overview. The panel reads it from the token automatically; only fill the field
              when one token can see several accounts.
            </p>
          </div>

          <div className="space-y-1 rounded-md border border-surface-800 bg-surface-900/40 p-2.5 text-[11px] text-slate-500">
            <p className="font-medium text-slate-400">What each permission is for</p>
            {PERMISSIONS.map((p) => (
              <p key={`why-${p.scope}-${p.resource}`}>
                <span className="text-slate-400">
                  {p.scope} · {p.resource} · {p.level}
                </span>{' '}
                — {p.usedFor}
              </p>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
