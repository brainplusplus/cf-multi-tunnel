/**
 * Filter model shared by every list page, ported from saas-rus so a filter UI
 * behaves the same in both projects.
 *
 * Filters are evaluated on the client: the panel's lists are small, and keeping
 * the predicate here means the same code filters a table that was already
 * fetched instead of teaching every endpoint a query language.
 */

export type FieldType = 'string' | 'number' | 'boolean' | 'select' | 'date';

export interface SelectOption {
  value: string;
  label: string;
}

export interface FieldDefinition {
  name: string;
  label: string;
  type: FieldType;
  options?: SelectOption[];
}

export interface QueryRule {
  id: string;
  type: 'rule';
  field: string;
  operator: string;
  value: string;
}

export interface QueryGroup {
  id: string;
  type: 'group';
  logic: 'AND' | 'OR';
  children: (QueryRule | QueryGroup)[];
}

export type QueryNode = QueryRule | QueryGroup;

export const OPERATORS: { value: string; label: string; types: FieldType[] }[] = [
  { value: '=', label: 'equals', types: ['string', 'number', 'select', 'date'] },
  { value: '!=', label: 'is not', types: ['string', 'number', 'select', 'date'] },
  { value: 'contains', label: 'contains', types: ['string'] },
  { value: 'not_contains', label: 'does not contain', types: ['string'] },
  { value: '>', label: 'greater than', types: ['number', 'date'] },
  { value: '>=', label: 'at least', types: ['number', 'date'] },
  { value: '<', label: 'less than', types: ['number', 'date'] },
  { value: '<=', label: 'at most', types: ['number', 'date'] },
];

const generateId = () => Math.random().toString(36).slice(2, 9);

export const createRule = (field = ''): QueryRule => ({
  id: generateId(),
  type: 'rule',
  field,
  operator: '=',
  value: '',
});

// Starts empty on purpose: a filter panel that opens with a blank rule already
// applied hides rows for no reason the operator can see.
export const createGroup = (logic: 'AND' | 'OR' = 'AND'): QueryGroup => ({
  id: generateId(),
  type: 'group',
  logic,
  children: [],
});

export function isGroup(node: QueryNode): node is QueryGroup {
  return (node as QueryGroup).children !== undefined;
}

/** How many rules are actually filtering, for the badge on the Filters button. */
export function countRules(group: QueryGroup): number {
  return group.children.reduce((sum, child) => sum + (isGroup(child) ? countRules(child) : 1), 0);
}

function matchesRule(item: Record<string, unknown>, rule: QueryRule): boolean {
  const raw = item[rule.field];
  const actual = raw === null || raw === undefined ? '' : String(raw).toLowerCase();
  const expected = rule.value.toLowerCase();

  switch (rule.operator) {
    case '=':
      return actual === expected;
    case '!=':
      return actual !== expected;
    case 'contains':
      return actual.includes(expected);
    case 'not_contains':
      return !actual.includes(expected);
    case '>':
    case '>=':
    case '<':
    case '<=': {
      const a = Number(actual);
      const b = Number(expected);
      if (Number.isNaN(a) || Number.isNaN(b)) return false;
      if (rule.operator === '>') return a > b;
      if (rule.operator === '>=') return a >= b;
      if (rule.operator === '<') return a < b;
      return a <= b;
    }
    default:
      return true;
  }
}

/** True when the row satisfies the group. An empty group matches everything. */
export function matchesQuery(item: Record<string, unknown>, group: QueryGroup): boolean {
  const predicates: boolean[] = [];

  for (const child of group.children) {
    if (isGroup(child)) {
      // A nested group with no rules is inert, not a filter that matches all.
      if (child.children.length > 0) predicates.push(matchesQuery(item, child));
      continue;
    }
    // A rule the operator has not finished typing must not hide every row.
    if (!child.field || child.value === '') continue;
    predicates.push(matchesRule(item, child));
  }

  if (predicates.length === 0) return true;
  return group.logic === 'AND' ? predicates.every(Boolean) : predicates.some(Boolean);
}
