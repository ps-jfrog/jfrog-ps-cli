# Cross-Environment Discovery Analysis Tool

This tool analyzes multiple discovery outputs from different JFrog environments and generates a unified report highlighting duplicates and inconsistencies across environments.

## Overview

The tool reads discovery output files (`report.csv` and `instance.json`) from multiple environments and:

1. **Identifies Duplicate Repositories** - Detects repos with the same name across multiple environments
2. **Compares Environment Configurations** - Shows instance-level stats (users, groups, projects, storage)
3. **Generates Unified Report** - Produces both human-readable text and machine-readable JSON output
4. **Flags Issues** - Highlights repositories that appear in multiple environments

## Usage

Build the tool:
```bash
go build -o analyze-discovery ./cmd/analyze-discovery
```

Run against discovery outputs:
```bash
# From the project root
./cmd/analyze-discovery -output output -report output/cross-env-analysis.txt

# Or from a built binary
analyze-discovery -output ./output -report ./cross-env-analysis.txt
```

### Flags

- `-output` (default: `output`) - Path to the directory containing discovery output subdirectories
- `-report` (default: `cross-env-analysis.txt`) - Output file path for the text report

## Output Files

The tool generates two files:

### 1. Text Report (`cross-env-analysis.txt`)

Human-readable format containing:
- Summary statistics across all environments
- Per-environment breakdown (repo count, user count, group count, project count)
- Detailed list of duplicate repositories with their configurations

Example excerpt:
```
[1] shared-artifacts
    ⚠️  DUPLICATE FOUND IN: psazuse, staging
    Details:
      psazuse:
        Type: Generic / LOCAL
        Project: shared
        Size: 1500 files | 25.5 GB
        Created: 2024-01-15T10:30:00Z by admin
        Last Activity: 2025-09-20T14:22:15Z
        Policies: daily-cleanup | weekly-archive
      
      staging:
        Type: Generic / LOCAL
        Project: shared
        Size: 1200 files | 22.3 GB
        ...
```

### 2. JSON Report (`cross-env-analysis.json`)

Machine-readable format containing:
- Generation timestamp
- Summary statistics
- Environment-level metrics
- Complete duplicate repository list with all details

Can be used for:
- Automated analysis and alerting
- Integration with other tools
- Data pipeline processing
- Dashboard visualization

## Discovery Output Structure

The tool expects discovery outputs in this structure:
```
output/
├── psazuse/
│   ├── report.csv          (repos data)
│   ├── policies.csv        (cleanup policies)
│   ├── instance.json       (instance-wide config)
│   └── audit.html          (interactive audit)
├── staging/
│   ├── report.csv
│   ├── policies.csv
│   ├── instance.json
│   └── audit.html
└── default/
    └── ...
```

Each environment directory can contain:
- `report.csv` - Repository inventory (required for repo analysis)
- `instance.json` - Instance configuration (optional, for detailed metrics)

If an environment has no `report.csv`, it's skipped.

## Duplicate Detection

Duplicates are identified by **exact repository name match** across environments.

When duplicates are found, the report includes:
- Repository name
- List of environments where it appears
- Complete details from each environment:
  - Package type and repo type
  - Project assignment
  - File count and storage usage
  - Creation date and creator
  - Last activity date
  - Associated cleanup policies

## Example: Multi-Environment Analysis

Running with 3 environments:
```
psazuse/   - Production Artifactory
staging/   - Staging Artifactory  
dev/       - Development Artifactory
```

Output report might show:
```
Total Environments: 3
Total Unique Repos Across All Environments: 4500
Repos Found in Multiple Environments: 47
```

This indicates 47 repositories that exist in more than one environment, which may indicate:
- Intentional replication/federation setup
- Incomplete migrations
- Configuration drift that needs reconciliation
- Backup/disaster recovery repos

## Use Cases

### Configuration Audits
Find repositories with the same name but different configurations across environments to identify:
- Policy mismatches
- Access control differences
- Storage size discrepancies
- Project assignment variations

### Compliance & Governance
Identify duplicate repos that might violate organizational policies for:
- Repository naming conventions
- Cleanup policy consistency
- Data residency requirements

### Migration Planning
Use duplicate information to plan:
- Repository consolidation
- Federation setup validation
- Repository replication strategies
- Cleanup of obsolete duplicates

### Disaster Recovery
Validate backup/DR strategies by reviewing:
- Which repos are replicated
- Consistency of backup configurations
- Coverage across environments

## Implementation Details

The tool is written in Go and uses:
- CSV parsing for `report.csv` analysis
- JSON unmarshaling for `instance.json` parsing
- Concurrent environment discovery
- Efficient duplicate detection using maps

## Performance

The tool:
- Scans all environments in parallel (I/O bound)
- Generates unified report in O(n) time complexity
- Uses minimal memory footprint suitable for large inventories (1000s of repos)
- Completes analysis typically within seconds

## Example Output

```json
{
  "generated_at": "2026-09-21T11:13:47-05:00",
  "summary": {
    "total_environments": 2,
    "total_unique_repos": 5000,
    "duplicate_count": 150
  },
  "environments": [
    {
      "name": "production",
      "repo_count": 3200,
      "user_count": 500,
      "group_count": 200,
      "project_count": 150
    },
    {
      "name": "staging",
      "repo_count": 2850,
      "user_count": 350,
      "group_count": 150,
      "project_count": 120
    }
  ],
  "duplicates": [
    {
      "name": "shared-libs",
      "environments": ["production", "staging"],
      "repos": {
        "production": { ... },
        "staging": { ... }
      }
    }
  ]
}
```
