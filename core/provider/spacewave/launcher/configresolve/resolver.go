package configresolve

import (
	"net/url"

	"github.com/pkg/errors"
)

// ResolveEndpoints returns configured endpoints when present, otherwise defaults.
func ResolveEndpoints(disable bool, configURLs []string, defaultURLs []string) ([]string, error) {
	configURLs, err := dedupURLs(configURLs, "endpoint")
	if err != nil {
		return nil, err
	}
	if disable {
		if len(configURLs) != 0 {
			return nil, errors.New("disable_endpoint_fetch: cannot be set with endpoints")
		}
		return nil, nil
	}
	if len(configURLs) != 0 {
		return configURLs, nil
	}
	return dedupURLs(defaultURLs, "build-time endpoint")
}

func dedupURLs(urls []string, label string) ([]string, error) {
	deduped := make([]string, 0, len(urls))
	seen := make(map[string]struct{}, len(urls))
	for _, u := range urls {
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		parsed, err := url.Parse(u)
		if err != nil {
			return nil, errors.Wrapf(err, "%s %q", label, u)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, errors.Errorf("%s %q: absolute URL required", label, u)
		}
		seen[u] = struct{}{}
		deduped = append(deduped, u)
	}
	return deduped, nil
}

// ResolveDistPeerIDs returns configured signer peer ID strings when present.
func ResolveDistPeerIDs(configIDs []string, defaultIDs []string) []string {
	configIDs = dedupStrings(configIDs)
	if len(configIDs) != 0 {
		return configIDs
	}
	return dedupStrings(defaultIDs)
}

func dedupStrings(values []string) []string {
	deduped := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		deduped = append(deduped, value)
	}
	return deduped
}
