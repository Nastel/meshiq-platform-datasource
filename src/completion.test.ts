import type { Monaco, monacoTypes } from '@grafana/ui';

import { clearJkqlCompletionHandler, registerJkqlLanguage, setJkqlCompletionHandler, SuggestionResolver } from './completion';

// A minimal fake Monaco good enough to drive registerCompletionItemProvider's callback directly —
// the unit under test is the provider function itself, not a real Monaco editor instance.
function makeFakeMonaco() {
  let provider: monacoTypes.languages.CompletionItemProvider | undefined;
  const monaco = {
    languages: {
      getLanguages: () => [],
      register: () => {},
      setMonarchTokensProvider: () => {},
      setLanguageConfiguration: () => {},
      registerCompletionItemProvider: (_id: string, p: monacoTypes.languages.CompletionItemProvider) => {
        provider = p;
      },
      CompletionItemKind: { Text: 0 },
    },
    Range: class {
      constructor(
        public startLineNumber: number,
        public startColumn: number,
        public endLineNumber: number,
        public endColumn: number
      ) {}
    },
  } as unknown as Monaco;
  return { monaco, getProvider: () => provider! };
}

function makeModel(text = ''): monacoTypes.editor.ITextModel {
  return {
    getOffsetAt: () => 0,
    getValue: () => text,
    getWordUntilPosition: () => ({ startColumn: 1, endColumn: 1, word: '' }),
  } as unknown as monacoTypes.editor.ITextModel;
}

function makeToken(): monacoTypes.CancellationToken {
  return {
    isCancellationRequested: false,
    onCancellationRequested: () => ({ dispose: () => {} }),
  } as unknown as monacoTypes.CancellationToken;
}

describe('registerJkqlLanguage completion provider', () => {
  afterEach(() => {
    jest.useRealTimers();
  });

  it('returns no suggestions when the active resolver goes null while the debounce is pending', async () => {
    jest.useFakeTimers();
    const { monaco, getProvider } = makeFakeMonaco();
    registerJkqlLanguage(monaco);
    const provider = getProvider();

    const resolver: SuggestionResolver = jest.fn().mockResolvedValue([{ label: 'Get' }]);
    setJkqlCompletionHandler(resolver);

    const resultPromise = provider.provideCompletionItems(
      makeModel('Ge'),
      { lineNumber: 1, column: 3 } as monacoTypes.Position,
      {} as monacoTypes.languages.CompletionContext,
      makeToken()
    );

    // The resolver is cleared (e.g. the owning editor unmounted) while the debounce is still
    // pending — before this fix, the provider would still call the now-null resolver after the
    // debounce resolved, throwing inside the async callback.
    clearJkqlCompletionHandler(resolver);
    jest.runAllTimers();

    const result = await resultPromise;
    expect(result).toEqual({ suggestions: [] });
    expect(resolver).not.toHaveBeenCalled();
  });

  it('still resolves suggestions normally when the resolver stays active through the debounce', async () => {
    jest.useFakeTimers();
    const { monaco, getProvider } = makeFakeMonaco();
    registerJkqlLanguage(monaco);
    const provider = getProvider();

    const resolver: SuggestionResolver = jest.fn().mockResolvedValue([{ label: 'Get' }]);
    setJkqlCompletionHandler(resolver);

    const resultPromise = provider.provideCompletionItems(
      makeModel('Ge'),
      { lineNumber: 1, column: 3 } as monacoTypes.Position,
      {} as monacoTypes.languages.CompletionContext,
      makeToken()
    );

    jest.runAllTimers();

    const result = await resultPromise;
    expect(resolver).toHaveBeenCalledWith('Ge', 0);
    expect((result as { suggestions: unknown[] }).suggestions).toHaveLength(1);

    clearJkqlCompletionHandler(resolver);
  });
});
