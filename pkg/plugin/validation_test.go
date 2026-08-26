package plugin

import "testing"

func TestValidateUserQuery_Allowed(t *testing.T) {
	tests := []string{
		"",
		"GET KafkaCluster Fields Name",
		"get number of KafkaTopic group by Name",
		"GET Number Of KafkaBroker Where WgsClusterName In ($cluster)",
		"get percent of KafkaTopic",
		"get number of and percentage of KafkaTopic",
		"GET top 10 IbmmqLocalQueue Fields Name, curQDepth",
		"get last 100 IbmmqLocalQueue",
		"GET DISTINCT KafkaCluster",
		"  get   KafkaCluster  ",
		"the GET KafkaCluster",
		"GET the top 10 KafkaCluster",
		"get COLLECTD.CPU field MetricTime",
	}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			if err := validateUserQuery(q); err != nil {
				t.Errorf("validateUserQuery(%q) = %v, want nil", q, err)
			}
		})
	}
}

// Metadata statements only ever return schema info, so they're allowed even when they mention
// an otherwise-blocked item type.
func TestValidateUserQuery_AllowedMetadataStatements(t *testing.T) {
	tests := []string{
		"GET ITEMS",
		"get item",
		"GET ITEMTYPES",
		"Get Fields For User",
		"GET FIELDS FOR ORGANIZATION",
		"get fieldtype",
		"GET FIELDS",
		"Get Custom Fields For KafkaCluster",
		"Get Distinct Custom Property For KafkaCluster",
		"Get Key From Name For KafkaCluster",
		"GET ENUMERATION FOR WgsEmsState",
		"get enumerations",
		"GET PARAMETER",
		"get parameters",
		"GET DISTINCT FIELDS FOR USER",
	}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			if err := validateUserQuery(q); err != nil {
				t.Errorf("validateUserQuery(%q) = %v, want nil", q, err)
			}
		})
	}
}

func TestValidateUserQuery_BlockedStatements(t *testing.T) {
	tests := []string{
		"COMPARE KafkaCluster",
		"UPSERT Log",
		"DELETE Log",
		"CREATE USER",
		"ALTER USER",
		"DROP USER",
		"GRANT USER",
		"REVOKE USER",
		"LOAD USER",
		"INVOKE something",
		"FIND 'text'",
		"USE PARAMETER x",
		"COMPUTE something",
	}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			if err := validateUserQuery(q); err != errNotAuthorized {
				t.Errorf("validateUserQuery(%q) = %v, want errNotAuthorized", q, err)
			}
		})
	}
}

func TestValidateUserQuery_BlockedItemTypes(t *testing.T) {
	tests := []string{
		"Get Log",
		"GET LOG",
		"get logs",
		"get number of Log",
		"get number of and percentage of logs",
		"GET top 10 Log",
		"get the Log",
		"GET USER",
		"get users",
		"GET usr",
		"get usrs",
		"GET ORGANIZATION",
		"get organizations",
		"GET org",
		"get orgs",
		"GET TEAM",
		"get teams",
		"GET REPOSITORY",
		"get repositories",
		"GET repo",
		"get repos",
		"GET ACCESS_TOKEN",
		"get accesstoken",
		"GET accesstokens",
		"get token",
		"GET tokens",
		"get QUOTA_USAGE",
		"GET quotausage",
		"get quotausages",
		"GET usage",
		"get usages",
		"GET VOLUME",
		"get volumes",
		"GET vol",
		"get vols",
		"GET DISTINCT users",
		"get number of users group by org",
	}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			if err := validateUserQuery(q); err != errNotAuthorized {
				t.Errorf("validateUserQuery(%q) = %v, want errNotAuthorized", q, err)
			}
		})
	}
}

// Reference/catalog item types with no metadata-statement form stay blocked.
func TestValidateUserQuery_BlockedReferenceItemTypes(t *testing.T) {
	tests := []string{
		"GET PROVIDER",
		"get providers",
		"get actionprovider",
		"GET PROVIDERTYPE",
		"get providertypes",
		"GET FUNCTION",
		"get functions",
		"GET func",
		"get funcs",
		"GET KEYWORD",
		"get keywords",
		"GET STATEMENT",
		"get statements",
		"GET stmt",
		"get stmts",
		"GET FEATURE",
		"get featureflags",
		"GET LICENSE",
		"get lic",
		"GET DYNAMIC_ITEM",
		"get dynamicitems",
		"GET EXT_ITEM",
		"get extitems",
		"GET EXT_FIELD",
		"get extfields",
		"GET EXT_ITEM_FIELD",
		"get extitemfields",
		"GET EXT_DATA_SRC_T",
		"get extdatasourcetype",
		"get externaldatasrc",
		"GET EXT_FUNCTION",
		"get extfuncs",
		"GET EXT_PROVIDER",
		"get extprovider",
		"GET IPLOCATION",
		"get iprange",
		"get iplocs",
	}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			if err := validateUserQuery(q); err != errNotAuthorized {
				t.Errorf("validateUserQuery(%q) = %v, want errNotAuthorized", q, err)
			}
		})
	}
}

func TestValidateUserQuery_CaseInsensitiveAndWhitespace(t *testing.T) {
	tests := []string{"get user", "Get User", "GET USER", "GeT UsEr", "get\tuser", "get  user"}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			if err := validateUserQuery(q); err != errNotAuthorized {
				t.Errorf("validateUserQuery(%q) = %v, want errNotAuthorized", q, err)
			}
		})
	}
}

// A leading "the" must not let a query dodge the GET check.
func TestValidateUserQuery_LeadingThe(t *testing.T) {
	if err := validateUserQuery("the GET KafkaCluster"); err != nil {
		t.Errorf("validateUserQuery(%q) = %v, want nil", "the GET KafkaCluster", err)
	}
	if err := validateUserQuery("the DELETE Log"); err != errNotAuthorized {
		t.Errorf("validateUserQuery(%q) = %v, want errNotAuthorized", "the DELETE Log", err)
	}
}

// A GET with no resolvable item type must fail closed, not default to allowed.
func TestValidateUserQuery_NoResolvableItemTypeFailsClosed(t *testing.T) {
	tests := []string{
		"GET",
		"get top",
		"GET number of",
	}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			if err := validateUserQuery(q); err != errNotAuthorized {
				t.Errorf("validateUserQuery(%q) = %v, want errNotAuthorized", q, err)
			}
		})
	}
}
