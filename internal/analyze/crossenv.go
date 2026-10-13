package analyze

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type CrossEnvRepoRecord struct {
	Name             string
	PackageType      string
	RepoType         string
	ProjectKey       string
	FilesCount       string
	UsageGB          string
	CreatedDate      string
	CreatedBy        string
	LastPushDate     string
	LastPushBy       string
	LastPullDate     string
	LastActivityDate string
	CleanupPolicies  string
}

type CrossEnvEnvironment struct {
	Name  string
	Repos []CrossEnvRepoRecord
}

type CrossEnvDuplicate struct {
	Name         string
	Environments []string
	Repos        map[string]CrossEnvRepoRecord
}

type CrossEnvReport struct {
	GeneratedAt      string
	Environments     []CrossEnvEnvironment
	Duplicates       []*CrossEnvDuplicate
	TotalRepos       int
	DuplicateCount   int
	EnvironmentStats map[string]interface{}
}

// AnalyzeCrossEnvironments scans the output directory for discovery outputs
// from multiple environments and generates a cross-environment report.
func AnalyzeCrossEnvironments(outputDir string) (*CrossEnvReport, error) {
	envs, err := discoverEnvironments(outputDir)
	if err != nil {
		return nil, err
	}

	if len(envs) == 0 {
		return nil, fmt.Errorf("no discovery outputs found in %s", outputDir)
	}

	return generateCrossEnvReport(envs), nil
}

func discoverEnvironments(outputDir string) ([]CrossEnvEnvironment, error) {
	var envs []CrossEnvEnvironment

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		envPath := filepath.Join(outputDir, entry.Name())
		reposPath := filepath.Join(envPath, "report.csv")

		// Check if report.csv exists
		if _, err := os.Stat(reposPath); err == nil {
			repos, err := readRepoCSV(reposPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to read %s: %v\n", reposPath, err)
				continue
			}

			envs = append(envs, CrossEnvEnvironment{
				Name:  entry.Name(),
				Repos: repos,
			})
		}
	}

	sort.Slice(envs, func(i, j int) bool {
		return envs[i].Name < envs[j].Name
	})

	return envs, nil
}

func readRepoCSV(filePath string) ([]CrossEnvRepoRecord, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	headers, err := reader.Read()
	if err != nil {
		return nil, err
	}

	// Map column names to indices
	colMap := make(map[string]int)
	for i, h := range headers {
		colMap[h] = i
	}

	var records []CrossEnvRepoRecord
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		record := CrossEnvRepoRecord{
			Name:             getOrEmpty(row, colMap, "repo_name"),
			PackageType:      getOrEmpty(row, colMap, "package_type"),
			RepoType:         getOrEmpty(row, colMap, "repo_type"),
			ProjectKey:       getOrEmpty(row, colMap, "project_key"),
			FilesCount:       getOrEmpty(row, colMap, "files_count"),
			UsageGB:          getOrEmpty(row, colMap, "usage_gb"),
			CreatedDate:      getOrEmpty(row, colMap, "created_date"),
			CreatedBy:        getOrEmpty(row, colMap, "created_by"),
			LastPushDate:     getOrEmpty(row, colMap, "last_push_date"),
			LastPushBy:       getOrEmpty(row, colMap, "last_push_by"),
			LastPullDate:     getOrEmpty(row, colMap, "last_pull_date"),
			LastActivityDate: getOrEmpty(row, colMap, "last_activity_date"),
			CleanupPolicies:  getOrEmpty(row, colMap, "cleanup_policy_keys"),
		}
		records = append(records, record)
	}

	return records, nil
}

func getOrEmpty(row []string, colMap map[string]int, colName string) string {
	if idx, ok := colMap[colName]; ok && idx < len(row) {
		return row[idx]
	}
	return ""
}

