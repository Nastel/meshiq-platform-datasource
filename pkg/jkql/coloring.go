package jkql

// This file is the single place to configure value coloring. To change or add colors, edit the
// palettes and rules below — the frame-building code does not need to change.
//
// Enum coloring is SCOPED: only enum fields listed in enumColorRules are colored (e.g. an
// unrelated enum keeps Grafana's automatic palette).
//
// String/labelset coloring uses a specific rule when one exists, otherwise falls back to the
// default two-state palette (binaryStateColors). Because that palette's values are specific
// (Yes/No, Enabled/Disabled, …), unrelated columns simply match nothing and stay uncolored.
//
// Two kinds of rules, matching the two Grafana mechanisms:
//   - enumColorRules:   enum field name         -> palette  (builds the enum color table)
//   - stringColorRules: (item type, field name) -> palette  (builds string/labelset value mappings)
//
// Everything set here is a default that travels with the query result, so colors work with no
// panel setup. Panel field config / overrides can still change them.

import (
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// colorByValue maps a value name to a Grafana color. Lookups are case-insensitive.
type colorByValue map[string]string

func (c colorByValue) color(value string) string {
	return c[strings.ToUpper(strings.TrimSpace(value))]
}

// itemField identifies one field within one item type. Both parts are compared case-insensitively
// (stored upper-cased by the rule lookup).
type itemField struct {
	item  string
	field string
}

// Grafana named-palette color tokens. Grafana resolves these through the active theme
// (theme.visualization.getColorByName), so they adapt to light/dark and match the same colors
// used elsewhere in Grafana (thresholds, the color picker). Change a value here to recolor
// everywhere at once; see https://grafana.com/docs for the full named palette (green, red,
// orange, yellow, blue, purple, plus dark-/semi-dark-/light-/super-light- variants).
const (
	colorGreen  = "semi-dark-green"
	colorBlue   = "semi-dark-blue"
	colorOrange = "semi-dark-orange"
	colorRed    = "semi-dark-red"
	// colorNeutral is Grafana's theme text color — for states that are neither good nor bad
	// (e.g. an INACTIVE channel is normal, not a failure), so they don't draw attention.
	colorNeutral = "text"
	// colorGray is a plain, non-theme-adaptive gray (Grafana's named palette has no gray hue,
	// so this falls back to the CSS color name) — for states that are unclear/not yet known,
	// as opposed to colorNeutral's "known and not a problem" meaning. Orange reads as a warning,
	// which is wrong for "we simply don't know yet".
	colorGray = "gray"
)

// ---------------------------------------------------------------------------
// Palettes — a color scheme for a set of value names. Reused across rules.
// ---------------------------------------------------------------------------

// severityColors follows the jKQL Severity levels (tnt4j OpLevel). Meaning, not ordinal: normal is
// green, informational is blue, warning is yellow, every error level is red.
var severityColors = colorByValue{
	"NONE":     colorGreen,
	"TRACE":    colorBlue,
	"DEBUG":    colorBlue,
	"INFO":     colorBlue,
	"NOTICE":   colorBlue,
	"WARNING":  colorOrange,
	"ERROR":    colorRed,
	"FAILURE":  colorRed,
	"CRITICAL": colorRed,
	"FATAL":    colorRed,
	"HALT":     colorRed,
}

// compCodeColors follows the jKQL CompCode values (SUCCESS/WARNING/ERROR).
var compCodeColors = colorByValue{
	"SUCCESS": colorGreen,
	"WARNING": colorOrange,
	"ERROR":   colorRed,
}

// binaryStateColors is a reusable palette for two-state string fields: the positive state is green,
// the negative state is red. Case-insensitive. These are the only values we expect for now.
var binaryStateColors = colorByValue{
	"YES":       colorGreen,
	"NO":        colorRed,
	"ALLOWED":   colorGreen,
	"DENIED":    colorRed,
	"INHIBITED": colorRed,
	"ENABLED":   colorGreen,
	"DISABLED":  colorRed,
}

// ---------------------------------------------------------------------------
// Rules — which field gets which palette. Add entries here to color a field.
// ---------------------------------------------------------------------------

// enumColorRules maps an enum field NAME to its palette. Only listed enum fields are colored.
var enumColorRules = map[string]colorByValue{
	"SEVERITY": severityColors,
	"COMPCODE": compCodeColors,
}

// stringColorRules maps an (ITEM TYPE, FIELD NAME) pair to a palette, overriding the default
// binaryStateColors for that specific field. List a field here only when it needs a palette other
// than the default. Example (uncomment and adjust to your item type and field):
//
//	{item: "ACCESSLOG", field: "SEVERITY"}: severityColors,
var stringColorRules = map[itemField]colorByValue{}

// ---------------------------------------------------------------------------
// Builders — used by dataframe.go. Return nil when a field has no rule.
// ---------------------------------------------------------------------------

// enumColors returns the ordinal-indexed color table for an enum field, or nil when the field has
// no rule (Grafana then auto-colors it). text is the field's ordinal->name table; the returned
// slice is aligned to it. UNKNOWN is colored gray rather than left to fall through to a palette
// color meant for a real error state — an unresolved/not-yet-known value isn't a warning.
func enumColors(fieldName string, text []string) []string {
	palette, ok := enumColorRules[strings.ToUpper(strings.TrimSpace(fieldName))]
	if !ok {
		return nil
	}
	colors := make([]string, len(text))
	matched := false
	for i, name := range text {
		if strings.EqualFold(strings.TrimSpace(name), "UNKNOWN") {
			colors[i] = colorGray
			matched = true
			continue
		}
		colors[i] = palette.color(name)
		if colors[i] != "" {
			matched = true
		}
	}
	if !matched {
		return nil
	}
	return colors
}

// stringValueMappings builds value mappings coloring the values of a string/labelset column. It
// uses the (item type, field) rule when one exists, otherwise the default binaryStateColors. It
// scans the column's distinct values and colors only the ones the palette knows, setting color
// only so the original text is still shown. Returns nil when nothing matched.
func stringValueMappings(model DataModel, header string) data.ValueMappings {
	fieldName := model.Names[header]
	if fieldName == "" {
		fieldName = header
	}
	palette, ok := stringColorRules[itemField{
		item:  strings.ToUpper(strings.TrimSpace(model.ItemType)),
		field: strings.ToUpper(strings.TrimSpace(fieldName)),
	}]
	if !ok {
		// No specific (item, field) rule: fall back to the default two-state palette. Its values
		// (Yes/No, Enabled/Disabled, …) are specific enough that unrelated columns simply match
		// nothing and stay uncolored.
		palette = binaryStateColors
	}

	dataType := model.DataTypes[header]
	mapper := data.ValueMapper{}
	for _, row := range model.Rows {
		s, ok := ConvertToGrafanaValue(row[header], dataType).(string)
		if !ok || s == "" {
			continue
		}
		if _, done := mapper[s]; done {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(s), "UNKNOWN") {
			mapper[s] = data.ValueMappingResult{Color: colorGray}
			continue
		}
		if color := palette.color(s); color != "" {
			mapper[s] = data.ValueMappingResult{Color: color}
		}
	}
	if len(mapper) == 0 {
		return nil
	}
	return data.ValueMappings{mapper}
}
