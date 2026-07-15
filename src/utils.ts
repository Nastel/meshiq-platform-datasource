import { ComboboxOption } from '@grafana/ui';

/**
 * Turns the backend's "<RepositoryName>$<OrganizationName>" identifier strings into Combobox
 * options. Combobox uses a flat option list with an optional `group` string; the org is attached
 * per option and only when more than one organization is present, so a single-org account yields a
 * plain ungrouped list.
 */
export function buildRepositoriesComboboxOptions(repositories: string[]): ComboboxOption[] {
  const orgs = new Set(repositories.map((repo) => repo.split('$')[1] ?? ''));
  const grouped = orgs.size > 1;

  return repositories.map((repo) => {
    const [name, org = ''] = repo.split('$');
    return grouped ? { label: name, value: repo, group: org } : { label: name, value: repo };
  });
}
