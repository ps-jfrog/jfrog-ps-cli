// Package instance defines the data shape of an instance-wide discovery
// snapshot: component versions, storage summary, authentication/signing-key
// configuration, users/groups/permission-target access, projects, and
// replication/federation config. It has no jfclient dependency by design -
// internal/discover populates a Snapshot (it needs jfclient to talk to a
// JPD), and internal/analyze only ever reads one back from JSON (it never
// connects anywhere), so this package is the dependency-free seam between
// them.
package instance

import (
	"net"
	"net/url"
	"strings"
)

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
	SAMLConfigured    bool           `json:"saml_configured"`
	LDAPConfigured    bool           `json:"ldap_configured"`
	HTTPSSOConfigured bool           `json:"http_sso_configured"`
	CrowdConfigured   bool           `json:"crowd_configured"`
	OIDCProviders     []OIDCProvider `json:"oidc_providers,omitempty"`
	SigningKeys       []SigningKey   `json:"signing_keys,omitempty"`
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
// filled in by cross-referencing repos' project_key, and Roles/Users/
// Groups/Repos/Environments filled in from the project's own
// roles/members/repos/environments sub-resources (four extra calls per
// project - see CollectOptions.ProjectDetail).
type Project struct {
	Key               string          `json:"key"`
	DisplayName       string          `json:"display_name"`
	Description       string          `json:"description,omitempty"`
	StorageQuotaBytes int64           `json:"storage_quota_bytes"`
	RepoCount         int             `json:"repo_count"`
	Roles             []ProjectRole   `json:"roles,omitempty"`
	Users             []ProjectMember `json:"users,omitempty"`
	Groups            []ProjectMember `json:"groups,omitempty"`
	Repos             []string        `json:"repos,omitempty"`
	Environments      []string        `json:"environments,omitempty"`
}

// ProjectRole is a custom or predefined role scoped to one project
// (GET /access/api/v1/projects/{key}/roles).
type ProjectRole struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"` // PREDEFINED or CUSTOM
	Environments []string `json:"environments,omitempty"`
	Actions      []string `json:"actions,omitempty"`
}

// ProjectMember is a user or group assigned to a project, with their
// project-scoped roles (GET /access/api/v1/projects/{key}/members/users
// or .../members/groups).
type ProjectMember struct {
	Name  string   `json:"name"`
	Roles []string `json:"roles,omitempty"`
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

// GlobalRole is a custom global role (GET /access/api/v1/roles).
type GlobalRole struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Type         string   `json:"type,omitempty"` // CUSTOM_GLOBAL
	Environments []string `json:"environments,omitempty"`
	Actions      []string `json:"actions,omitempty"`
}

// GlobalEnvironment is a global environment definition
// (GET /access/api/v1/environments).
type GlobalEnvironment struct {
	Name string `json:"name"`
}

// ScopedToken is an Access scoped token's metadata (GET
// /access/api/v1/tokens) - never the token secret itself, which the API
// doesn't return here anyway.
type ScopedToken struct {
	TokenID     string `json:"token_id"`
	Subject     string `json:"subject"`
	Description string `json:"description,omitempty"`
	Scope       string `json:"scope,omitempty"`
	IssuedAt    int64  `json:"issued_at,omitempty"`
	Expiry      int64  `json:"expiry,omitempty"`
	Refreshable bool   `json:"refreshable"`
}

// Certificate is an Artifactory SSL certificate's metadata (GET
// /artifactory/api/system/security/certificates). The API never returns
// the PEM content itself.
// JSON tags below match the Artifactory REST API's own field casing
// (camelCase) rather than this package's usual snake_case, deliberately -
// these are thin pass-through config dumps, and matching the wire format
// makes them easy to cross-reference against JFrog's REST API docs.
type Certificate struct {
	Alias      string `json:"certificateAlias"`
	IssuedTo   string `json:"issuedTo"`
	IssuedBy   string `json:"issuedBy"`
	ValidUntil string `json:"validUntil"`
}

// PropertySet is an Artifactory property set (GET /artifactory/api/propertysets,
// which returns only the name and a property count, not the properties
// themselves).
type PropertySet struct {
	Name            string `json:"name"`
	PropertiesCount int    `json:"propertiesCount"`
}

