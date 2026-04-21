package labels

// Note: loosely based on the Kubernetes labeling rules:
// https://github.com/kubernetes/kubernetes/blob/0cd75e8fec/staging/src/k8s.io/apimachinery/pkg/util/validation/validation.go

import (
	"regexp"

	"github.com/pkg/errors"
)

const (
	dns1123LabelFmt    string = "[a-z0-9]([-a-z0-9]*[a-z0-9])?"
	dns1123LabelErrMsg string = "a DNS-1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character"
)

// DNS1123LabelMaxLength is a label's max length in DNS (RFC 1123)
const DNS1123LabelMaxLength int = 63

// DNS1123LabelRegexp is a regular expression matching a dns1123 label.
var DNS1123LabelRegexp = regexp.MustCompile("^" + dns1123LabelFmt + "$")

// ValidateDNSLabel checks if the id is a valid DNS label.
func ValidateDNSLabel(id string) error {
	if len(id) > DNS1123LabelMaxLength {
		return errors.Errorf(
			"length %d cannot be greater than %d",
			len(id), DNS1123LabelMaxLength,
		)
	}
	if !DNS1123LabelRegexp.MatchString(id) {
		return errors.New(dns1123LabelErrMsg)
	}
	return nil
}

const (
	dns1123SubdomainFmt    string = dns1123LabelFmt + "(\\." + dns1123LabelFmt + ")*"
	dns1123SubdomainErrMsg string = "a DNS-1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character"
)

// DNS1123SubdomainMaxLength is a subdomain's max length in DNS (RFC 1123)
const DNS1123SubdomainMaxLength int = 253

// DNS1123SubdomainRegexp is a regular expression matching a RFC 1123 subdomain.
var DNS1123SubdomainRegexp = regexp.MustCompile("^" + dns1123SubdomainFmt + "$")

// ValidateDNSSubdomain checks if ID is a valid DNS subdomain.
func ValidateDNSSubdomain(value string) error {
	if len(value) > DNS1123SubdomainMaxLength {
		return errors.Errorf(
			"length %d cannot be greater than %d",
			len(value), DNS1123SubdomainMaxLength,
		)
	}
	if !DNS1123SubdomainRegexp.MatchString(value) {
		return errors.New(dns1123SubdomainErrMsg)
	}
	return nil
}
