package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type RepoRecord struct {
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

type InstanceData struct {
	SystemInfo interface{}   `json:"system_info"`
	Users      []interface{} `json:"users"`
	Groups     []interface{} `json:"groups"`
	Projects   []ProjectData `json:"projects"`
	Storage    interface{}   `json:"storage"`
}

type ProjectData struct {
	Key          string                 `json:"key"`
	DisplayName  string                 `json:"display_name"`
	Description  string                 `json:"description"`
	Repositories []string               `json:"repositories"`
	Roles        []ProjectRoleData       `json:"roles"`
	Admin        map[string]interface{} `json:"admin,omitempty"`
}

type ProjectRoleData struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Environments []string `json:"environments,omitempty"`
	Actions      []string `json:"actions,omitempty"`
}

type EnvironmentData struct {
	Name     string
	Repos    []RepoRecord
	Instance *InstanceData
}

type DuplicateRepo struct {
	Name         string
	Environments []string
	Repos        map[string]RepoRecord
}

func main() {
	outputDir := flag.String("output", "output", "Path to discovery output directory")
	reportFile := flag.String("report", "cross-env-analysis.txt", "Output report file")
	flag.Parse()

	// Discover all environments
	envs, err := discoverEnvironments(*outputDir)
	if err != nil {
		log.Fatalf("Failed to discover environments: %v", err)
	}

	if len(envs) == 0 {
		fmt.Println("No discovery output found in", *outputDir)
		return
	}

	fmt.Printf("Found %d environment(s): %v\n", len(envs), getEnvNames(envs))

	// Generate report
	report := generateReport(envs)

	// Write report to file
	if err := writeReport(*reportFile, report); err != nil {
		log.Fatalf("Failed to write report: %v", err)
	}

	fmt.Printf("Report written to %s\n", *reportFile)
	printReportSummary(report)
}

func discoverEnvironments(outputDir string) ([]EnvironmentData, error) {
	var envs []EnvironmentData

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
		instancePath := filepath.Join(envPath, "instance.json")

		// Check if report.csv exists
		repos := []RepoRecord{}
		if _, err := os.Stat(reposPath); err == nil {
			r, err := readRepoCSV(reposPath)
			if err != nil {
				fmt.Printf("Warning: Failed to read %s: %v\n", reposPath, err)
			} else {
				repos = r
			}
		}

		// Try to read instance.json
		var instance *InstanceData
		if _, err := os.Stat(instancePath); err == nil {
			inst, err := readInstanceJSON(instancePath)
			if err != nil {
				fmt.Printf("Warning: Failed to read %s: %v\n", instancePath, err)
			} else {
				instance = inst
			}
		}

		// Only add environment if we have at least repos or instance data
		if len(repos) > 0 || instance != nil {
			envs = append(envs, EnvironmentData{
				Name:     entry.Name(),
				Repos:    repos,
				Instance: instance,
			})
		}
	}

	sort.Slice(envs, func(i, j int) bool {
		return envs[i].Name < envs[j].Name
	})

	return envs, nil
}