// Proxy is an Artifactory HTTP proxy config (GET /artifactory/api/proxies).
// Never captures a proxy password (the API doesn't return one here).
type Proxy struct {
	Key             string `json:"key"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Username        string `json:"username,omitempty"`
	DefaultProxy    bool   `json:"defaultProxy"`
	PlatformDefault bool   `json:"platformDefault"`
}

// Backup is an Artifactory backup schedule (GET /artifactory/api/backup).
type Backup struct {
	Key                  string   `json:"key"`
	Enabled              bool     `json:"enabled"`
	CronExp              string   `json:"cronExp"`
	RetentionPeriodHours int      `json:"retentionPeriodHours"`
	ExcludedRepositories []string `json:"excludedRepositories,omitempty"`
}

// MailServerConfig is the Artifactory mail server config (GET
// /artifactory/api/system/mailserver). Never captures a password (the API
// doesn't return one here).
type MailServerConfig struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host,omitempty"`
	Port    int    `json:"port,omitempty"`
	From    string `json:"from,omitempty"`
	UseSSL  bool   `json:"ssl"`
	UseTLS  bool   `json:"tls"`
}

// TrashConfig is Artifactory's trash-can retention config (GET
// /artifactory/api/trash/config).
type TrashConfig struct {
	Enabled             bool `json:"enabled"`
	RetentionPeriodDays int  `json:"retentionPeriodDays"`
	AllowPermDeletes    bool `json:"allowPermDeletes"`
}

// PasswordExpirationPolicy is the platform password-aging policy (GET
// /artifactory/api/security/configuration/passwordExpirationPolicy).
type PasswordExpirationPolicy struct {
	Enabled        bool `json:"enabled"`
	PasswordMaxAge int  `json:"passwordMaxAge"`
	NotifyByEmail  bool `json:"notifyByEmail"`
}

// UserLockPolicy is the platform failed-login lockout policy (GET
// /artifactory/api/security/userLockPolicy).
type UserLockPolicy struct {
	Enabled       bool `json:"enabled"`
	LoginAttempts int  `json:"loginAttempts"`
}

// RepoLayout is a repository layout definition (GET
// /artifactory/api/repolayouts).
type RepoLayout struct {
	Name                string `json:"name"`
	ArtifactPathPattern string `json:"artifactPathPattern,omitempty"`
}

// ReverseProxyConfig is Artifactory's reverse-proxy config (GET
// /artifactory/api/system/reverse-proxy).
type ReverseProxyConfig struct {
	ServerProvider   string `json:"serverProvider,omitempty"`
	PublicServerName string `json:"publicServerName,omitempty"`
	UseHTTPS         bool   `json:"useHttps"`
	UseHTTP          bool   `json:"useHttp"`
	HTTPSPort        int    `json:"httpsPort,omitempty"`
	HTTPPort         int    `json:"httpPort,omitempty"`
}

// ArchiveCleanupPolicy is an Artifactory archive cleanup policy (GET
// /artifactory/api/cleanup/archive/policies) - distinct from the package
// cleanup policies captured by `discover repos`/`discover policies`.
type ArchiveCleanupPolicy struct {
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
	CronExp     string `json:"cron_exp"`
	Enabled     bool   `json:"enabled"`
}

// LicenseInfo is the platform license summary (GET /access/api/v1/license).
// Never captures the license hash/fingerprint.
type LicenseInfo struct {
	Type         string `json:"type"`
	LicensedTo   string `json:"licensed_to,omitempty"`
	ValidThrough string `json:"valid_through,omitempty"`
}

// WorkerService is a JFrog Worker service (GET /access/api/v1/workers).
// Never captures the worker's source code.
type WorkerService struct {
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	Action      string `json:"action,omitempty"`
}

// PlatformConfig bundles instance-wide platform/security configuration
// that isn't repo- or user-shaped: roles, tokens, certificates, and a
// grab-bag of singleton config endpoints. Every field is independently
// soft-failed (see Snapshot.Warnings) - one broken/unlicensed endpoint
// never blanks out the rest.
type PlatformConfig struct {
	GlobalRoles            []GlobalRole              `json:"global_roles,omitempty"`
	GlobalEnvironments     []GlobalEnvironment       `json:"global_environments,omitempty"`
	ScopedTokens           []ScopedToken             `json:"scoped_tokens,omitempty"`
	Certificates           []Certificate             `json:"certificates,omitempty"`
	PropertySets           []PropertySet             `json:"property_sets,omitempty"`
	Proxies                []Proxy                   `json:"proxies,omitempty"`
	Backups                []Backup                  `json:"backups,omitempty"`
	MailServer             *MailServerConfig         `json:"mail_server,omitempty"`
	Trash                  *TrashConfig              `json:"trash,omitempty"`
	PasswordPolicy         *PasswordExpirationPolicy `json:"password_policy,omitempty"`
	UserLockPolicy         *UserLockPolicy           `json:"user_lock_policy,omitempty"`
	RepoLayouts            []RepoLayout              `json:"repo_layouts,omitempty"`
	ReverseProxy           *ReverseProxyConfig       `json:"reverse_proxy,omitempty"`
	ArchiveCleanupPolicies []ArchiveCleanupPolicy    `json:"archive_cleanup_policies,omitempty"`
	License                *LicenseInfo              `json:"license,omitempty"`
	Workers                []WorkerService           `json:"workers,omitempty"`
	Webhooks               []Webhook                 `json:"webhooks,omitempty"`
}

