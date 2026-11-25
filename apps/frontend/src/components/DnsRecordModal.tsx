import { useEffect, useState } from 'react';
import { apiFields, apiMessage } from '../api/client';
import { dnsApi } from '../api/endpoints';
import { Field, Modal } from './ui';
import { useToast } from './Toast';
import { useTranslation } from '../i18n/useTranslation';
import type { DNSRecord, DNSRecordPayload } from '../types';

const RECORD_TYPES = ['A', 'AAAA', 'CNAME', 'TXT', 'MX', 'NS', 'SRV', 'CAA'];
const PROXIABLE = new Set(['A', 'AAAA', 'CNAME']);
// 1 is Cloudflare's "Auto" TTL. The other values are the dashboard's presets;
// anything else has to be typed into Cloudflare directly.
const TTL_OPTIONS = [1, 60, 300, 1200, 3600, 86400];

// The API wants absolute names; the form shows the part before the zone so a
// user never has to retype the domain, matching Cloudflare's own DNS form.
function relativeName(full: string, zone: string): string {
  if (!zone) return full;
  if (full === zone) return '@';
  if (full.endsWith('.' + zone)) return full.slice(0, -(zone.length + 1));
  return full;
}

function initialName(record: DNSRecord | null, zone: string): string {
  if (!record) return '';
  return relativeName(record.name, zone);
}

interface FormState {
  type: string;
  name: string;
  content: string;
  ttl: string;
  proxied: boolean;
  priority: string;
  comment: string;
  service: string;
  protocol: string;
  weight: string;
  port: string;
  target: string;
  flags: string;
  tag: string;
  value: string;
}

function stateFor(record: DNSRecord | null, zone: string): FormState {
  if (!record) {
    return {
      type: 'A', name: '', content: '', ttl: '1', proxied: true,
      priority: '10', comment: '', service: '_sip', protocol: '_tcp',
      weight: '0', port: '', target: '', flags: '0', tag: 'issue', value: '',
    };
  }
  const labels = record.name.split('.');
  const srv = record.type === 'SRV';
  const caa = record.type === 'CAA';
  return {
    type: record.type,
    name: initialName(record, zone),
    content: srv || caa ? '' : record.content,
    ttl: String(record.ttl || 1),
    proxied: record.proxied,
    priority: String(record.priority ?? record.data?.priority ?? 10),
    comment: record.comment ?? '',
    service: srv ? labels[0] ?? '' : '_sip',
    protocol: srv ? labels[1] ?? '' : '_tcp',
    weight: String(record.data?.weight ?? 0),
    port: record.data?.port != null ? String(record.data.port) : '',
    target: srv ? record.data?.target ?? '' : '',
    flags: String(record.data?.flags ?? 0),
    tag: record.data?.tag ?? 'issue',
    value: caa ? record.data?.value ?? '' : '',
  };
}

function toNumber(value: string): number | undefined {
  if (value.trim() === '') return undefined;
  const n = Number(value);
  return Number.isFinite(n) ? n : undefined;
}

/**
 * Add/edit form for one DNS record. Fields are driven by the record type so the
 * form never asks for a value Cloudflare would reject: MX needs a priority, SRV
 * needs service/protocol/port, CAA needs flags/tag/value, TXT cannot be proxied.
 */
