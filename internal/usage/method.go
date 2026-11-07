package usage

// Method names how a record was obtained, so that records written by
// different methods can be told apart after the method is changed.
//
// The values are written to the "m" field of every record. They must appear
// as string literals only in the const block below; everywhere else, refer to
// them by name. TestMethodLiteralsOnlyInDefinition enforces this.
type Method string

const (
	// MethodStreamJSONResident sends /usage to one long-running claude
	// process that reads and writes stream-json.
	MethodStreamJSONResident Method = "stream-json-resident"
	// MethodOneshot starts "claude -p /usage" for every reading.
	MethodOneshot Method = "oneshot"
	// MethodOAuthAPI asks Anthropic's servers for the usage directly,
	// without claude. It is reserved and not implemented.
	MethodOAuthAPI Method = "oauth-api"
)

// Methods returns every method. The literal check reads its values from here
// rather than repeating them.
func Methods() []Method {
	return []Method{MethodStreamJSONResident, MethodOneshot, MethodOAuthAPI}
}