// Webhook is an Artifactory webhook subscription (GET
// /event/api/v1/subscriptions). Never captures the handler's signing
// secret, even though the API returns one.
type Webhook struct {
	Key         string   `json:"key"`
	Description string   `json:"description,omitempty"`
	Enabled     bool     `json:"enabled"`
	Domain      string   `json:"domain,omitempty"`
	EventTypes  []string `json:"event_types,omitempty"`
	HandlerURLs []string `json:"handler_urls,omitempty"`
}

// XrayPolicy is an Xray security/license/operational-risk policy (GET
// /xray/api/v1/policies). Rules are captured as a generic map so nested,
// policy-type-specific criteria (CVSS ranges, license lists, operational
// risk thresholds, ...) survive without a bespoke type per variant - the
// analyze report's per-row drawer shows the full structure on click.
type XrayPolicy struct {
	Name        string                   `json:"name"`
	Type        string                   `json:"type"`
	Description string                   `json:"description,omitempty"`
	Author      string                   `json:"author,omitempty"`
	Created     string                   `json:"created,omitempty"`
	Modified    string                   `json:"modified,omitempty"`
	Rules       []map[string]interface{} `json:"rules,omitempty"`
}

// XrayWatch is an Xray watch configuration (GET /xray/api/v2/watches). The
// wire response nests name/description/active/project_key under
// "general_data" and resources under "project_resources.resources" -
// internal/discover flattens that into this shape.
type XrayWatch struct {
	Name             string                   `json:"name"`
	Description      string                   `json:"description,omitempty"`
	Active           bool                     `json:"active"`
	ProjectKey       string                   `json:"project_key,omitempty"`
	Resources        []map[string]interface{} `json:"resources,omitempty"`
	AssignedPolicies []string                 `json:"assigned_policies,omitempty"`
	WatchRecipients  []string                 `json:"watch_recipients,omitempty"`
}

// XrayIgnoreRule is an Xray ignore rule (GET /xray/api/v1/ignore_rules,
// response wrapped as {"data": [...]}).
type XrayIgnoreRule struct {
	ID             string                 `json:"id"`
	Notes          string                 `json:"notes,omitempty"`
	Author         string                 `json:"author,omitempty"`
	ExpirationDate string                 `json:"expiration_date,omitempty"`
	IgnoreFilters  map[string]interface{} `json:"ignore_filters,omitempty"`
}

// XrayCurationPolicy is an Xray curation policy (GET
// /xray/api/v1/curation/policies, response wrapped as {"data": [...]}).
// Condition is captured as a generic map since its shape varies with the
// policy's condition type - the analyze report's drawer shows it in full.
type XrayCurationPolicy struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Enabled      bool                   `json:"enabled"`
	Scope        string                 `json:"scope,omitempty"`
	PolicyAction string                 `json:"policy_action,omitempty"`
	CreatedBy    string                 `json:"created_by,omitempty"`
	UpdatedBy    string                 `json:"updated_by,omitempty"`
	Condition    map[string]interface{} `json:"condition,omitempty"`
}

// CurationWaiverRequester is one person's request to unblock a package on a
// CurationWaiver (GET /xray/api/v1/curation/waiver_requests's "requesters").
type CurationWaiverRequester struct {
	User          string `json:"user"`
	Email         string `json:"email,omitempty"`
	RequestedAt   string `json:"requested_at,omitempty"`
	Justification string `json:"justification,omitempty"`
}