export function DnsRecordModal({
  open,
  zoneRowId,
  zoneName,
  editing,
  onClose,
  onSaved,
}: {
  open: boolean;
  zoneRowId: string;
  zoneName: string;
  editing: DNSRecord | null;
  onClose: () => void;
  onSaved: (record: DNSRecord, mode: 'create' | 'edit') => void;
}) {
  const { t } = useTranslation('dns');
  const { t: tc } = useTranslation('common');
  const toast = useToast();
  const [form, setForm] = useState<FormState>(() => stateFor(editing, zoneName));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [fields, setFields] = useState<Record<string, string>>({});

  // Re-seed whenever the modal opens: the same component is reused for "add"
  // and for editing a row picked from the table.
  useEffect(() => {
    if (!open) return;
    setForm(stateFor(editing, zoneName));
    setError('');
    setFields({});
  }, [open, editing, zoneName]);

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) =>
    setForm((prev) => ({ ...prev, [key]: value }));

  const proxiable = PROXIABLE.has(form.type);
  const isSrv = form.type === 'SRV';
  const isCaa = form.type === 'CAA';
  const needsPriority = form.type === 'MX' || isSrv;

  const submit = async () => {
    setBusy(true);
    setError('');
    setFields({});
    try {
      const payload: DNSRecordPayload = {
        type: form.type,
        ttl: Number(form.ttl),
        comment: form.comment.trim() || undefined,
      };
      if (proxiable) payload.proxied = form.proxied;

      if (isSrv) {
        payload.service = form.service.trim();
        payload.protocol = form.protocol.trim();
        payload.target = form.target.trim();
        payload.port = toNumber(form.port);
        payload.weight = toNumber(form.weight);
        payload.priority = toNumber(form.priority);
      } else {
        payload.name = form.name;
        payload.content = form.content;
      }
      if (needsPriority && !isSrv) payload.priority = toNumber(form.priority);
      if (isCaa) {
        payload.flags = toNumber(form.flags);
        payload.tag = form.tag.trim();
        payload.value = form.value;
      }

      const saved = editing
        ? await dnsApi.updateRecord(zoneRowId, editing.id, payload)
        : await dnsApi.createRecord(zoneRowId, payload);
      toast.push('success', t(editing ? 'updated' : 'created', { name: saved.name }));
      onSaved(saved, editing ? 'edit' : 'create');
    } catch (err) {
      setError(apiMessage(err));
      setFields(apiFields(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t(editing ? 'modal.edit_title' : 'modal.create_title')}
      description={t(editing ? 'modal.edit_desc' : 'modal.create_desc')}
      footer={
        <div className="flex justify-end gap-2">
          <button className="btn-ghost" onClick={onClose} disabled={busy}>
            {tc('cancel')}
          </button>
          <button className="btn-primary" onClick={() => void submit()} disabled={busy}>
            {t('modal.submit')}
          </button>
        </div>
      }
    >
      <div className="space-y-3">
        {error && <div className="rounded-lg border border-rose-500/30 bg-rose-950/40 px-3 py-2 text-xs text-rose-200">{error}</div>}

        <div className="grid grid-cols-2 gap-3">
          <Field label={t('fields.type')} error={fields.type}>
            <select
              className="field"
              value={form.type}
              disabled={Boolean(editing)}
              onChange={(e) => set('type', e.target.value)}
            >
              {RECORD_TYPES.map((type) => (
                <option key={type} value={type}>
                  {type}
                </option>
              ))}
            </select>
          </Field>
          <Field label={t('fields.ttl')}>
            <select className="field" value={form.ttl} onChange={(e) => set('ttl', e.target.value)}>
              {TTL_OPTIONS.map((ttl) => (
                <option key={ttl} value={ttl}>
                  {ttl === 1 ? t('ttl.auto') : t('ttl.seconds', { count: ttl })}
                </option>
              ))}
            </select>
          </Field>
        </div>

        {isSrv ? (
          <div className="grid grid-cols-2 gap-3">
            <Field label={t('fields.service')} hint={t('fields.service_hint')} error={fields.service}>
              <input className="field" value={form.service} onChange={(e) => set('service', e.target.value)} placeholder="_sip" />
            </Field>
            <Field label={t('fields.protocol')} error={fields.service}>
              <select className="field" value={form.protocol} onChange={(e) => set('protocol', e.target.value)}>
                <option value="_tcp">_tcp</option>
                <option value="_udp">_udp</option>
                <option value="_tls">_tls</option>
              </select>
            </Field>
            <Field label={t('fields.priority')} error={fields.priority}>
              <input className="field" value={form.priority} onChange={(e) => set('priority', e.target.value)} inputMode="numeric" />
            </Field>
            <Field label={t('fields.weight')} error={fields.weight}>
              <input className="field" value={form.weight} onChange={(e) => set('weight', e.target.value)} inputMode="numeric" />
            </Field>
            <Field label={t('fields.port')} error={fields.port}>
              <input className="field" value={form.port} onChange={(e) => set('port', e.target.value)} inputMode="numeric" />
            </Field>
            <Field label={t('fields.target')} error={fields.target}>
              <input className="field" value={form.target} onChange={(e) => set('target', e.target.value)} placeholder="sip.example.com" />
            </Field>
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-3">
            <Field label={t('fields.name')} hint={t('fields.name_hint')} error={fields.name}>
              <div className="flex items-center gap-1">
                <input className="field" value={form.name} onChange={(e) => set('name', e.target.value)} placeholder="app" />
                <span className="whitespace-nowrap text-[11px] text-slate-500">.{zoneName}</span>
              </div>
            </Field>
            {isCaa ? (
              <Field label={t('fields.tag')} error={fields.tag}>
                <select className="field" value={form.tag} onChange={(e) => set('tag', e.target.value)}>
                  <option value="issue">issue</option>
                  <option value="issuewild">issuewild</option>
                  <option value="iodef">iodef</option>
                </select>
              </Field>
            ) : (
              <Field
                label={t('fields.content')}
                hint={
                  form.type === 'A'
                    ? t('fields.content_a')
                    : form.type === 'AAAA'
                      ? t('fields.content_aaaa')
                      : form.type === 'CNAME'
                        ? t('fields.content_cname')
                        : form.type === 'NS'
                          ? t('fields.content_ns')
                          : form.type === 'MX'
                            ? t('fields.content_mx')
                            : t('fields.content_txt')
                }
                error={fields.content}
              >
                <input className="field" value={form.content} onChange={(e) => set('content', e.target.value)} />
              </Field>
            )}
            {needsPriority && (
              <Field label={t('fields.priority')} error={fields.priority}>
                <input className="field" value={form.priority} onChange={(e) => set('priority', e.target.value)} inputMode="numeric" />
              </Field>
            )}
            {isCaa && (
              <>
                <Field label={t('fields.flags')} error={fields.flags}>
                  <input className="field" value={form.flags} onChange={(e) => set('flags', e.target.value)} inputMode="numeric" />
                </Field>
                <Field label={t('fields.value')} error={fields.value}>
                  <input className="field" value={form.value} onChange={(e) => set('value', e.target.value)} placeholder="letsencrypt.org" />
                </Field>
              </>
            )}
          </div>
        )}

        {proxiable && (
          <label className="flex items-start gap-2 rounded-lg border border-surface-700 bg-surface-950/30 px-3 py-2">
            <input type="checkbox" className="mt-0.5" checked={form.proxied} onChange={(e) => set('proxied', e.target.checked)} />
            <span>
              <span className="block text-xs font-medium text-slate-200">{t('fields.proxied')}</span>
              <span className="block text-[11px] text-slate-500">{t('fields.proxied_hint')}</span>
            </span>
          </label>
        )}

        <Field label={t('fields.comment')} hint={t('fields.comment_hint')}>
          <input className="field" value={form.comment} onChange={(e) => set('comment', e.target.value)} />
        </Field>
      </div>
    </Modal>
  );
}
