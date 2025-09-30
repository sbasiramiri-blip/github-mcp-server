package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
)

var wizardCmd = &cobra.Command{
	Use:   "wizard",
	Short: "Interactive configuration wizard for GitHub MCP Server",
	Long:  `This wizard will help you configure which specific tools to enable.`,
	RunE:  runWizard,
}

type toolInfo struct {
	name        string
	description string
	toolsetName string
	isReadOnly  bool
}

// Define all available tools statically to avoid runtime initialization issues
var availableTools = map[string][]toolInfo{
	"repos": {
		{name: "search_repositories", description: "Search for GitHub repositories", isReadOnly: true},
		{name: "get_file_contents", description: "Get file contents from a repository", isReadOnly: true},
		{name: "list_commits", description: "List commits in a repository", isReadOnly: true},
		{name: "search_code", description: "Search code across GitHub", isReadOnly: true},
		{name: "get_commit", description: "Get details of a specific commit", isReadOnly: true},
		{name: "list_branches", description: "List branches in a repository", isReadOnly: true},
		{name: "list_tags", description: "List tags in a repository", isReadOnly: true},
		{name: "get_tag", description: "Get details of a specific tag", isReadOnly: true},
		{name: "list_releases", description: "List releases in a repository", isReadOnly: true},
		{name: "get_latest_release", description: "Get the latest release of a repository", isReadOnly: true},
		{name: "get_release_by_tag", description: "Get a release by its tag", isReadOnly: true},
		{name: "list_starred_repositories", description: "List starred repositories", isReadOnly: true},
		{name: "create_or_update_file", description: "Create or update a file in a repository", isReadOnly: false},
		{name: "create_repository", description: "Create a new repository", isReadOnly: false},
		{name: "fork_repository", description: "Fork a repository", isReadOnly: false},
		{name: "create_branch", description: "Create a new branch", isReadOnly: false},
		{name: "push_files", description: "Push files to a repository", isReadOnly: false},
		{name: "delete_file", description: "Delete a file from a repository", isReadOnly: false},
		{name: "star_repository", description: "Star a repository", isReadOnly: false},
		{name: "unstar_repository", description: "Unstar a repository", isReadOnly: false},
	},
	"issues": {
		{name: "get_issue", description: "Get details of an issue", isReadOnly: true},
		{name: "search_issues", description: "Search for issues", isReadOnly: true},
		{name: "list_issues", description: "List issues in a repository", isReadOnly: true},
		{name: "get_issue_comments", description: "Get comments on an issue", isReadOnly: true},
		{name: "list_issue_types", description: "List available issue types", isReadOnly: true},
		{name: "list_sub_issues", description: "List sub-issues of an issue", isReadOnly: true},
		{name: "create_issue", description: "Create a new issue", isReadOnly: false},
		{name: "add_issue_comment", description: "Add a comment to an issue", isReadOnly: false},
		{name: "update_issue", description: "Update an issue", isReadOnly: false},
		{name: "assign_copilot_to_issue", description: "Assign Copilot to an issue", isReadOnly: false},
		{name: "add_sub_issue", description: "Add a sub-issue", isReadOnly: false},
		{name: "remove_sub_issue", description: "Remove a sub-issue", isReadOnly: false},
		{name: "reprioritize_sub_issue", description: "Reprioritize a sub-issue", isReadOnly: false},
	},
	"users": {
		{name: "search_users", description: "Search for GitHub users", isReadOnly: true},
	},
	"orgs": {
		{name: "search_orgs", description: "Search for GitHub organizations", isReadOnly: true},
	},
	"pull_requests": {
		{name: "get_pull_request", description: "Get details of a pull request", isReadOnly: true},
		{name: "list_pull_requests", description: "List pull requests in a repository", isReadOnly: true},
		{name: "get_pull_request_files", description: "Get files changed in a pull request", isReadOnly: true},
		{name: "search_pull_requests", description: "Search for pull requests", isReadOnly: true},
		{name: "get_pull_request_status", description: "Get status of a pull request", isReadOnly: true},
		{name: "get_pull_request_review_comments", description: "Get review comments on a pull request", isReadOnly: true},
		{name: "get_pull_request_reviews", description: "Get reviews on a pull request", isReadOnly: true},
		{name: "get_pull_request_diff", description: "Get diff of a pull request", isReadOnly: true},
		{name: "merge_pull_request", description: "Merge a pull request", isReadOnly: false},
		{name: "update_pull_request_branch", description: "Update pull request branch", isReadOnly: false},
		{name: "create_pull_request", description: "Create a pull request", isReadOnly: false},
		{name: "update_pull_request", description: "Update a pull request", isReadOnly: false},
		{name: "request_copilot_review", description: "Request Copilot review", isReadOnly: false},
		{name: "create_and_submit_pull_request_review", description: "Create and submit a PR review", isReadOnly: false},
		{name: "create_pending_pull_request_review", description: "Create a pending PR review", isReadOnly: false},
		{name: "add_comment_to_pending_review", description: "Add comment to pending review", isReadOnly: false},
		{name: "submit_pending_pull_request_review", description: "Submit a pending PR review", isReadOnly: false},
		{name: "delete_pending_pull_request_review", description: "Delete a pending PR review", isReadOnly: false},
	},
	"code_security": {
		{name: "get_code_scanning_alert", description: "Get a code scanning alert", isReadOnly: true},
		{name: "list_code_scanning_alerts", description: "List code scanning alerts", isReadOnly: true},
	},
	"secret_protection": {
		{name: "get_secret_scanning_alert", description: "Get a secret scanning alert", isReadOnly: true},
		{name: "list_secret_scanning_alerts", description: "List secret scanning alerts", isReadOnly: true},
	},
	"dependabot": {
		{name: "get_dependabot_alert", description: "Get a Dependabot alert", isReadOnly: true},
		{name: "list_dependabot_alerts", description: "List Dependabot alerts", isReadOnly: true},
	},
	"notifications": {
		{name: "list_notifications", description: "List notifications", isReadOnly: true},
		{name: "get_notification_details", description: "Get notification details", isReadOnly: true},
		{name: "dismiss_notification", description: "Dismiss a notification", isReadOnly: false},
		{name: "mark_all_notifications_read", description: "Mark all notifications as read", isReadOnly: false},
		{name: "manage_notification_subscription", description: "Manage notification subscription", isReadOnly: false},
		{name: "manage_repository_notification_subscription", description: "Manage repository notification subscription", isReadOnly: false},
	},
	"discussions": {
		{name: "list_discussions", description: "List discussions", isReadOnly: true},
		{name: "get_discussion", description: "Get a discussion", isReadOnly: true},
		{name: "get_discussion_comments", description: "Get discussion comments", isReadOnly: true},
		{name: "list_discussion_categories", description: "List discussion categories", isReadOnly: true},
	},
	"actions": {
		{name: "list_workflows", description: "List workflows", isReadOnly: true},
		{name: "list_workflow_runs", description: "List workflow runs", isReadOnly: true},
		{name: "get_workflow_run", description: "Get a workflow run", isReadOnly: true},
		{name: "get_workflow_run_logs", description: "Get workflow run logs", isReadOnly: true},
		{name: "list_workflow_jobs", description: "List workflow jobs", isReadOnly: true},
		{name: "get_job_logs", description: "Get job logs", isReadOnly: true},
		{name: "list_workflow_run_artifacts", description: "List workflow run artifacts", isReadOnly: true},
		{name: "download_workflow_run_artifact", description: "Download workflow run artifact", isReadOnly: true},
		{name: "get_workflow_run_usage", description: "Get workflow run usage", isReadOnly: true},
		{name: "run_workflow", description: "Run a workflow", isReadOnly: false},
		{name: "rerun_workflow_run", description: "Rerun a workflow run", isReadOnly: false},
		{name: "rerun_failed_jobs", description: "Rerun failed jobs", isReadOnly: false},
		{name: "cancel_workflow_run", description: "Cancel a workflow run", isReadOnly: false},
		{name: "delete_workflow_run_logs", description: "Delete workflow run logs", isReadOnly: false},
	},
	"security_advisories": {
		{name: "list_global_security_advisories", description: "List global security advisories", isReadOnly: true},
		{name: "get_global_security_advisory", description: "Get a global security advisory", isReadOnly: true},
		{name: "list_repository_security_advisories", description: "List repository security advisories", isReadOnly: true},
		{name: "list_org_repository_security_advisories", description: "List org repository security advisories", isReadOnly: true},
	},
	"context": {
		{name: "get_me", description: "Get current user information", isReadOnly: true},
		{name: "get_teams", description: "Get teams", isReadOnly: true},
		{name: "get_team_members", description: "Get team members", isReadOnly: true},
	},
	"gists": {
		{name: "list_gists", description: "List gists", isReadOnly: true},
		{name: "create_gist", description: "Create a gist", isReadOnly: false},
		{name: "update_gist", description: "Update a gist", isReadOnly: false},
	},
	"projects": {
		{name: "list_projects", description: "List projects", isReadOnly: true},
		{name: "get_project", description: "Get a project", isReadOnly: true},
		{name: "list_project_fields", description: "List project fields", isReadOnly: true},
		{name: "get_project_field", description: "Get a project field", isReadOnly: true},
		{name: "list_project_items", description: "List project items", isReadOnly: true},
		{name: "get_project_item", description: "Get a project item", isReadOnly: true},
		{name: "add_project_item", description: "Add a project item", isReadOnly: false},
		{name: "delete_project_item", description: "Delete a project item", isReadOnly: false},
		{name: "update_project_item", description: "Update a project item", isReadOnly: false},
	},
}

