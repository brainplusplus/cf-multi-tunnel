import { useCallback, useMemo } from 'react';
import { useLanguage, type Language } from './LanguageContext';

import common from '../locales/common.json';
import nav from '../locales/nav.json';
import login from '../locales/login.json';
import dashboard from '../locales/dashboard.json';
import accounts from '../locales/accounts.json';
import tunnels from '../locales/tunnels.json';
import apps from '../locales/apps.json';
import users from '../locales/users.json';
import settings from '../locales/settings.json';
import audit from '../locales/audit.json';
import account from '../locales/account.json';
import tunnelDetail from '../locales/tunnel_detail.json';
import dns from '../locales/dns.json';

// A translation leaf is a pair, never a plain string: a missing language is a
// build error instead of a key that silently renders as English.
type TranslationLeaf = { id: string; en: string };
type TranslationNode = TranslationLeaf | { [key: string]: TranslationNode };
type TranslationRecord = Record<string, TranslationNode>;

// Every page gets its own module so two pages cannot fight over a key such as
// "title". Adding a page means adding a json file here, nothing else.
const locales = {
  common,
  nav,
  login,
  dashboard,
  accounts,
  tunnels,
  apps,
  users,
  settings,
  audit,
  account,
  tunnel_detail: tunnelDetail,
  dns,
} satisfies Record<string, TranslationRecord>;

export type LocaleModule = keyof typeof locales;

function isLeaf(value: TranslationNode | undefined): value is TranslationLeaf {
  // A leaf has BOTH id and en as strings. A nested key named "id" (e.g.
  // columns.id) only has "id", so it must not be mistaken for a leaf.
  if (typeof value !== 'object' || value === null) return false;
  const candidate = value as Record<string, unknown>;
  return typeof candidate.id === 'string' && typeof candidate.en === 'string';
}

function lookup(module: string, key: string): TranslationNode | undefined {
  const root = (locales as Record<string, TranslationRecord>)[module];
  if (!root) return undefined;
  let node: TranslationNode | undefined = root;
  for (const part of key.split('.')) {
    if (typeof node !== 'object' || node === null) return undefined;
    node = (node as Record<string, TranslationNode>)[part];
  }
  return node;
}

/**
 * t('columns.email') inside module "users" reads locales/users.json.
 *
 * A missing key returns the key itself and logs a warning in dev, so a typo is
 * visible instead of rendering "undefined". Interpolation uses {{name}}.
 */
export function useTranslation<M extends LocaleModule>(module: M) {
  const { language, setLanguage } = useLanguage();

  const t = useCallback(
    (key: string, params?: Record<string, string | number>) => {
      const entry = lookup(module, key);
      if (!isLeaf(entry)) {
        if (import.meta.env.DEV) {
          console.warn(`[i18n] missing key ${module}.${key}`);
        }
        return key;
      }
      let text: string = entry[language as Language] ?? entry.id;
      if (params) {
        for (const [name, value] of Object.entries(params)) {
          text = text.replace(new RegExp(`{{${name}}}`, 'g'), String(value));
        }
      }
      return text;
    },
    [module, language],
  );

  return useMemo(() => ({ t, language, setLanguage }), [t, language, setLanguage]);
}
