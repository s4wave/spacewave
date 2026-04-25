package provider_local

// ProviderIRI returns the GC IRI for a provider: "provider:{id}".
func ProviderIRI(providerID string) string {
	return "provider:" + providerID
}