func generateCrossEnvReport(envs []CrossEnvEnvironment) *CrossEnvReport {
	// Build repo name to environments map
	repoMap := make(map[string]*CrossEnvDuplicate)

	for _, env := range envs {
		for _, repo := range env.Repos {
			if repo.Name == "" {
				continue
			}

			if _, exists := repoMap[repo.Name]; !exists {
				repoMap[repo.Name] = &CrossEnvDuplicate{
					Name:         repo.Name,
					Environments: []string{},
					Repos:        make(map[string]CrossEnvRepoRecord),
				}
			}
			repoMap[repo.Name].Environments = append(repoMap[repo.Name].Environments, env.Name)
			repoMap[repo.Name].Repos[env.Name] = repo
		}
	}

	// Find duplicates
	var duplicates []*CrossEnvDuplicate
	for _, dup := range repoMap {
		if len(dup.Environments) > 1 {
			duplicates = append(duplicates, dup)
		}
	}

	sort.Slice(duplicates, func(i, j int) bool {
		return duplicates[i].Name < duplicates[j].Name
	})

	// Build environment stats
	envStats := make(map[string]interface{})
	for _, env := range envs {
		stats := map[string]interface{}{
			"repo_count":       len(env.Repos),
			"duplicate_count": 0,
		}
		for _, dup := range duplicates {
			for _, dupEnv := range dup.Environments {
				if dupEnv == env.Name {
					stats["duplicate_count"] = stats["duplicate_count"].(int) + 1
					break
				}
			}
		}
		envStats[env.Name] = stats
	}

	return &CrossEnvReport{
		Environments:     envs,
		Duplicates:       duplicates,
		TotalRepos:       len(repoMap),
		DuplicateCount:   len(duplicates),
		EnvironmentStats: envStats,
	}
}

// WriteCrossEnvReport writes text, JSON, and HTML versions of the report
func WriteCrossEnvReport(report *CrossEnvReport, textPath string) error {
	baseDir := filepath.Dir(textPath)
	baseName := strings.TrimSuffix(filepath.Base(textPath), filepath.Ext(textPath))

	// Write text report
	if err := writeCrossEnvTextReport(report, textPath); err != nil {
		return err
	}

	// Write JSON report
	jsonPath := filepath.Join(baseDir, baseName+".json")
	if err := writeCrossEnvJSONReport(report, jsonPath); err != nil {
		return err
	}

	// Write HTML report
	htmlPath := filepath.Join(baseDir, baseName+".html")
	if err := writeCrossEnvHTMLReport(report, htmlPath); err != nil {
		return err
	}

	return nil
}

func writeCrossEnvTextReport(report *CrossEnvReport, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Fprintf(file, "===============================================\n")
	fmt.Fprintf(file, "CROSS-ENVIRONMENT DISCOVERY ANALYSIS REPORT\n")
	fmt.Fprintf(file, "Generated: %s\n", report.GeneratedAt)
	fmt.Fprintf(file, "===============================================\n\n")

	// Summary
	fmt.Fprintf(file, "SUMMARY\n")
	fmt.Fprintf(file, "-------\n")
	fmt.Fprintf(file, "Total Environments: %d\n", len(report.Environments))
	fmt.Fprintf(file, "Total Unique Repos Across All Environments: %d\n", report.TotalRepos)
	fmt.Fprintf(file, "Repos Found in Multiple Environments: %d\n", report.DuplicateCount)
	fmt.Fprintf(file, "\n")

	// Environment breakdown
	fmt.Fprintf(file, "ENVIRONMENT BREAKDOWN\n")
	fmt.Fprintf(file, "--------------------\n")
	for _, env := range report.Environments {
		stats := report.EnvironmentStats[env.Name].(map[string]interface{})
		duplicateCount := stats["duplicate_count"].(int)
		fmt.Fprintf(file, "  %s:\n", env.Name)
		fmt.Fprintf(file, "    - Total Repos: %d\n", len(env.Repos))
		fmt.Fprintf(file, "    - In Multiple Environments: %d\n", duplicateCount)
		fmt.Fprintf(file, "\n")
	}

	// Detailed duplicates
	fmt.Fprintf(file, "\n")
	fmt.Fprintf(file, "DUPLICATE REPOSITORIES (Found in Multiple Environments)\n")
	fmt.Fprintf(file, "======================================================\n\n")

	if len(report.Duplicates) == 0 {
		fmt.Fprintf(file, "No duplicate repositories found.\n")
	} else {
		for i, dup := range report.Duplicates {
			fmt.Fprintf(file, "[%d] %s\n", i+1, dup.Name)
			fmt.Fprintf(file, "    ⚠️  DUPLICATE FOUND IN: %s\n", strings.Join(dup.Environments, ", "))
			fmt.Fprintf(file, "    Details:\n")

			for _, env := range dup.Environments {
				repo := dup.Repos[env]
				fmt.Fprintf(file, "      %s:\n", env)
				fmt.Fprintf(file, "        Type: %s / %s\n", repo.PackageType, repo.RepoType)
				fmt.Fprintf(file, "        Project: %s\n", repo.ProjectKey)
				fmt.Fprintf(file, "        Size: %s files | %s GB\n", repo.FilesCount, repo.UsageGB)
				fmt.Fprintf(file, "        Created: %s by %s\n", repo.CreatedDate, repo.CreatedBy)
				fmt.Fprintf(file, "        Last Activity: %s\n", repo.LastActivityDate)
				if repo.CleanupPolicies != "" {
					fmt.Fprintf(file, "        Policies: %s\n", repo.CleanupPolicies)
				}
			}
			fmt.Fprintf(file, "\n")
		}
	}

	return nil
}

