import { Filter, FolderPlus, Plus, Trash2 } from 'lucide-react';
import {
  OPERATORS,
  countRules,
  createGroup,
  createRule,
  isGroup,
  type FieldDefinition,
  type QueryGroup,
  type QueryNode,
  type QueryRule,
} from './query';
import { useTranslation } from '../i18n/useTranslation';

interface QueryBuilderProps {
  fields: FieldDefinition[];
  value: QueryGroup;
  onChange: (next: QueryGroup) => void;
}

/**
 * Nested AND/OR filter editor.
 *
 * The parent owns the QueryGroup so it can reset the table to page 1 and feed
 * the same object to matchesQuery; this component only edits the tree.
 */
export function QueryBuilder({ fields, value, onChange }: QueryBuilderProps) {
  return (
    <div className="rounded-xl border border-surface-700 bg-surface-950/40 p-3">
      <GroupEditor fields={fields} group={value} onChange={onChange} depth={0} />
    </div>
  );
}

/** The button + badge that shows and hides the panel. */
export function FilterToggle({
  open,
  count,
  onClick,
}: {
  open: boolean;
  count: number;
  onClick: () => void;
}) {
  const { t } = useTranslation('common');
  return (
    <button
      className={`btn-ghost !py-1.5 !text-xs ${open ? '!border-accent-500/60 !text-accent-300' : ''}`}
      onClick={onClick}
    >
      <Filter className="h-4 w-4" /> {t('filters')}
      {count > 0 && (
        <span className="rounded bg-accent-500/20 px-1.5 py-0.5 font-mono text-[10px] text-accent-300">{count}</span>
      )}
    </button>
  );
}

export function filterCount(group: QueryGroup): number {
  return countRules(group);
}

function GroupEditor({
  fields,
  group,
  onChange,
  depth,
}: {
  fields: FieldDefinition[];
  group: QueryGroup;
  onChange: (next: QueryGroup) => void;
  depth: number;
}) {
  const { t } = useTranslation('common');

  const updateChild = (id: string, next: QueryNode) => {
    onChange({ ...group, children: group.children.map((c) => (c.id === id ? next : c)) });
  };
  const removeChild = (id: string) => {
    onChange({ ...group, children: group.children.filter((c) => c.id !== id) });
  };
  const addRule = () => {
    onChange({ ...group, children: [...group.children, createRule(fields[0]?.name ?? '')] });
  };
  const addGroup = () => {
    onChange({ ...group, children: [...group.children, createGroup(group.logic === 'AND' ? 'OR' : 'AND')] });
  };

  return (
    <div className={depth > 0 ? 'mt-2 rounded-lg border border-dashed border-surface-600 bg-surface-900/40 p-2.5' : ''}>
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <div className="inline-flex overflow-hidden rounded-md border border-surface-600">
          {(['AND', 'OR'] as const).map((logic) => (
            <button
              key={logic}
              onClick={() => onChange({ ...group, logic })}
              className={`px-2.5 py-1 text-[11px] font-medium ${
                group.logic === logic ? 'bg-accent-500/20 text-accent-300' : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              {logic === 'AND' ? t('filter_and') : t('filter_or')}
            </button>
          ))}
        </div>
        <button className="btn-ghost !px-2 !py-1 !text-[11px]" onClick={addRule}>
          <Plus className="h-3 w-3" /> {t('filter_add_rule')}
        </button>
        <button className="btn-ghost !px-2 !py-1 !text-[11px]" onClick={addGroup}>
          <FolderPlus className="h-3 w-3" /> {t('filter_add_group')}
        </button>
        {depth === 0 && group.children.length > 0 && (
          <button
            className="ml-auto text-[11px] text-slate-500 hover:text-rose-300"
            onClick={() => onChange(createGroup(group.logic))}
          >
            {t('filter_clear')}
          </button>
        )}
      </div>

      {group.children.length === 0 ? (
        <p className="text-xs text-slate-500">{t('filter_empty')}</p>
      ) : (
        <div className="space-y-2">
          {group.children.map((child) =>
            isGroup(child) ? (
              <div key={child.id} className="flex items-start gap-2">
                <div className="min-w-0 flex-1">
                  <GroupEditor
                    fields={fields}
                    group={child}
                    onChange={(next) => updateChild(child.id, next)}
                    depth={depth + 1}
                  />
                </div>
                <button
                  className="btn-ghost !px-1.5 !py-1"
                  onClick={() => removeChild(child.id)}
                  title={t('filter_remove_group')}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            ) : (
              <RuleEditor
                key={child.id}
                fields={fields}
                rule={child}
                onChange={(next) => updateChild(child.id, next)}
                onRemove={() => removeChild(child.id)}
              />
            ),
          )}
        </div>
      )}
    </div>
  );
}

function RuleEditor({
  fields,
  rule,
  onChange,
  onRemove,
}: {
  fields: FieldDefinition[];
  rule: QueryRule;
  onChange: (next: QueryRule) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation('common');
  const field = fields.find((f) => f.name === rule.field);
  const operators = OPERATORS.filter((op) => !field || op.types.includes(field.type));

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <select
        className="field !w-auto !py-1 !text-xs"
        value={rule.field}
        onChange={(e) => {
          // Switching field can invalidate the operator ("> " on a text field),
          // so fall back to "=" rather than leaving an impossible pair.
          const nextField = fields.find((f) => f.name === e.target.value);
          const allowed = OPERATORS.filter((op) => !nextField || op.types.includes(nextField.type));
          const operator = allowed.some((op) => op.value === rule.operator) ? rule.operator : allowed[0]?.value ?? '=';
          onChange({ ...rule, field: e.target.value, operator, value: '' });
        }}
      >
        {fields.map((f) => (
          <option key={f.name} value={f.name}>
            {f.label}
          </option>
        ))}
      </select>

      <select
        className="field !w-auto !py-1 !text-xs"
        value={rule.operator}
        onChange={(e) => onChange({ ...rule, operator: e.target.value })}
      >
        {operators.map((op) => (
          <option key={op.value} value={op.value}>
            {t(`operator.${op.value}`)}
          </option>
        ))}
      </select>

      {field?.type === 'select' && field.options ? (
        <select
          className="field !w-auto !py-1 !text-xs"
          value={rule.value}
          onChange={(e) => onChange({ ...rule, value: e.target.value })}
        >
          <option value="">—</option>
          {field.options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      ) : (
        <input
          className="field !w-40 !py-1 !text-xs"
          type={field?.type === 'number' ? 'number' : field?.type === 'date' ? 'date' : 'text'}
          placeholder={t('filter_value')}
          value={rule.value}
          onChange={(e) => onChange({ ...rule, value: e.target.value })}
        />
      )}

      <button className="btn-ghost !px-1.5 !py-1" onClick={onRemove} title={t('filter_remove_rule')}>
        <Trash2 className="h-3.5 w-3.5" />
      </button>
    </div>
  );
}
