import type { Monaco, monacoTypes } from '@grafana/ui';

import { MeshIqCompletionItem } from './types';

/** Monaco language id used for the jKQL editor. */
export const JKQL_LANGUAGE_ID = 'jkql';

// Keywords for the GET-statement subset the plugin supports. Non-GET statements
// (INSERT/UPDATE/DELETE/…) and all date/time *filter* keywords are intentionally excluded — Grafana's
// time range drives the time window instead. The time *units* below are kept only because
// `GROUP BY <timeField> BUCKETED BY <n> <unit>` is how time-series are built. jKQL is case-insensitive,
// so these are matched with ignoreCase.
const JKQL_KEYWORDS = [
  // statement + aggregate
  'GET', 'THE', 'NUMBER', 'COUNT', 'PERCENT', 'OF', 'DISTINCT',
  // limit
  'TOP', 'BOTTOM', 'FIRST', 'LAST', 'LATEST', 'EARLIEST', 'BEST', 'WORST', 'LARGEST', 'SMALLEST', 'LONGEST', 'SHORTEST',
  // fields / projection
  'FIELDS', 'ALL', 'AS',
  // filter
  'WHERE', 'AND', 'OR', 'NOT', 'IN', 'CONTAINS', 'STARTS', 'WITH', 'ENDS', 'MATCHES', 'EXISTS', 'BETWEEN', 'EQUALS',
  // group by
  'GROUP', 'BY', 'HAVING', 'BUCKETED', 'SIZE', 'INCLUDE', 'NULLS', 'NULL', 'EMPTY', 'TRIM', 'NONE',
  // sort + range
  'SORT', 'ORDER', 'ASC', 'DESC', 'RANGE',
  // literals
  'TRUE', 'FALSE',
  // time units (GROUP BY … BUCKETED BY <n> <unit> only)
  'SECOND', 'SECONDS', 'MINUTE', 'MINUTES', 'HOUR', 'HOURS', 'DAY', 'DAYS', 'WEEK', 'WEEKS', 'MONTH', 'MONTHS',
];

// Monarch tokenizer: keyword/function/string/number/operator/variable coloring for the GET subset.
const jkqlMonarchLanguage: monacoTypes.languages.IMonarchLanguage = {
  ignoreCase: true,
  defaultToken: '',
  keywords: JKQL_KEYWORDS,
  tokenizer: {
    root: [
      // Grafana template variables: ${var}, [[var]], $var
      [/\$\{[^}]*\}/, 'variable'],
      [/\[\[[^\]]*\]\]/, 'variable'],
      [/\$[a-zA-Z0-9_]+/, 'variable'],

      // strings (single- or double-quoted, backslash escapes)
      [/"([^"\\]|\\.)*"/, 'string'],
      [/'([^'\\]|\\.)*'/, 'string'],

      // numbers: decimals, and ints with an optional size suffix (5K, 2M, 1G)
      [/\d+\.\d+/, 'number.float'],
      [/\d+[KMGT]?\b/, 'number'],

      // function call: a name immediately followed by "(" (e.g. avg(...), Properties('...'))
      [/[a-zA-Z_]\w*(?=\s*\()/, 'type'],

      // identifiers vs. keywords
      [/[a-zA-Z_]\w*/, { cases: { '@keywords': 'keyword', '@default': 'identifier' } }],

      // operators
      [/<=|>=|!=|<>|~|=|<|>/, 'operator'],
      [/[+\-*/%]/, 'operator'],

      // brackets & separators
      [/[()]/, '@brackets'],
      [/,/, 'delimiter'],

      [/\s+/, 'white'],
    ],
  },
};

// Bracket matching + auto-closing. The GET subset only uses parentheses (function args, IN lists,
// grouped conditions) and quotes; no [] or {} constructs to close.
const jkqlLanguageConfiguration: monacoTypes.languages.LanguageConfiguration = {
  brackets: [['(', ')']],
  autoClosingPairs: [
    { open: '(', close: ')' },
    { open: "'", close: "'" },
    { open: '"', close: '"' },
  ],
  surroundingPairs: [
    { open: '(', close: ')' },
    { open: "'", close: "'" },
    { open: '"', close: '"' },
  ],
};

/** Resolves completions for the text up to a caret offset. */
export type SuggestionResolver = (text: string, caretIndex: number) => Promise<MeshIqCompletionItem[]>;

// The completion provider is registered once per Monaco instance and delegates to whichever
// resolver is currently active (set by the focused query editor). This keeps a single global
// provider instead of stacking a duplicate one per editor mount. The backend returns an empty
// list when completion is disabled, so the provider registers unconditionally and just relies on
// that — no separate "is completion enabled" branch here.
let activeResolver: SuggestionResolver | null = null;

export function setJkqlCompletionHandler(resolver: SuggestionResolver): void {
  activeResolver = resolver;
}

