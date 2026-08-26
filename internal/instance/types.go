// Package instance defines the data shape of an instance-wide discovery
// snapshot: component versions, storage summary, authentication/signing-key
// configuration, users/groups/permission-target access, projects, and
// replication/federation config. It has no jfclient dependency by design -
// internal/discover populates a Snapshot (it needs jfclient to talk to a
// JPD), and internal/analyze only ever reads one back from JSON (it never
// connects anywhere), so this package is the dependency-free seam between
// them.
package instance

// SystemInfo holds component versions and the storage summary from
// /artifactory/api/storageinfo. A component's *Available field is false
// when its version endpoint returned 404 (not licensed/not installed on
// this instance) - that's a normal, expected state, not an error.
type SystemInfo struct {
	ArtifactoryVersion  string   `json:"artifactory_version"`
	ArtifactoryRevision string   `json:"artifactory_revision"`
	Addons              []string `json:"addons,omitempty"`

	XrayAvailable bool   `json:"xray_available"`
	XrayVersion   string `json:"xray_version,omitempty"`

	AccessAvailable bool   `json:"access_available"`
	AccessVersion   string `json:"access_version,omitempty"`

	DistributionAvailable bool   `json:"distribution_available"`
	DistributionVersion   string `json:"distribution_version,omitempty"`

	// Storage summary, straight from storageinfo's binariesSummary /
	// fileStoreSummary (already human-formatted strings upstream).
	BinariesCount  string `json:"binaries_count,omitempty"`
	BinariesSize   string `json:"binaries_size,omitempty"`
	ArtifactsCount string `json:"artifacts_count,omitempty"`
	ArtifactsSize  string `json:"artifacts_size,omitempty"`
	Optimization   string `json:"optimization,omitempty"`
	StorageType    string `json:"storage_type,omitempty"`
}

// OIDCProvider is one configured OpenID Connect identity provider
// (GET /access/api/v1/oidc).
type OIDCProvider struct {
	Name         string `json:"name"`
	IssuerURL    string `json:"issuer_url"`
	Audience     string `json:"audience"`
	ProviderType string `json:"provider_type"`
}

// SigningKey is one configured repository signing key pair
// (GET /artifactory/api/security/keypair). Only metadata is captured -
// never key material, public or private.
type SigningKey struct {
	PairName string `json:"pair_name"`
	PairType string `json:"pair_type"`
	Alias    string `json:"alias"`
}

// SecurityInfo summarizes how the instance authenticates and whether
// artifact/metadata signing keys are configured - both worth surfacing
// prominently in an audit.
type SecurityInfo struct {
	SAMLConfigured bool           `json:"saml_configured"`
	LDAPConfigured bool           `json:"ldap_configured"`
	OIDCProviders  []OIDCProvider `json:"oidc_providers,omitempty"`
	SigningKeys    []SigningKey   `json:"signing_keys,omitempty"`
}

// User is one entry of GET /access/api/v2/users.
type User struct {
	Username string `json:"username"`
	Realm    string `json:"realm"`
	Status   string `json:"status,omitempty"`
}

// Group is one entry of GET /access/api/v2/groups.
type Group struct {
	Name string `json:"name"`
}

// PermissionTarget is a permission target's repo scope plus the
// user/group -> actions grants on it (GET
// /access/api/v2/permissions/{name}, resources.artifact only - build/
// release-bundle scopes aren't captured).
type PermissionTarget struct {
	Name         string              `json:"name"`
	Repos        []string            `json:"repos,omitempty"`
	UserActions  map[string][]string `json:"user_actions,omitempty"`
	GroupActions map[string][]string `json:"group_actions,omitempty"`
}

// Project is one entry of GET /access/api/v1/projects, with RepoCount
// filled in by cross-referencing repos' project_key.
type Project struct {
	Key               string `json:"key"`
	DisplayName       string `json:"display_name"`
	Description       string `json:"description,omitempty"`
	StorageQuotaBytes int64  `json:"storage_quota_bytes"`
	RepoCount         int    `json:"repo_count"`
}

// ReplicationConfig is one entry of GET /artifactory/api/replications.
type ReplicationConfig struct {
	RepoKey         string `json:"repo_key"`
	ReplicationType string `json:"replication_type"`
	Enabled         bool   `json:"enabled"`
	URL             string `json:"url,omitempty"`
	CronExp         string `json:"cron_exp,omitempty"`
	SyncDeletes     bool   `json:"sync_deletes"`
}

// FederationMember is one member of a federated repo's member list.
type FederationMember struct {
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

// FederatedRepo is a federated repo's member config, fetched from its
// individual repo configuration (there is no bulk federation-config
// endpoint).
type FederatedRepo struct {
	RepoKey string             `json:"repo_key"`
	Members []FederationMember `json:"members"`
}

// Snapshot is the full instance-wide discovery result, written/read as one
// JSON file by internal/discover's "discover instance" and consumed by
// "discover analyze --instance".
type Snapshot struct {
	GeneratedAt string       `json:"generated_at"`
	System      SystemInfo   `json:"system"`
	Security    SecurityInfo `json:"security"`

	Users          []User `json:"users,omitempty"`
	UsersTruncated bool   `json:"users_truncated,omitempty"`

	Groups          []Group `json:"groups,omitempty"`
	GroupsTruncated bool    `json:"groups_truncated,omitempty"`

	Permissions          []PermissionTarget `json:"permissions,omitempty"`
	PermissionsTruncated bool               `json:"permissions_truncated,omitempty"`

	Projects []Project `json:"projects,omitempty"`

	Replications   []ReplicationConfig `json:"replications,omitempty"`
	FederatedRepos []FederatedRepo     `json:"federated_repos,omitempty"`

	// Warnings collects non-fatal problems (a single permission-detail
	// fetch failing, a soft-skipped optional service, etc.) so the whole
	// collection doesn't abort over one bad call.
	Warnings []string `json:"warnings,omitempty"`
}
