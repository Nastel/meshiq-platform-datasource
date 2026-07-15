# Changelog

## 1.0.0 (Unreleased)

Initial release.

- jKQL query editor with syntax highlighting, run on Ctrl/Cmd+Enter or blur, and optional
  autocomplete served by the meshIQ completion service.
- Table and time series result formats; time-bucketed results pivot into graphable series.
- Full result-set conversion: scalars, arrays, enums (with value coloring), label sets,
  variants, maps (exploded or as JSON), ranges, and custom properties.
- Repository selection per data source and per query.
- Template variables from jKQL queries, item types, or field lists; multi-value variables
  expand to quoted jKQL lists.
- Backend alerting and annotation queries.
- Health check reporting the server's version and row limit.
- Access token sent as the `X-API-Key` header; queries run concurrently and honor panel
  cancellation.