func writeCrossEnvJSONReport(report *CrossEnvReport, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Build a JSON-friendly representation
	envSummaries := make([]map[string]interface{}, 0)
	for _, env := range report.Environments {
		stats := report.EnvironmentStats[env.Name].(map[string]interface{})
		envSummaries = append(envSummaries, map[string]interface{}{
			"name":                env.Name,
			"repo_count":          stats["repo_count"],
			"duplicate_count":     stats["duplicate_count"],
		})
	}

	jsonReport := map[string]interface{}{
		"generated_at": report.GeneratedAt,
		"summary": map[string]interface{}{
			"total_environments":    len(report.Environments),
			"total_unique_repos":    report.TotalRepos,
			"duplicate_count":       report.DuplicateCount,
		},
		"environments": envSummaries,
		"duplicates":   report.Duplicates,
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonReport)
}

func writeCrossEnvHTMLReport(report *CrossEnvReport, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Build environment statistics for the HTML
	envStats := make([]map[string]interface{}, 0)
	for _, env := range report.Environments {
		stats := report.EnvironmentStats[env.Name].(map[string]interface{})
		envStats = append(envStats, map[string]interface{}{
			"name":               env.Name,
			"repo_count":         stats["repo_count"],
			"duplicate_count":    stats["duplicate_count"],
		})
	}

	// Build duplicate rows for the table
	duplicateRows := make([]map[string]interface{}, 0)
	for _, dup := range report.Duplicates {
		details := make([]map[string]interface{}, 0)
		for _, envName := range dup.Environments {
			repo := dup.Repos[envName]
			details = append(details, map[string]interface{}{
				"environment":    envName,
				"package_type":   repo.PackageType,
				"repo_type":      repo.RepoType,
				"project_key":    repo.ProjectKey,
				"files_count":    repo.FilesCount,
				"usage_gb":       repo.UsageGB,
				"created_date":   repo.CreatedDate,
				"created_by":     repo.CreatedBy,
				"last_activity":  repo.LastActivityDate,
				"policies":       repo.CleanupPolicies,
			})
		}
		duplicateRows = append(duplicateRows, map[string]interface{}{
			"name":         dup.Name,
			"environments": strings.Join(dup.Environments, ", "),
			"env_count":    len(dup.Environments),
			"details":      details,
		})
	}

	data := map[string]interface{}{
		"generated_at":      report.GeneratedAt,
		"total_environments": len(report.Environments),
		"total_repos":       report.TotalRepos,
		"duplicate_count":   report.DuplicateCount,
		"environments":      envStats,
		"duplicates":        duplicateRows,
	}

	html, err := renderCrossEnvHTML(data)
	if err != nil {
		return err
	}

	_, err = file.WriteString(html)
	return err
}

