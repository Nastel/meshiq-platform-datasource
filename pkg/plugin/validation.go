package plugin

import (
	"errors"
	"strings"
)

// errNotAuthorized is returned for any rejected jKQL statement.
var errNotAuthorized = errors.New("Specified token is not authorized to execute this statement")

// blockedItemTypeLabels are the jKQL item-type labels (canonical name + known aliases) that user
// queries must not target: LOG, every admin item type, and every reference/catalog item type —
// across both supported dataservice versions. The metadata-statement forms in
// metadataStatementLeadWords (Items, Fields, Enumeration, ...) bypass this list since they only
// ever return schema info, not business data.
var blockedItemTypeLabels = map[string]bool{
	"log": true, "logs": true,

	// Admin item types.
	"user": true, "users": true, "usr": true, "usrs": true,
	"organization": true, "organizations": true, "org": true, "orgs": true,
	"team": true, "teams": true,
	"repository": true, "repositories": true, "repo": true, "repos": true,
	"accesstoken": true, "accesstokens": true, "token": true, "tokens": true,
	"quotausage": true, "quotausages": true, "usage": true, "usages": true,
	"volume": true, "volumes": true, "vol": true, "vols": true,

	// Reference/catalog item types.
	"dynamicitem": true, "dynamicitems": true,
	"enumeration": true, "enumerations": true, "enum": true, "enums": true,
	"extdatasourcetype": true, "extdatasrct": true, "extdatasourcetypes": true, "extdatasrctype": true,
	"extdatasrctypes": true, "externaldatasourcetypes": true, "externaldatasrctype": true,
	"externaldatasrctypes": true, "extdatasource": true, "extdatasources": true, "extdatasrc": true,
	"extdatasrcs": true, "externaldatasource": true, "externaldatasources": true, "externaldatasrc": true,
	"extfield": true, "extfields": true, "externalfields": true,
	"extfunction": true, "extfunctions": true, "extfunc": true, "extfuncs": true,
	"extitem": true, "extitems": true, "externalitems": true,
	"extitemfield": true, "extitemfields": true, "externalitemfields": true,
	"extprovidertype": true, "extprovider": true,
	"feature": true, "features": true, "featureflag": true, "featureflags": true,
	"field": true, "fieldtype": true, "fields": true, "fieldtypes": true,
	"function": true, "functions": true, "func": true, "funcs": true,
	"item": true, "itemtype": true, "items": true, "itemtypes": true,
	"keyword": true, "keywords": true,
	"license": true, "licenses": true, "lic": true,
	"parameter": true, "parameters": true,
	"providertype": true, "providertypes": true, "providers": true, "provider": true,
	"actionprovider": true, "actionproviders": true,
	"statement": true, "statements": true, "stmt": true, "stmts": true,
	"iplocation": true, "iplocations": true, "iploc": true, "iplocs": true,
	"iprange": true, "ipranges": true,
}

// metadataStatementLeadWords identify a metadata statement (Get Items/Fields/Enumeration/
// Parameter/Key...) rather than a business-data query — these always return schema info, so
// they're allowed even when they mention an otherwise-blocked item type, e.g. "Get Fields For
// User". Other metadata forms (Functions, Keywords, Statements, Providertypes) stay blocked.
var metadataStatementLeadWords = map[string]bool{
	"items": true, "item": true, "itemtype": true, "itemtypes": true,
	"fields": true, "field": true, "fieldtype": true, "fieldtypes": true,
	"enumeration": true, "enumerations": true,
	"parameter": true, "parameters": true,
	"property": true, "properties": true,
	"key": true,
}

// metadataStatementFillerWords are words that precede a metadataStatementLeadWords token
// ("Get Custom Fields For X") without being safe to skip in a plain business-data GET.
var metadataStatementFillerWords = map[string]bool{
	"custom": true,
}

// leadingFillerWords are the words jKQL allows between GET and the item type (limit words like
// "top"/"last", "number of"/"percent of", "distinct", "the").
var leadingFillerWords = map[string]bool{
	"the":   true,
	"count": true, "number": true, "numbers": true, "percent": true, "percentage": true,
	"percentages": true, "of": true, "and": true,
	"last": true, "first": true, "top": true, "bottom": true, "latest": true, "earliest": true,
	"best": true, "worst": true, "largest": true, "smallest": true, "longest": true, "shortest": true,
	"level":    true,
	"distinct": true,
}

// validateUserQuery rejects a user-submitted jKQL statement before it reaches the dataservice:
// defense in depth in case an access token wasn't properly scoped down server-side. Only GET
// statements are allowed, and only against item types not in blockedItemTypeLabels. Unrecognized
// input is rejected (fail closed), not allowed through.
func validateUserQuery(jkql string) error {
	tokens := strings.Fields(jkql)
	if len(tokens) == 0 {
		return nil
	}

	tokens = dropLeading(tokens, "the")
	if len(tokens) == 0 {
		return nil
	}

	if !strings.EqualFold(tokens[0], "get") {
		return errNotAuthorized
	}
	tokens = tokens[1:]

	if len(tokens) > maxLeadingTokens {
		tokens = tokens[:maxLeadingTokens]
	}
	for _, tok := range tokens {
		normalized := strings.ToLower(strings.ReplaceAll(tok, "_", ""))
		if leadingFillerWords[normalized] || metadataStatementFillerWords[normalized] || isUnsignedInteger(tok) {
			continue
		}
		if metadataStatementLeadWords[normalized] {
			return nil
		}
		if blockedItemTypeLabels[normalized] {
			return errNotAuthorized
		}
		return nil
	}

	return errNotAuthorized
}

// dropLeading removes leading occurrences of word (case-insensitive) from tokens.
func dropLeading(tokens []string, word string) []string {
	i := 0
	for i < len(tokens) && strings.EqualFold(tokens[i], word) {
		i++
	}
	return tokens[i:]
}

// maxLeadingTokens bounds the scan for an item type after GET, so unusual input can't scan
// unboundedly.
const maxLeadingTokens = 16

func isUnsignedInteger(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
