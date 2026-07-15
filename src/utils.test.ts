import { buildRepositoriesComboboxOptions } from './utils';

describe('buildRepositoriesComboboxOptions', () => {
  it('returns an ungrouped list when every repository belongs to the same organization', () => {
    const options = buildRepositoriesComboboxOptions(['Default$Org', 'Other$Org']);

    expect(options).toEqual([
      { label: 'Default', value: 'Default$Org' },
      { label: 'Other', value: 'Other$Org' },
    ]);
  });

  it('groups by organization once more than one is present', () => {
    const options = buildRepositoriesComboboxOptions(['Default$Org', 'Special$OtherOrg']);

    expect(options).toEqual([
      { label: 'Default', value: 'Default$Org', group: 'Org' },
      { label: 'Special', value: 'Special$OtherOrg', group: 'OtherOrg' },
    ]);
  });

  it('returns an empty list for an empty input', () => {
    expect(buildRepositoriesComboboxOptions([])).toEqual([]);
  });

  it('treats a missing organization suffix as its own group when other repositories have one', () => {
    const options = buildRepositoriesComboboxOptions(['Default$Org', 'NoOrgSuffix']);

    expect(options).toEqual([
      { label: 'Default', value: 'Default$Org', group: 'Org' },
      { label: 'NoOrgSuffix', value: 'NoOrgSuffix', group: '' },
    ]);
  });
});