func renderCrossEnvHTML(data map[string]interface{}) (string, error) {
	tmpl := template.Must(template.New("crossenv").Parse(crossEnvHTMLTemplate))
	var result strings.Builder
	if err := tmpl.Execute(&result, data); err != nil {
		return "", err
	}
	return result.String(), nil
}

const crossEnvHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Cross-Environment Discovery Analysis</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }

        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            border-radius: 12px;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
            overflow: hidden;
        }

        header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 40px;
            text-align: center;
        }

        header h1 {
            font-size: 32px;
            margin-bottom: 10px;
        }

        header p {
            opacity: 0.9;
            font-size: 14px;
        }

        .content {
            padding: 40px;
        }

        .summary-cards {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 40px;
        }

        .card {
            background: #f8f9fa;
            border-left: 4px solid #667eea;
            padding: 20px;
            border-radius: 8px;
            text-align: center;
        }

        .card h3 {
            font-size: 24px;
            color: #667eea;
            margin-bottom: 5px;
        }

        .card p {
            color: #6c757d;
            font-size: 14px;
        }

        section {
            margin-bottom: 40px;
        }

        section h2 {
            font-size: 20px;
            color: #333;
            margin-bottom: 20px;
            border-bottom: 2px solid #667eea;
            padding-bottom: 10px;
        }

        .environments-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }

        .env-card {
            background: #f8f9fa;
            border: 1px solid #e9ecef;
            border-radius: 8px;
            padding: 20px;
            transition: transform 0.2s;
        }

        .env-card:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(102, 126, 234, 0.15);
        }

        .env-card h3 {
            color: #667eea;
            margin-bottom: 10px;
            font-size: 16px;
        }

        .env-stat {
            display: flex;
            justify-content: space-between;
            padding: 8px 0;
            font-size: 14px;
            border-bottom: 1px solid #e9ecef;
        }

        .env-stat:last-child {
            border-bottom: none;
        }

        .env-stat-value {
            font-weight: 600;
            color: #667eea;
        }

        .duplicates-section {
            margin-top: 30px;
        }

        .duplicate-item {
            background: #fff3cd;
            border-left: 4px solid #ffc107;
            margin-bottom: 20px;
            border-radius: 4px;
            overflow: hidden;
        }

        .duplicate-header {
            background: #fff3cd;
            padding: 15px 20px;
            cursor: pointer;
            display: flex;
            justify-content: space-between;
            align-items: center;
            transition: background 0.2s;
        }

        .duplicate-header:hover {
            background: #ffe69c;
        }

        .duplicate-header h3 {
            font-size: 16px;
            color: #856404;
            margin: 0;
            flex: 1;
        }

        .duplicate-header .badge {
            background: #ffc107;
            color: #000;
            padding: 4px 12px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: 600;
            margin-left: 10px;
        }

        .duplicate-content {
            display: none;
            padding: 20px;
            background: #fffbf0;
            border-top: 1px solid #ffe69c;
        }

        .duplicate-item.expanded .duplicate-content {
            display: block;
        }

        .duplicate-arrow {
            transition: transform 0.2s;
            font-size: 18px;
        }

        .duplicate-item.expanded .duplicate-arrow {
            transform: rotate(180deg);
        }

        .env-details {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 20px;
            margin-top: 15px;
        }

        .detail-card {
            background: white;
            border: 1px solid #ffe69c;
            border-radius: 4px;
            padding: 15px;
        }

        .detail-card h4 {
            color: #667eea;
            font-size: 14px;
            margin-bottom: 10px;
            border-bottom: 1px solid #e9ecef;
            padding-bottom: 8px;
        }

        .detail-row {
            display: flex;
            justify-content: space-between;
            padding: 6px 0;
            font-size: 13px;
            border-bottom: 1px solid #f0f0f0;
        }

        .detail-row:last-child {
            border-bottom: none;
        }

        .detail-label {
            font-weight: 600;
            color: #6c757d;
        }

        .detail-value {
            color: #333;
            text-align: right;
            word-break: break-word;
        }

        .no-duplicates {
            background: #d4edda;
            border: 1px solid #c3e6cb;
            color: #155724;
            padding: 20px;
            border-radius: 4px;
            text-align: center;
        }

        .footer {
            background: #f8f9fa;
            padding: 20px 40px;
            text-align: center;
            font-size: 12px;
            color: #6c757d;
            border-top: 1px solid #e9ecef;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>Cross-Environment Discovery Analysis</h1>
            <p>Generated: {{.generated_at}}</p>
        </header>

        <div class="content">
            <div class="summary-cards">
                <div class="card">
                    <h3>{{.total_environments}}</h3>
                    <p>Environments Analyzed</p>
                </div>
                <div class="card">
                    <h3>{{.total_repos}}</h3>
                    <p>Total Unique Repositories</p>
                </div>
                <div class="card">
                    <h3>{{.duplicate_count}}</h3>
                    <p>Repositories in Multiple Environments</p>
                </div>
            </div>

            <section>
                <h2>Environment Summary</h2>
                <div class="environments-grid">
                    {{range .environments}}
                    <div class="env-card">
                        <h3>{{.name}}</h3>
                        <div class="env-stat">
                            <span>Total Repos:</span>
                            <span class="env-stat-value">{{.repo_count}}</span>
                        </div>
                        <div class="env-stat">
                            <span>Duplicates Found:</span>
                            <span class="env-stat-value">{{.duplicate_count}}</span>
                        </div>
                    </div>
                    {{end}}
                </div>
            </section>

            <section class="duplicates-section">
                <h2>Duplicate Repositories</h2>
                {{if .duplicates}}
                    {{range .duplicates}}
                    <div class="duplicate-item" onclick="this.classList.toggle('expanded')">
                        <div class="duplicate-header">
                            <h3>{{.name}}</h3>
                            <span class="badge">{{.env_count}} environments</span>
                            <div class="duplicate-arrow">▼</div>
                        </div>
                        <div class="duplicate-content">
                            <p><strong>Found in:</strong> {{.environments}}</p>
                            <div class="env-details">
                                {{range .details}}
                                <div class="detail-card">
                                    <h4>{{.environment}}</h4>
                                    <div class="detail-row">
                                        <span class="detail-label">Type:</span>
                                        <span class="detail-value">{{.package_type}} / {{.repo_type}}</span>
                                    </div>
                                    <div class="detail-row">
                                        <span class="detail-label">Project:</span>
                                        <span class="detail-value">{{.project_key}}</span>
                                    </div>
                                    <div class="detail-row">
                                        <span class="detail-label">Files:</span>
                                        <span class="detail-value">{{.files_count}}</span>
                                    </div>
                                    <div class="detail-row">
                                        <span class="detail-label">Usage:</span>
                                        <span class="detail-value">{{.usage_gb}} GB</span>
                                    </div>
                                    <div class="detail-row">
                                        <span class="detail-label">Created:</span>
                                        <span class="detail-value">{{.created_date}}</span>
                                    </div>
                                    {{if .created_by}}
                                    <div class="detail-row">
                                        <span class="detail-label">By:</span>
                                        <span class="detail-value">{{.created_by}}</span>
                                    </div>
                                    {{end}}
                                    <div class="detail-row">
                                        <span class="detail-label">Last Activity:</span>
                                        <span class="detail-value">{{.last_activity}}</span>
                                    </div>
                                    {{if .policies}}
                                    <div class="detail-row">
                                        <span class="detail-label">Policies:</span>
                                        <span class="detail-value">{{.policies}}</span>
                                    </div>
                                    {{end}}
                                </div>
                                {{end}}
                            </div>
                        </div>
                    </div>
                    {{end}}
                {{else}}
                    <div class="no-duplicates">
                        ✓ No duplicate repositories found across environments
                    </div>
                {{end}}
            </section>
        </div>

        <div class="footer">
            <p>This report identifies repositories with the same name across multiple environments.</p>
        </div>
    </div>
</body>
</html>`
