package plugin

// This file holds the dataservice wire contract: the jk_* query-parameter and response-field names.
// The jk_* string values are the on-the-wire protocol and must not be renamed. They are shared by
// the query client (client.go) and the separate autocomplete client (completion.go), so they live
// here rather than in either client.

// RequestParameter enumerates the dataservice HTTP query parameters.
type RequestParameter = string

// HDR_API_KEY is the dataservice access-token header. The service accepts the token either here
// or as the jk_token query parameter, and the header takes precedence. The plugin sends it ONLY
// in this header: a token in the URL would end up in proxy/access logs and in *url.Error messages
// (which surface in panel errors and the health-check message).
const HDR_API_KEY = "X-API-Key"

const (
	// REQ_TOKEN is part of the wire contract but deliberately not sent — see HDR_API_KEY.
	REQ_TOKEN    RequestParameter = "jk_token"
	REQ_QUERY    RequestParameter = "jk_query"
	REQ_TIMEZONE RequestParameter = "jk_tz"
	REQ_LOCALE   RequestParameter = "jk_locale"
	REQ_REPO     RequestParameter = "jk_repo"
	REQ_DATE     RequestParameter = "jk_date"
	REQ_MAXROWS  RequestParameter = "jk_maxrows"
	REQ_TRACE    RequestParameter = "jk_trace"
)

// ResponseField enumerates the dataservice error-envelope fields ({ "jk_ccode": "ERROR",
// "jk_error": "..." }).
type ResponseField = string

const (
	RESP_CCODE ResponseField = "jk_ccode" // completion code (CompCodeType)
	RESP_ERROR ResponseField = "jk_error" // error message
)