// XrayCurationWaiver is a curation waiver request - a request to unblock a
// specific package version that a curation policy blocked (GET
// /xray/api/v1/curation/waiver_requests, response wrapped as {"data": [...]}).
// Distinct from a XrayCurationPolicy itself: a policy defines what's
// blocked, a waiver is a (pending/approved/rejected) exception request
// against that block.
type XrayCurationWaiver struct {
	ID         int64                     `json:"id"`
	Status     string                    `json:"status"`
	CreatedAt  string                    `json:"created_at,omitempty"`
	ClosedAt   string                    `json:"closed_at,omitempty"`
	Requesters []CurationWaiverRequester `json:"requesters,omitempty"`
}

// XraySettings combines a few fields of Xray's global system parameters
// (GET /xray/api/v1/configuration/systemParameters) with the separate
// DB-sync time endpoint (GET /xray/api/v1/configuration/dbsync/time).
type XraySettings struct {
	MaxDiskDataUsage        int    `json:"max_disk_data_usage,omitempty"`
	MonitorSamplingInterval int    `json:"monitor_sampling_interval,omitempty"`
	JobInterval             int    `json:"job_interval,omitempty"`
	AllowSendingAnalytics   bool   `json:"allow_sending_analytics"`
	DBSyncUpdatesTime       string `json:"db_sync_updates_time,omitempty"`
}

// XrayConfig bundles Xray policy/watch/ignore-rule/curation configuration.
// Every field is independently soft-failed - Xray being unlicensed simply
// leaves this whole struct zero-valued (see Snapshot.System.XrayAvailable).
// CurationAvailable distinguishes "curation is enabled but has zero
// policies configured" from "curation itself isn't enabled/licensed here" -
// both otherwise look identical (an empty CurationPolicies slice).
// CurationWaivers is only fetched when CurationAvailable is true.
type XrayConfig struct {
	Policies          []XrayPolicy         `json:"policies,omitempty"`
	Watches           []XrayWatch          `json:"watches,omitempty"`
	IgnoreRules       []XrayIgnoreRule     `json:"ignore_rules,omitempty"`
	CurationAvailable bool                 `json:"curation_available,omitempty"`
	CurationPolicies  []XrayCurationPolicy `json:"curation_policies,omitempty"`
	CurationWaivers   []XrayCurationWaiver `json:"curation_waivers,omitempty"`
	Settings          *XraySettings        `json:"settings,omitempty"`
}

// AppTrustApplicationLabel is a key:value label attached to an AppTrust application.
type AppTrustApplicationLabel struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// AppTrustApplication is one application from GET /apptrust/api/v1/applications
// (response wraps the list in an "applications" array).
type AppTrustApplication struct {
	ApplicationKey  string                     `json:"application_key"`
	ApplicationName string                     `json:"application_name"`
	ProjectKey      string                     `json:"project_key,omitempty"`
	Description     string                     `json:"description,omitempty"`
	Criticality     string                     `json:"criticality,omitempty"` // unspecified|low|medium|high|critical
	MaturityLevel   string                     `json:"maturity_level,omitempty"` // unspecified|experimental|production|end_of_life
	UserOwners      []string                   `json:"user_owners,omitempty"`
	GroupOwners     []string                   `json:"group_owners,omitempty"`
	Labels          []AppTrustApplicationLabel `json:"labels,omitempty"`
}

// AppTrustPolicyScope defines which projects/applications a policy targets.
type AppTrustPolicyScope struct {
	ScopeType           string   `json:"scope_type,omitempty"` // global|project|application
	ProjectKeys         []string `json:"project_keys,omitempty"`
	ApplicationKeys     []string `json:"application_keys,omitempty"`
	ApplicationLabels   []string `json:"application_labels,omitempty"`
	ApplicationVersions []string `json:"application_versions,omitempty"`
}

// AppTrustPolicy is one policy from GET /unifiedpolicy/api/v1/policies
// (response wraps the list in an "items" array). Part of the JFrog Unified
// Policy Service, shared by AppTrust and Xray/Curation governance.
type AppTrustPolicy struct {
	ID                  string                 `json:"id"`
	Name                string                 `json:"name"`
	Description         string                 `json:"description,omitempty"`
	Enabled             bool                   `json:"enabled"`
	Mode                string                 `json:"mode,omitempty"` // block|warning
	Scope               AppTrustPolicyScope    `json:"scope,omitempty"`
	ActionType          string                 `json:"action_type,omitempty"`
	StageKey            string                 `json:"stage_key,omitempty"`
	StageGate           string                 `json:"stage_gate,omitempty"` // entry|exit|release
	RuleIDs             []string               `json:"rule_ids,omitempty"`
	WaiverRequestConfig string                 `json:"waiver_request_config,omitempty"` // forbidden|manual|auto_approved
	CreatedAt           string                 `json:"created_at,omitempty"`
	CreatedBy           string                 `json:"created_by,omitempty"`
	UpdatedAt           string                 `json:"updated_at,omitempty"`
	UpdatedBy           string                 `json:"updated_by,omitempty"`
}

