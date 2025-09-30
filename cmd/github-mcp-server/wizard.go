package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/github/github-mcp-server/pkg/github"
	"github.com/github/github-mcp-server/pkg/raw"
	gogithub "github.com/google/go-github/v74/github"
	"github.com/shurcooL/githubv4"
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

type toolsetInfo struct {
	name        string
	description string
	tools       []toolInfo
}

// getAvailableToolsets dynamically extracts toolsets and their tools from the server
func getAvailableToolsets() []toolsetInfo {
	// Create dummy client functions that return nil
	// These are safe because we're only extracting metadata, not executing the tools
	getClient := func(ctx context.Context) (*gogithub.Client, error) {
		return nil, nil
	}
	getGQLClient := func(ctx context.Context) (*githubv4.Client, error) {
		return nil, nil
	}
	getRawClient := func(ctx context.Context) (*raw.Client, error) {
		return nil, nil
	}
	translator := func(key string, defaultValue string) string {
		return defaultValue
	}
	
	// Create a dummy toolset group to extract the structure
	// We use read-only false to get all tools
	tsg := github.DefaultToolsetGroup(false, getClient, getGQLClient, getRawClient, translator, 5000)
	
	var toolsetList []toolsetInfo
	
	for toolsetName, toolset := range tsg.Toolsets {
		ts := toolsetInfo{
			name:        toolsetName,
			description: toolset.Description,
			tools:       []toolInfo{},
		}
		
		// Get all available tools (both read and write)
		allTools := toolset.GetAvailableTools()
		for _, tool := range allTools {
			ts.tools = append(ts.tools, toolInfo{
				name:        tool.Tool.Name,
				description: tool.Tool.Description,
				toolsetName: toolsetName,
				isReadOnly:  tool.Tool.Annotations.ReadOnlyHint != nil && *tool.Tool.Annotations.ReadOnlyHint,
			})
		}
		
		// Sort tools by name
		sort.Slice(ts.tools, func(i, j int) bool {
			return ts.tools[i].name < ts.tools[j].name
		})
		
		toolsetList = append(toolsetList, ts)
	}
	
	// Sort toolsets by name
	sort.Slice(toolsetList, func(i, j int) bool {
		return toolsetList[i].name < toolsetList[j].name
	})
	
	return toolsetList
}

// getFirstSentence extracts the first sentence from a description
func getFirstSentence(description string) string {
    // Find the first period followed by a space or end of string
    if idx := strings.Index(description, ". "); idx != -1 {
        return description[:idx+1]
    }
    // If no ". " found, check if it ends with a period
    if strings.HasSuffix(description, ".") {
        return description
    }
    // If no period at all, return as is
    return description
}


func runWizard(cmd *cobra.Command, args []string) error {
    fmt.Println("🧙 GitHub MCP Server Configuration Wizard")
    fmt.Println("==========================================")
    fmt.Println()

    // Dynamically get available toolsets
    toolsets := getAvailableToolsets()
    
    // Flatten all tools into a single list
    var allToolOptions []string
    toolMap := make(map[string]toolInfo)
    
    for _, ts := range toolsets {
        for _, tool := range ts.tools {
            // Use only the first sentence of the description
            shortDesc := getFirstSentence(tool.description)
            option := fmt.Sprintf("%s%s (from %s)", tool.name, shortDesc, tool.toolsetName)
            allToolOptions = append(allToolOptions, option)
            toolMap[option] = tool
        }
    }
    
    // Present a simple multi-select list
    var selectedOptions []string
    toolPrompt := &survey.MultiSelect{
        Message:  "Select the tools you want to enable (type to filter, use arrows/space to select):",
        Options:  allToolOptions,
        PageSize: 15,
        VimMode:  true,
    }
    
    if err := survey.AskOne(toolPrompt, &selectedOptions); err != nil {
        return err
    }
    
    // Extract selected tool names
    var selectedTools []string
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