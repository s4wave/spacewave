package provider_spacewave

// ProviderIRI returns the GC IRI for a spacewave provider: "sw-provider:{id}".
func ProviderIRI(providerID string) string {
	return "sw-provider:" + providerID
}
