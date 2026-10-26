package resource_proxy

// Rules gives each method the proxy carries its Rule, keyed by service ID and
// then by method ID.
type Rules map[string]map[string]Rule

// Rule returns the rule of one method, or End for a method r does not name.
func (r Rules) Rule(service, method string) Rule {
	return r[service][method]
}
