//go:build js

package fetch

// cache enums
const (
	CacheDefault      = "default"
	CacheNoStore      = "no-store"
	CacheReload       = "reload"
	CacheNone         = "no-cache"
	CacheForce        = "force-cache"
	CacheOnlyIfCached = "only-if-cached"
)

// credentials enums
const (
	CredentialsOmit       = "omit"
	CredentialsSameOrigin = "same-origin"
	CredentialsInclude    = "include"
)

// Common HTTP methods.
//
// Unless otherwise noted, these are defined in RFC 7231 section 4.3.
const (
	MethodGet     = "GET"
	MethodHead    = "HEAD"
	MethodPost    = "POST"
	MethodPut     = "PUT"
	MethodPatch   = "PATCH" // RFC 5789
	MethodDelete  = "DELETE"
	MethodConnect = "CONNECT"
	MethodOptions = "OPTIONS"
	MethodTrace   = "TRACE"
)

// Mode enums
const (
	ModeSameOrigin = "same-origin"
	ModeNoCORS     = "no-cors"
	ModeCORS       = "cors"
	ModeNavigate   = "navigate"
)

// Redirect enums
const (
	RedirectFollow = "follow"
	RedirectError  = "error"
	RedirectManual = "manual"
)

// Referrer enums
const (
	ReferrerNone   = "no-referrer"
	ReferrerClient = "client"
)

// ReferrerPolicy enums
const (
	ReferrerPolicyNone        = "no-referrer"
	ReferrerPolicyNoDowngrade = "no-referrer-when-downgrade"
	ReferrerPolicyOrigin      = "origin"
	ReferrerPolicyCrossOrigin = "origin-when-cross-origin"
	ReferrerPolicyUnsafeURL   = "unsafe-url"
)