func runWizard(cmd *cobra.Command, args []string) error {
	fmt.Println("🧙 GitHub MCP Server Configuration Wizard")
	fmt.Println("==========================================")
	fmt.Println()

	// Collect all available tools
	var allTools []toolInfo
	for toolsetName, tools := range availableTools {
		for _, tool := range tools {
			tool.toolsetName = toolsetName
			allTools = append(allTools, tool)
		}
	}

	// Sort tools alphabetically by name
	sort.Slice(allTools, func(i, j int) bool {
		return allTools[i].name < allTools[j].name
	})

	// Create options for the survey with formatted descriptions
	var toolOptions []string
	toolMap := make(map[string]toolInfo)
	for _, tool := range allTools {
		readOnlyTag := ""
		if tool.isReadOnly {
			readOnlyTag = " [READ-ONLY]"
		}
		option := fmt.Sprintf("%s%s - %s (from %s)", tool.name, readOnlyTag, tool.description, tool.toolsetName)
		toolOptions = append(toolOptions, option)
		toolMap[option] = tool
	}

	// Select individual tools
	selectedOptions := []string{}
	toolPrompt := &survey.MultiSelect{
		Message:  "Select the specific tools you want to enable:",
		Options:  toolOptions,
		PageSize: 20,
	}

	if err := survey.AskOne(toolPrompt, &selectedOptions); err != nil {
		return err
	}

	// Parse selected tools
	selectedTools := []string{}
	for _, selection := range selectedOptions {
		if tool, exists := toolMap[selection]; exists {
			selectedTools = append(selectedTools, tool.name)
		}
	}

	// Build args array
	cmdArgs := []string{
		"run",
		"cmd/github-mcp-server/main.go",
		"cmd/github-mcp-server/wizard.go",
		"stdio",
	}

	// Add specific tools
	if len(selectedTools) > 0 {
		cmdArgs = append(cmdArgs, "--tools")
		cmdArgs = append(cmdArgs, strings.Join(selectedTools, ","))
	}

	// Display the command
	fmt.Println()
	fmt.Println("✅ Configuration Complete!")
	fmt.Println("==========================")
	fmt.Println()
	fmt.Println("Selected tools:", strings.Join(selectedTools, ", "))
	fmt.Println()
	fmt.Println("Add this to your mcp.json configuration:")
	fmt.Println()

	// Print JSON format
	fmt.Println(`"args": [`)
	for i, arg := range cmdArgs {
		comma := ","
		if i == len(cmdArgs)-1 {
			comma = ""
		}
		fmt.Printf(`    "%s"%s`+"\n", arg, comma)
	}
	fmt.Println(`],`)

	fmt.Println()
	fmt.Println("Or run directly with:")
	fmt.Printf("go %s\n", strings.Join(cmdArgs, " "))

	return nil
}