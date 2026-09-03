package xray

// Report types accepted by "jfps xray reports generate".
const (
	ReportTypeViolations      = "violations"
	ReportTypeVulnerabilities = "vulnerabilities"
	ReportTypeExposures       = "exposures"
)

// dateRange is the {"start":..., "end":...} shape shared by several filter
// fields (updated/published/scan_date). Empty fields are omitted so a
// partially-specified range degrades to whichever bound was actually given.
type dateRange struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

func (d *dateRange) empty() bool {
	return d == nil || (d.Start == "" && d.End == "")
}

// cvssRange is the {"min_score":..., "max_score":...} shape of the
// vulnerabilities filter's cvss_score field.
type cvssRange struct {
	MinScore float64 `json:"min_score,omitempty"`
	MaxScore float64 `json:"max_score,omitempty"`
}

func (c *cvssRange) empty() bool {
	return c == nil || (c.MinScore == 0 && c.MaxScore == 0)
}

// repositoryResource is one element of resources.repositories.
type repositoryResource struct {
	Name string `json:"name"`
}

// buildWithRepo names one build and the build-info repository it lives in --
// required by the Xray API to resolve scan data for a build resource; a
// bare build name list alone silently returns zero rows.
type buildWithRepo struct {
	Build string `json:"build"`
	Repo  string `json:"repo"`
}

// buildsResource is resources.builds.
type buildsResource struct {
	Names                  []string        `json:"names"`
	NumberOfLatestVersions int             `json:"number_of_latest_versions,omitempty"`
	BuildsWithRepos        []buildWithRepo `json:"builds_with_repos,omitempty"`
}

// releaseBundlesResource is resources.release_bundles.
type releaseBundlesResource struct {
	Names                  []string `json:"names"`
	NumberOfLatestVersions int      `json:"number_of_latest_versions,omitempty"`
}

// projectsResource is resources.projects.
type projectsResource struct {
	Names []string `json:"names"`
}

// reportResources is the "resources" object of a report-generation request.
// Exactly one field may be set -- the Xray API 400s if more than one
// resource kind is present in the same request.
type reportResources struct {
	Repositories   []repositoryResource    `json:"repositories,omitempty"`
	Builds         *buildsResource         `json:"builds,omitempty"`
	ReleaseBundles *releaseBundlesResource `json:"release_bundles,omitempty"`
	Projects       *projectsResource       `json:"projects,omitempty"`
}

// reportFilters is the union of every field used across the three report
// types' filter schemas. Only the fields relevant to the report type in use
// are ever populated; omitempty drops the rest, matching the source
// scripts' "delete the key entirely" behavior for inapplicable fields.
type reportFilters struct {
	// violations / vulnerabilities (both are "security" reports)
	Type       string     `json:"type,omitempty"`
	Severities []string   `json:"severities,omitempty"`
	WatchNames []string   `json:"watch_names,omitempty"`
	Updated    *dateRange `json:"updated,omitempty"`

	// vulnerabilities only
	HasRemediation *bool      `json:"has_remediation,omitempty"`
	CVE            string     `json:"cve,omitempty"`
	IssueID        string     `json:"issue_id,omitempty"`
	CVSSScore      *cvssRange `json:"cvss_score,omitempty"`
	Published      *dateRange `json:"published,omitempty"`
	ScanDate       *dateRange `json:"scan_date,omitempty"`

	// exposures only
	Category         string `json:"category,omitempty"`
	ImpactedArtifact string `json:"impacted_artifact,omitempty"`
}

// reportRequest is the full body of
// POST /xray/api/v1/reports/{violations|vulnerabilities|exposures}.
type reportRequest struct {
	Name      string          `json:"name"`
	Resources reportResources `json:"resources"`
	Filters   reportFilters   `json:"filters"`
}

// createReportResponse is the response of the report-creation POST.
type createReportResponse struct {
	ReportID int `json:"report_id"`
}

// reportStatusResponse is the response of GET /xray/api/v1/reports/{id}.
type reportStatusResponse struct {
	Status string `json:"status"`
	Name   string `json:"name"`
}

// Terminal (and non-terminal) values of reportStatusResponse.Status.
const (
	reportStatusPending   = "pending"
	reportStatusRunning   = "running"
	reportStatusCompleted = "completed"
	reportStatusAborted   = "aborted"
	reportStatusFailed    = "failed"
)

// ReportScope selects which JPD resources a generated report covers.
// Exactly one of Reponame, Project, Buildname, Watchname, or
// (ResourceType == "builds" alone) must be set -- see ResolveReportScope.
type ReportScope struct {
	Reponame     string
	Project      string
	Buildname    string
	BuildRepo    string // required with Buildname: the build-info repo it lives in
	Watchname    string
	ResourceType string // "builds": instance-wide build discovery when no other scope field is set
	RepoRClass   string // optional --repo-rclass filter applied only with --project
}

// ReportGenOptions configures a single "jfps xray reports generate"
// invocation (before any date-range chunking is applied).
type ReportGenOptions struct {
	ReportType string
	Scope      ReportScope

	Severities   []string
	UpdatedStart string
	UpdatedEnd   string
	DaysPerChunk int

	BuildNumberOfVersions int // default 5 when 0

	ExposureCategory         string
	ExposureImpactedArtifact string
	ScanDateStart            string
	ScanDateEnd              string

	HasRemediation *bool
	CVE            string
	IssueID        string
	CVSSMinScore   float64
	CVSSMaxScore   float64
	PublishedStart string
	PublishedEnd   string

	BuildGitRepoReport bool
	ReportUploadRepo   string
	OutputDir          string
}

// reportNamePrefix returns the filename prefix used by the source scripts
// for each report type.
func reportNamePrefix(reportType string) string {
	switch reportType {
	case ReportTypeVulnerabilities:
		return "vulnerabilityreport"
	case ReportTypeExposures:
		return "exposuresreport"
	default:
		return "violationreport"
	}
}