func readRepoCSV(filePath string) ([]RepoRecord, error) {
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

	var records []RepoRecord
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		record := RepoRecord{
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

func readInstanceJSON(filePath string) (*InstanceData, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var instance InstanceData
	if err := json.NewDecoder(file).Decode(&instance); err != nil {
		return nil, err
	}

	return &instance, nil
}

func generateReport(envs []EnvironmentData) map[string]interface{} {
	// Build repo name to environments map
	repoMap := make(map[string]*DuplicateRepo)

	for _, env := range envs {
		for _, repo := range env.Repos {
			if repo.Name == "" {
				continue
			}

			if _, exists := repoMap[repo.Name]; !exists {
				repoMap[repo.Name] = &DuplicateRepo{
					Name:         repo.Name,
					Environments: []string{},
					Repos:        make(map[string]RepoRecord),
				}
			}
			repoMap[repo.Name].Environments = append(repoMap[repo.Name].Environments, env.Name)
			repoMap[repo.Name].Repos[env.Name] = repo
		}
	}

	// Find duplicates
	var duplicates []*DuplicateRepo
	uniqueByEnv := make(map[string]int)

	for _, dup := range repoMap {
		if len(dup.Environments) > 1 {
			duplicates = append(duplicates, dup)
		}
		for _, env := range dup.Environments {
			uniqueByEnv[env]++
		}
	}

	sort.Slice(duplicates, func(i, j int) bool {
		return duplicates[i].Name < duplicates[j].Name
	})

	// Group repositories by project
	projectRepos := groupRepositoriesByProject(envs)

	// Group project roles by project
	projectRoles := groupRolesByProject(envs)

	return map[string]interface{}{
		"environments":  envs,
		"totalRepos":    len(repoMap),
		"duplicates":    duplicates,
		"uniqueByEnv":   uniqueByEnv,
		"totalEnvs":     len(envs),
		"projectRepos":  projectRepos,
		"projectRoles":  projectRoles,
	}
}

func groupRepositoriesByProject(envs []EnvironmentData) map[string][]RepoRecord {
	projectRepos := make(map[string][]RepoRecord)

	for _, env := range envs {
		for _, repo := range env.Repos {
			if repo.ProjectKey != "" {
				projectRepos[repo.ProjectKey] = append(projectRepos[repo.ProjectKey], repo)
			}
		}
	}

	// Sort repositories within each project
	for _, repos := range projectRepos {
		sort.Slice(repos, func(i, j int) bool {
			return repos[i].Name < repos[j].Name
		})
	}

	return projectRepos
}

func groupRolesByProject(envs []EnvironmentData) map[string][]ProjectRoleData {
	projectRoles := make(map[string][]ProjectRoleData)

	for _, env := range envs {
		if env.Instance == nil || len(env.Instance.Projects) == 0 {
			continue
		}

		for _, proj := range env.Instance.Projects {
			if proj.Roles != nil {
				projectRoles[proj.Key] = append(projectRoles[proj.Key], proj.Roles...)
			}
		}
	}

	return projectRoles
}

func writeReport(filePath string, report map[string]interface{}) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	envs := report["environments"].([]EnvironmentData)
	duplicates := report["duplicates"].([]*DuplicateRepo)
	uniqueByEnv := report["uniqueByEnv"].(map[string]int)

	fmt.Fprintf(file, "===============================================\n")
	fmt.Fprintf(file, "CROSS-ENVIRONMENT DISCOVERY ANALYSIS REPORT\n")
	fmt.Fprintf(file, "Generated: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(file, "===============================================\n\n")

	// Summary
	fmt.Fprintf(file, "SUMMARY\n")
	fmt.Fprintf(file, "-------\n")
	fmt.Fprintf(file, "Total Environments: %d\n", len(envs))
	fmt.Fprintf(file, "Total Unique Repos Across All Environments: %d\n", report["totalRepos"])
	fmt.Fprintf(file, "Repos Found in Multiple Environments: %d\n", len(duplicates))
	fmt.Fprintf(file, "\n")

	// Environment breakdown with instance info
	fmt.Fprintf(file, "ENVIRONMENT BREAKDOWN\n")
	fmt.Fprintf(file, "--------------------\n")
	for _, env := range envs {
		uniqueCount := len(env.Repos)
		duplicateCount := uniqueByEnv[env.Name]
		fmt.Fprintf(file, "  %s:\n", env.Name)
		fmt.Fprintf(file, "    - Total Repos: %d\n", uniqueCount)
		fmt.Fprintf(file, "    - In Multiple Environments: %d\n", duplicateCount)

		if env.Instance != nil {
			fmt.Fprintf(file, "    - Users: %d\n", len(env.Instance.Users))
			fmt.Fprintf(file, "    - Groups: %d\n", len(env.Instance.Groups))
			fmt.Fprintf(file, "    - Projects: %d\n", len(env.Instance.Projects))
		}
		fmt.Fprintf(file, "\n")
	}

	// Detailed duplicates
	fmt.Fprintf(file, "\n")
	fmt.Fprintf(file, "DUPLICATE REPOSITORIES (Found in Multiple Environments)\n")
	fmt.Fprintf(file, "======================================================\n\n")

	if len(duplicates) == 0 {
		fmt.Fprintf(file, "No duplicate repositories found.\n")
	} else {
		for i, dup := range duplicates {
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

	// Project Repositories
	projectRepos := report["projectRepos"].(map[string][]RepoRecord)
	if len(projectRepos) > 0 {
		fmt.Fprintf(file, "\n")
		fmt.Fprintf(file, "PROJECT REPOSITORIES (Grouped by Project)\n")
		fmt.Fprintf(file, "=======================================\n\n")

		// Sort project keys
		var projectKeys []string
		for pk := range projectRepos {
			projectKeys = append(projectKeys, pk)
		}
		sort.Strings(projectKeys)

		for _, projectKey := range projectKeys {
			repos := projectRepos[projectKey]
			fmt.Fprintf(file, "[%s] (%d repositories)\n", projectKey, len(repos))
			for _, repo := range repos {
				fmt.Fprintf(file, "  - %s\n", repo.Name)
				fmt.Fprintf(file, "    Type: %s / %s\n", repo.PackageType, repo.RepoType)
				fmt.Fprintf(file, "    Size: %s files | %s GB\n", repo.FilesCount, repo.UsageGB)
			}
			fmt.Fprintf(file, "\n")
		}
	}

	// Project Roles
	projectRoles := report["projectRoles"].(map[string][]ProjectRoleData)
	if len(projectRoles) > 0 {
		fmt.Fprintf(file, "\n")
		fmt.Fprintf(file, "PROJECT ROLES (Grouped by Project)\n")
		fmt.Fprintf(file, "=================================\n\n")

		// Sort project keys
		var projectKeys []string
		for pk := range projectRoles {
			projectKeys = append(projectKeys, pk)
		}
		sort.Strings(projectKeys)

		for _, projectKey := range projectKeys {
			roles := projectRoles[projectKey]
			fmt.Fprintf(file, "[%s] (%d roles)\n", projectKey, len(roles))
			for _, role := range roles {
				fmt.Fprintf(file, "  - %s\n", role.Name)
				fmt.Fprintf(file, "    Type: %s\n", role.Type)
				if len(role.Environments) > 0 {
					fmt.Fprintf(file, "    Environments: %s\n", strings.Join(role.Environments, ", "))
				}
				if len(role.Actions) > 0 {
					fmt.Fprintf(file, "    Actions: %s\n", strings.Join(role.Actions, ", "))
				}
			}
			fmt.Fprintf(file, "\n")
		}
	}

	// Generate JSON version for programmatic use
	jsonReport := map[string]interface{}{
		"generated_at": time.Now().Format(time.RFC3339),
		"summary": map[string]interface{}{
			"total_environments":    len(envs),
			"total_unique_repos":    report["totalRepos"],
			"duplicate_count":       len(duplicates),
		},
		"environments":     generateEnvSummary(envs),
		"duplicates":       duplicates,
		"project_repos":    projectRepos,
		"project_roles":    projectRoles,
	}

	jsonPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".json"
	jsonFile, err := os.Create(jsonPath)
	if err != nil {
		return err
	}
	defer jsonFile.Close()

	encoder := json.NewEncoder(jsonFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(jsonReport); err != nil {
		return err
	}

	return nil
}

func generateEnvSummary(envs []EnvironmentData) []map[string]interface{} {
	var summary []map[string]interface{}
	for _, env := range envs {
		s := map[string]interface{}{
			"name":       env.Name,
			"repo_count": len(env.Repos),
		}
		if env.Instance != nil {
			s["user_count"] = len(env.Instance.Users)
			s["group_count"] = len(env.Instance.Groups)
			s["project_count"] = len(env.Instance.Projects)
		}
		summary = append(summary, s)
	}
	return summary
}

func printReportSummary(report map[string]interface{}) {
	fmt.Printf("\n=== REPORT SUMMARY ===\n")
	fmt.Printf("Total Environments: %d\n", report["totalEnvs"])
	fmt.Printf("Total Unique Repos: %d\n", report["totalRepos"])
	fmt.Printf("Duplicate Repos Found: %d\n", len(report["duplicates"].([]*DuplicateRepo)))
}

func getEnvNames(envs []EnvironmentData) []string {
	var names []string
	for _, env := range envs {
		names = append(names, env.Name)
	}
	return names
}