/**
 * Clears the active resolver only when it still belongs to the caller. An unmounting editor must
 * not clear a sibling's resolver: with two query rows mounted, deleting row B must not kill
 * completion in the still-focused row A.
 */
export function clearJkqlCompletionHandler(resolver: SuggestionResolver | null): void {
  if (resolver && activeResolver === resolver) {
    activeResolver = null;
  }
}

/**
 * Registers the jKQL language on the given Monaco instance, exactly once: syntax highlighting
 * (Monarch tokenizer), bracket/quote behavior (language configuration), and the completion provider.
 * Safe to call from every editor's onBeforeEditorMount.
 */
export function registerJkqlLanguage(monaco: Monaco): void {
  const alreadyRegistered = monaco.languages.getLanguages().some((lang) => lang.id === JKQL_LANGUAGE_ID);
  if (alreadyRegistered) {
    return;
  }

  monaco.languages.register({ id: JKQL_LANGUAGE_ID });
  monaco.languages.setMonarchTokensProvider(JKQL_LANGUAGE_ID, jkqlMonarchLanguage);
  monaco.languages.setLanguageConfiguration(JKQL_LANGUAGE_ID, jkqlLanguageConfiguration);

  monaco.languages.registerCompletionItemProvider(JKQL_LANGUAGE_ID, {
    triggerCharacters: [' ', '.', ',', '(', "'"],
    provideCompletionItems: async (model, position, _context, token) => {
      if (!activeResolver) {
        return { suggestions: [] };
      }
      // Debounce: every trigger character (even plain space) would otherwise fire its own
      // backend round-trip; only the position the caret settles on is worth a request.
      await debounce(COMPLETION_DEBOUNCE_MS, token);
      if (token.isCancellationRequested) {
        return { suggestions: [] };
      }

      const caretIndex = model.getOffsetAt(position);
      const items = await activeResolver(model.getValue(), caretIndex);
      // The caret may have moved again (or the editor lost focus) while the request was in
      // flight; Monaco discards a canceled provider's result anyway, but skip building the
      // suggestion list against a position that's no longer current.
      if (token.isCancellationRequested) {
        return { suggestions: [] };
      }
      return { suggestions: items.map((item) => toMonacoSuggestion(monaco, model, position, item)) };
    },
  });
}

/** How long provideCompletionItems waits for typing to settle before calling the resolver. */
const COMPLETION_DEBOUNCE_MS = 200;

/** Resolves after ms, or immediately once token is canceled — whichever comes first. */
function debounce(ms: number, token: monacoTypes.CancellationToken): Promise<void> {
  return new Promise((resolve) => {
    if (token.isCancellationRequested) {
      resolve();
      return;
    }
    const timer = setTimeout(() => {
      subscription.dispose();
      resolve();
    }, ms);
    const subscription = token.onCancellationRequested(() => {
      clearTimeout(timer);
      subscription.dispose();
      resolve();
    });
  });
}

// toMonacoSuggestion converts a service CompletionItem into a Monaco suggestion, honoring the
// service's insertText and deleteBackwards (how many characters before the caret to replace).
function toMonacoSuggestion(
  monaco: Monaco,
  model: monacoTypes.editor.ITextModel,
  position: monacoTypes.Position,
  item: MeshIqCompletionItem
): monacoTypes.languages.CompletionItem {
  const range =
    item.deleteBackwards && item.deleteBackwards > 0
      ? new monaco.Range(
          position.lineNumber,
          Math.max(1, position.column - item.deleteBackwards),
          position.lineNumber,
          position.column
        )
      : wordRange(monaco, model, position);

  return {
    label: item.label,
    kind: mapKind(monaco, item.kind),
    insertText: item.insertText ?? item.label,
    range,
  };
}

// wordRange replaces the word currently under the caret, so typing a prefix and picking a suggestion
// doesn't duplicate the prefix.
function wordRange(monaco: Monaco, model: monacoTypes.editor.ITextModel, position: monacoTypes.Position): monacoTypes.IRange {
  const word = model.getWordUntilPosition(position);
  return new monaco.Range(position.lineNumber, word.startColumn, position.lineNumber, word.endColumn);
}

// mapKind maps the service's CompletionItemKind enum names to Monaco kinds (which pick the icon).
function mapKind(monaco: Monaco, kind?: string): monacoTypes.languages.CompletionItemKind {
  const Kind = monaco.languages.CompletionItemKind;
  switch (kind) {
    case 'StatementType':
    case 'Keyword':
      return Kind.Keyword;
    case 'ItemType':
      return Kind.Class;
    case 'Field':
      return Kind.Field;
    case 'Function':
      return Kind.Function;
    case 'Operator':
      return Kind.Operator;
    case 'Limit':
      return Kind.Enum;
    case 'Totals':
      return Kind.Value;
    case 'Separator':
    case 'Token':
    default:
      return Kind.Text;
  }
}