// AppTrustRule is one rule from GET /unifiedpolicy/api/v1/rules
// (response wraps the list in an "items" array).
type AppTrustRule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	TemplateID  string `json:"template_id,omitempty"`
	IsCustom    bool   `json:"is_custom"`
	CreatedAt   string `json:"created_at,omitempty"`
	CreatedBy   string `json:"created_by,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	UpdatedBy   string `json:"updated_by,omitempty"`
}

// AppTrustTemplate is one template from GET /unifiedpolicy/api/v1/templates
// (response wraps the list in an "items" array) - the rule type/logic a Rule
// instantiates (e.g. a CVSS-threshold check backed by a rego policy).
type AppTrustTemplate struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	Version        string   `json:"version,omitempty"`
	Category       string   `json:"category,omitempty"` // security|legal|operational|quality|audit|workflow
	Scanners       []string `json:"scanners,omitempty"`
	DataSourceType string   `json:"data_source_type,omitempty"`
	IsCustom       bool     `json:"is_custom"`
	CreatedAt      string   `json:"created_at,omitempty"`
	CreatedBy      string   `json:"created_by,omitempty"`
}

// AppTrustGateInfo is one gate within a lifecycle stage, with the count of
// policies attached to it (from the lifecycle/stages endpoint's gates_info).
type AppTrustGateInfo struct {
	Name          string `json:"name"`
	PoliciesCount int    `json:"policies_count"`
}

// AppTrustStage is one lifecycle stage for a project, from
// GET /apptrust/api/v1/projects/{project_key}/lifecycle/stages. ProjectKey is
// filled in by the caller (the endpoint is per-project, not a global list).
type AppTrustStage struct {
	ProjectKey string             `json:"project_key"`
	Category   string             `json:"category,omitempty"` // none|code|promote|release
	StageName  string             `json:"stage_name"`
	Scope      string             `json:"scope,omitempty"` // global|project
	Gates      []string           `json:"gates,omitempty"`
	GatesInfo  []AppTrustGateInfo `json:"gates_info,omitempty"`
}

// AppTrustEvidence is one evidence record from GET /evidence/api/v1/evidence-search
// (response wraps the list in an "evidence" array) - DSSE/in-toto attestations
// attached to a build, package, or release bundle.
type AppTrustEvidence struct {
	ID                 string `json:"id"`
	Repository         string `json:"repository,omitempty"`
	Path               string `json:"path,omitempty"`
	Name               string `json:"name,omitempty"`
	PredicateCategory  string `json:"predicate_category,omitempty"`
	PredicateType      string `json:"predicate_type,omitempty"`
	PredicateSlug      string `json:"predicate_slug,omitempty"`
	CreatedAt          string `json:"created_at,omitempty"`
	CreatedBy          string `json:"created_by,omitempty"`
	Verified           bool   `json:"verified,omitempty"`
	Stage              string `json:"stage,omitempty"`
}

// AppTrustReleaseBundle is one Release Bundle v2 name/repo pairing from
// GET /lifecycle/api/v2/release_bundle/names (response wraps the list in a
// "release_bundles" array) - the name itself, not a specific version.
type AppTrustReleaseBundle struct {
	RepositoryKey     string `json:"repository_key"`
	ReleaseBundleName string `json:"release_bundle_name"`
	ProjectKey        string `json:"project_key,omitempty"`
	ProjectName       string `json:"project_name,omitempty"`
}

// AppTrustApplicationPolicy is a junction showing which applications a
// policy targets (and the scope mechanism: project, application key, or label).
type AppTrustApplicationPolicy struct {
	ApplicationKey string `json:"application_key"`
	ApplicationName string `json:"application_name"`
	ProjectKey     string `json:"project_key,omitempty"`
	PolicyID       string `json:"policy_id"`
	PolicyName     string `json:"policy_name"`
	ScopeType      string `json:"scope_type"` // how is this app targeted: global|project|application|label
	ScopeMechanism string `json:"scope_mechanism,omitempty"` // which project_key/app_key/label matched
	PolicyMode     string `json:"policy_mode,omitempty"` // block|warning
	PolicyEnabled  bool   `json:"policy_enabled"`
}

// AppTrustPolicyRule is a junction showing which rules implement which policies.
type AppTrustPolicyRule struct {
	PolicyID    string `json:"policy_id"`
	PolicyName  string `json:"policy_name"`
	RuleID      string `json:"rule_id"`
	RuleName    string `json:"rule_name"`
	RuleOrder   int    `json:"rule_order,omitempty"` // position in policy's rule list
	TemplateID  string `json:"template_id,omitempty"`
	TemplateName string `json:"template_name,omitempty"`
}

// ProjectRepository represents a project-to-repo mapping
type ProjectRepository struct {
	ProjectKey string `json:"project_key"`
	RepoKey    string `json:"repo_key"`
	PackageType string `json:"package_type,omitempty"`
	RepoType   string `json:"repo_type,omitempty"`
}

// Snapshot is the full instance-wide discovery result, written/read as one
// JSON file by internal/discover's "discover instance" and consumed by
// "discover analyze --instance".
type Snapshot struct {
	GeneratedAt string `json:"generated_at"`
	// URL is the JPD base URL used to collect this snapshot (e.g.
	// "https://psazuse.jfrog.io"), and Name is the short instance-identifying
	// name NameFromURL derives from it (e.g. "psazuse") - the analyze report
	// displays both, and jfps discover's --output default directory naming
	// uses the same Name so the two stay consistent.
	URL      string       `json:"url,omitempty"`
	Name     string       `json:"name,omitempty"`
	System   SystemInfo   `json:"system"`
	Security SecurityInfo `json:"security"`

	Users          []User `json:"users,omitempty"`
	UsersTruncated bool   `json:"users_truncated,omitempty"`

	Groups          []Group `json:"groups,omitempty"`
	GroupsTruncated bool    `json:"groups_truncated,omitempty"`

	Permissions          []PermissionTarget `json:"permissions,omitempty"`
	PermissionsTruncated bool               `json:"permissions_truncated,omitempty"`

	Projects []Project `json:"projects,omitempty"`

	Replications   []ReplicationConfig `json:"replications,omitempty"`
	FederatedRepos []FederatedRepo     `json:"federated_repos,omitempty"`

	AppTrustPolicies          []AppTrustPolicy           `json:"apptrust_policies,omitempty"`
	AppTrustApplications      []AppTrustApplication      `json:"apptrust_applications,omitempty"`
	AppTrustRules             []AppTrustRule             `json:"apptrust_rules,omitempty"`
	AppTrustTemplates         []AppTrustTemplate         `json:"apptrust_templates,omitempty"`
	AppTrustStages            []AppTrustStage            `json:"apptrust_stages,omitempty"`
	AppTrustEvidence          []AppTrustEvidence         `json:"apptrust_evidence,omitempty"`
	AppTrustReleaseBundles    []AppTrustReleaseBundle    `json:"apptrust_release_bundles,omitempty"`
	AppTrustApplicationPolicies []AppTrustApplicationPolicy `json:"apptrust_application_policies,omitempty"`
	AppTrustPolicyRules       []AppTrustPolicyRule       `json:"apptrust_policy_rules,omitempty"`
	ProjectRepos              []ProjectRepository        `json:"project_repos,omitempty"`

	Platform PlatformConfig `json:"platform"`
	Xray     XrayConfig     `json:"xray"`

	// Warnings collects non-fatal problems (a single permission-detail
	// fetch failing, a soft-skipped optional service, etc.) so the whole
	// collection doesn't abort over one bad call.
	Warnings []string `json:"warnings,omitempty"`
}

// NameFromURL derives a short instance-identifying name from a JPD base
// URL, e.g. "https://psazuse.jfrog.io" -> "psazuse",
// "https://jfrog-a.cloudlab.rocks" -> "jfrog-a". Falls back to the full
// host for a bare IP (splitting "10.0.0.1" at the first "." would
// collide two different instances on "172"), and to "default" if rawURL is
// empty or doesn't parse into a usable host at all.
func NameFromURL(rawURL string) string {
	const fallback = "default"
	if rawURL == "" {
		return fallback
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return fallback
	}
	host := u.Hostname()
	if net.ParseIP(host) != nil {
		return host
	}
	if i := strings.IndexByte(host, '.'); i > 0 {
		return host[:i]
	}
	return host
}
