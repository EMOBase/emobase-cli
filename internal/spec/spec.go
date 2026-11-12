// Package spec describes the environment contract between the EMOBase
// deployment repositories: which keys each role must define, which keys are
// shared, and which stack is authoritative for each shared value.
package spec

import "strings"

// Authority names the stack whose value wins when the same key appears in
// several .env files.
type Authority string

const (
	AuthorityInfra  Authority = "emobase-infra"
	AuthorityPortal Authority = "emobase-portal-deploy"
	AuthoritySelf   Authority = "self"
)

// SharedKey is a key that must be identical across stacks on one server.
type SharedKey struct {
	Key       string
	Authority Authority
	Secret    bool
	// RealmScoped keys only apply to a species that reuses the shared realm.
	RealmScoped bool
}

// SharedToInfra are copied from emobase-infra into the portal and species.
var SharedToInfra = []SharedKey{
	{Key: "MYSQL_ROOT_PASSWORD", Authority: AuthorityInfra, Secret: true},
	{Key: "KEYCLOAK_PUBLIC_URL", Authority: AuthorityInfra},
	{Key: "KEYCLOAK_INTERNAL_URL", Authority: AuthorityInfra},
	{Key: "EMOBASE_INFRA_NETWORK", Authority: AuthorityInfra},
}

// RealmKeys are shared between infra and every stack that uses the shared
// realm. A dedicated species realm owns its own values.
var RealmKeys = []SharedKey{
	{Key: "KEYCLOAK_REALM", Authority: AuthorityInfra},
	{Key: "KEYCLOAK_CLIENT_ID", Authority: AuthorityInfra},
	{Key: "KEYCLOAK_CLIENT_SECRET", Authority: AuthorityInfra, Secret: true},
}

// RequiredInfra is the minimum set for a working shared infrastructure stack.
// Shared keys are checked separately, so they are excluded here.
var RequiredInfra = []string{
	"KEYCLOAK_HOST_PORT",
	"KEYCLOAK_ADMIN",
	"KEYCLOAK_ADMIN_PASSWORD",
	"COCO_USERNAME",
	"COCO_PASSWORD",
}

// RequiredPortal is the minimum set for the portal stack.
var RequiredPortal = []string{
	"PUBLIC_SERVER",
	"HTTP_PORT",
	"PUBLIC_DATA_DIR",
	"EMOBASE_GENOMICS_MYSQL__HOST",
	"EMOBASE_GENOMICS_MYSQL__PORT",
	"EMOBASE_GENOMICS_MYSQL__USER",
	"EMOBASE_GENOMICS_MYSQL__DATABASE",
	"AUTH_SECRET",
	"EMOBASE_GENOMICS_IMAGE",
	"EMOBASE_PORTAL_WEB_IMAGE",
}

// RequiredSpecies is the minimum set for a species stack. It mirrors what
// emobase-deploy/init.sh writes and compose.yml consumes.
var RequiredSpecies = []string{
	"MAIN_SPECIES",
	"PUBLIC_SERVER",
	"HTTP_PORT",
	"PORTAL_INTERNAL_URL",
	"DIRECTUS_DB_DATABASE",
	"DIRECTUS_SECRET",
	"DIRECTUS_ADMIN_EMAIL",
	"DIRECTUS_ADMIN_PASSWORD",
	"COCO_USERNAME",
	"COCO_PASSWORD",
	"EMOBASE_WEB_IMAGE",
	"SSR_OIDC_SESSION_SECRET",
	"INTERNAL_API_GENOMICS_SERVICE",
	"INTERNAL_KEYCLOAK_ISSUER",
	"INTERNAL_DIRECTUS_URL",
	"PUBLIC_DATA_DIR",
	"PRIVATE_DATA_DIR",
	"IMAGE_DIR",
	"UID",
	"GID",
}

// URLKeys have no trailing slash by convention; Keycloak in particular treats
// a trailing slash as a different issuer.
var URLKeys = map[string]bool{
	"PUBLIC_SERVER":                true,
	"PORTAL_SERVER":                true,
	"PORTAL_INTERNAL_URL":          true,
	"KEYCLOAK_PUBLIC_URL":          true,
	"KEYCLOAK_INTERNAL_URL":        true,
	"INTERNAL_KEYCLOAK_ISSUER":     true,
	"DIRECTUS_KEYCLOAK_ISSUER_URL": true,
}

// IsSecret reports whether a value must be masked in output.
func IsSecret(key string) bool {
	upper := strings.ToUpper(key)
	for _, marker := range []string{"PASSWORD", "SECRET", "TOKEN"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	// Match KEY as a whole underscore-separated word only, so URLs such as
	// DIRECTUS_KEYCLOAK_ISSUER_URL are not mistaken for secrets.
	for _, part := range strings.Split(upper, "_") {
		if part == "KEY" {
			return true
		}
	}
	return false
}

// IsPlaceholder reports values left by .env.example or an empty value.
func IsPlaceholder(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return true
	}
	low := strings.ToLower(v)
	return strings.Contains(low, "change-me") || strings.Contains(low, "changeme")
}

// NormalizeURL removes a trailing slash for comparison.
func NormalizeURL(v string) string {
	return strings.TrimRight(strings.TrimSpace(v), "/")
}

// Mask hides secret values, keeping only a length hint.
func Mask(v string) string {
	if v == "" {
		return "(empty)"
	}
	return "********"
}

// Lookup returns the shared-key definition for key, if any.
func Lookup(key string) (SharedKey, bool) {
	for _, k := range SharedToInfra {
		if k.Key == key {
			return k, true
		}
	}
	for _, k := range RealmKeys {
		if k.Key == key {
			k.RealmScoped = true
			return k, true
		}
	}
	return SharedKey{}, false
}

// ComposeProjectOf returns the Compose project name for a stack. The infra and
// portal compose files declare `name:`; species define COMPOSE_PROJECT_NAME.
func ComposeProjectOf(projectField, envValue, dirBase string) string {
	if projectField != "" {
		return projectField
	}
	if envValue != "" {
		return envValue
	}
	return dirBase
}
