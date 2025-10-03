package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/github/github-mcp-server/pkg/github"
	"github.com/github/github-mcp-server/pkg/raw"
	gogithub "github.com/google/go-github/v74/github"
	"github.com/mark3labs/mcp-go/server"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
	"github.com/tiktoken-go/tokenizer"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Interactive configuration tool for GitHub MCP Server",
	Long:  `This interactive tool will help you configure which specific tools to enable.`,
	RunE:  runConfigure,
}

// Styles for the configuration UI
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			MarginBottom(1).
			MarginTop(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			MarginBottom(1)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#7D56F4")).
				Bold(true).
				PaddingLeft(1).
				PaddingRight(1)

	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			PaddingLeft(1)

	selectedCheckStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00FF00")).
				Bold(true)

	unselectedCheckStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888"))

	categoryStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFA500")).
			Bold(true).
			MarginTop(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			MarginTop(1)

	filterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))

	toolsetBadgeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#84a5ecff"))

	tokenBadgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e8b75aff"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00")).
			Bold(true)


	asciiArtStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)


	accentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D7FF")).
			Bold(true)
)

const asciiArt = `
   _____ _ _   _    _       _      __  __  _____ _____    _____                          
  / ____(_) | | |  | |     | |    |  \/  |/ ____|  __ \  / ____|                         
 | |  __ _| |_| |__| |_   _| |__  | \  / | |    | |__) || (___   ___ _ ____   _____ _ __ 
 | | |_ | | __|  __  | | | | '_ \ | |\/| | |    |  ___/  \___ \ / _ \ '__\ \ / / _ \ '__|
 | |__| | | |_| |  | | |_| | |_)  | |  | | |____| |      ____) |  __/ |   \ V /  __/ |   
  \_____|_|\__|_|  |_|\__,_|_.__/ |_|  |_|\_____|_|     |_____/ \___|_|    \_/ \___|_|   
                                                                                         `

type toolInfo struct {
	name        string
	description string
	toolsetName string
	isReadOnly  bool
	tokenCount  int    // Estimated token count for this tool's definition
}

type toolsetInfo struct {
	name        string
	description string
	tools       []toolInfo
}

type configureModel struct {
	toolsets       []toolsetInfo
	allTools       []toolInfo
	filteredTools  []toolInfo
	cursor         int
	selected       map[int]bool
	filter         string
	filterActive   bool
	width          int
	height         int
	quitting       bool
	confirmed      bool
	viewportOffset int
	showWelcome    bool
	encoder        tokenizer.Codec // Tokenizer encoder for counting tokens
}

func initialConfigureModel(toolsets []toolsetInfo) configureModel {
	// Initialize tokenizer
	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		// If tokenizer fails to initialize, continue without it
		enc = nil
	}

	// Flatten all tools
	var allTools []toolInfo
	for _, ts := range toolsets {
		allTools = append(allTools, ts.tools...)
	}

	// Sort all tools alphabetically by name
	sort.Slice(allTools, func(i, j int) bool {
		return allTools[i].name < allTools[j].name
	})

	return configureModel{
		toolsets:      toolsets,
		allTools:      allTools,
		filteredTools: allTools,
		selected:      make(map[int]bool),
		width:         80,
		height:        24,
		showWelcome:   true, // Start with welcome screen
		encoder:       enc,
	}
}

func (m configureModel) Init() tea.Cmd {
	return nil
}

func (m configureModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Welcome screen - only Enter or Space continues
		if m.showWelcome {
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				m.quitting = true
				return m, tea.Quit
			case "enter", " ":
				m.showWelcome = false
				return m, nil
			}
			// Ignore all other keys on welcome screen
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			if m.filterActive {
				// Exit filter mode
				m.filterActive = false
				m.filter = ""
				m.filteredTools = m.allTools
				m.cursor = 0
				m.viewportOffset = 0
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit

		case "enter":
			if m.filterActive {
				// Exit filter mode
				m.filterActive = false
				return m, nil
			}
			// Confirm selection
			m.confirmed = true
			return m, tea.Quit

		case "/":
			if !m.filterActive {
				m.filterActive = true
				m.filter = ""
				return m, nil
			}

		case "backspace":
			if m.filterActive && len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.applyFilter()
				return m, nil
			}

		case "esc":
			if m.filterActive {
				m.filterActive = false
				m.filter = ""
				m.filteredTools = m.allTools
				m.cursor = 0
				m.viewportOffset = 0
				return m, nil
			}

		case "up", "k":
			if !m.filterActive && m.cursor > 0 {
				m.cursor--
				m.adjustViewport()
			}

		case "down", "j":
			if !m.filterActive && m.cursor < len(m.filteredTools)-1 {
				m.cursor++
				m.adjustViewport()
			}

		case "g":
			if !m.filterActive {
				m.cursor = 0
				m.viewportOffset = 0
			}

		case "G":
			if !m.filterActive {
				m.cursor = len(m.filteredTools) - 1
				m.adjustViewport()
			}

		case " ", "x":
			if !m.filterActive && len(m.filteredTools) > 0 {
				// Toggle selection
				m.selected[m.cursor] = !m.selected[m.cursor]
			}

		case "a":
			if !m.filterActive {
				// Select all filtered
				for i := range m.filteredTools {
					m.selected[i] = true
				}
			}

		case "n":
			if !m.filterActive {
				// Deselect all
				m.selected = make(map[int]bool)
			}

		default:
			if m.filterActive && len(msg.String()) == 1 {
				m.filter += msg.String()
				m.applyFilter()
			}
		}
	}

	return m, nil
}

func (m *configureModel) applyFilter() {
	if m.filter == "" {
		m.filteredTools = m.allTools
	} else {
		m.filteredTools = []toolInfo{}
		filterLower := strings.ToLower(m.filter)
		for _, tool := range m.allTools {
			if strings.Contains(strings.ToLower(tool.name), filterLower) ||
				strings.Contains(strings.ToLower(tool.description), filterLower) ||
				strings.Contains(strings.ToLower(tool.toolsetName), filterLower) {
				m.filteredTools = append(m.filteredTools, tool)
			}
		}
	}
	m.cursor = 0
	m.viewportOffset = 0
}

func (m *configureModel) adjustViewport() {
	maxVisible := m.height - 10 // Reserve space for header and footer
	if maxVisible < 1 {
		maxVisible = 10
	}

	if m.cursor < m.viewportOffset {
		m.viewportOffset = m.cursor
	} else if m.cursor >= m.viewportOffset+maxVisible {
		m.viewportOffset = m.cursor - maxVisible + 1
	}
}

func (m configureModel) View() string {
	if m.quitting && !m.confirmed {
		return dimStyle.Render("\nConfiguration cancelled.\n")
	}

	if m.confirmed {
		// Show a brief "saving" message before exiting
		return successStyle.Render("\n✓ Generating configuration...\n")
	}

	// Show welcome screen first
	if m.showWelcome {
		return m.renderWelcome()
	}

	var s strings.Builder

	// Header
	s.WriteString(titleStyle.Render("🔧 GitHub MCP Server Configuration Tool"))
	s.WriteString("\n")
	s.WriteString(subtitleStyle.Render("Select the tools you want to enable for your MCP server"))
	s.WriteString("\n")

	// Filter bar
	if m.filterActive {
		s.WriteString(filterStyle.Render("Filter: ") + m.filter + "█")
		s.WriteString("\n")
	} else if m.filter != "" {
		s.WriteString(filterStyle.Render(fmt.Sprintf("Filtered: %d/%d tools", len(m.filteredTools), len(m.allTools))))
		s.WriteString("\n")
	}
	s.WriteString("\n")

	// Tool list
	maxVisible := m.height - 10
	if maxVisible < 1 {
		maxVisible = 10
	}

	visibleStart := m.viewportOffset
	visibleEnd := visibleStart + maxVisible
	if visibleEnd > len(m.filteredTools) {
		visibleEnd = len(m.filteredTools)
	}

	// Calculate selected count and total tokens
	selectedCount := 0
	totalTokens := 0
	for i, selected := range m.selected {
		if selected {
			selectedCount++
			if i < len(m.filteredTools) {
				totalTokens += m.filteredTools[i].tokenCount
			}
		}
	}

	if len(m.filteredTools) == 0 {
		s.WriteString(dimStyle.Render("  No tools match your filter\n"))
	} else {
		for i := visibleStart; i < visibleEnd; i++ {
			tool := m.filteredTools[i]

			// Check if selected
			checkbox := "[ ]"
			checkStyle := unselectedCheckStyle
			if m.selected[i] {
				checkbox = "[✓]"
				checkStyle = selectedCheckStyle
			}

			// Render cursor
			cursor := "  "
			nameStyle := itemStyle
			if i == m.cursor && !m.filterActive {
				cursor = "▸ "
				nameStyle = selectedItemStyle
			}

			// Format the line
			line := fmt.Sprintf("%s%s %s ",
				cursor,
				checkStyle.Render(checkbox),
				nameStyle.Render(tool.name),
			)

			// Add category badge
			category := toolsetBadgeStyle.Render(fmt.Sprintf("[%s]", tool.toolsetName))
			line += category

			// Add token count if available
			if tool.tokenCount > 0 {
				tokenBadge := tokenBadgeStyle.Render(fmt.Sprintf(" ~%s tokens", formatTokenCount(tool.tokenCount)))
				line += tokenBadge
			}

			// Add description (truncated if needed)
			desc := getFirstSentence(tool.description)
			maxDescLen := 45 // Reduced to make room for token count
			if len(desc) > maxDescLen {
				desc = desc[:maxDescLen-3] + "..."
			}
			line += " " + dimStyle.Render(desc)

			s.WriteString(line)
			s.WriteString("\n")
		}

		// Scroll indicator
		if len(m.filteredTools) > maxVisible {
			scrollInfo := fmt.Sprintf("  (showing %d-%d of %d)",
				visibleStart+1, visibleEnd, len(m.filteredTools))
			s.WriteString(dimStyle.Render(scrollInfo))
			s.WriteString("\n")
		}
	}

	s.WriteString("\n")

	// Footer with help
	footerInfo := fmt.Sprintf("Selected: %d tools", selectedCount)
	if m.encoder != nil && totalTokens > 0 {
		footerInfo += fmt.Sprintf(" • Estimated tokens: ~%s", formatTokenCount(totalTokens))
	}
	s.WriteString(helpStyle.Render(footerInfo))
	s.WriteString("\n")
	s.WriteString(helpStyle.Render("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
	s.WriteString("\n")

	if m.filterActive {
		s.WriteString(helpStyle.Render("esc: exit filter • backspace: delete character • enter: apply filter"))
	} else {
		s.WriteString(helpStyle.Render("↑/↓ or j/k: navigate • space: toggle • /: filter • a: select all • n: clear all"))
		s.WriteString("\n")
		s.WriteString(helpStyle.Render("g: top • G: bottom • enter: confirm • q: quit"))
	}

	return s.String()
}

func (m configureModel) renderWelcome() string {
	var s strings.Builder

	// Center the content vertically
	totalLines := strings.Count(asciiArt, "\n") + 20 // ASCII art + text
	topPadding := (m.height - totalLines) / 2
	if topPadding < 0 {
		topPadding = 0
	}

	for i := 0; i < topPadding; i++ {
		s.WriteString("\n")
	}

	// ASCII art
	s.WriteString(asciiArtStyle.Render(asciiArt))
	s.WriteString("\n\n")

	// Welcome message
	welcomeMsg := lipgloss.NewStyle().
		Width(70).
		Align(lipgloss.Center).
		Foreground(lipgloss.Color("#FFFFFF")).
		Render("Welcome to the GitHub MCP Server Configuration Tool!")
	s.WriteString(welcomeMsg)
	s.WriteString("\n\n")

	// Description
	description := []string{
		"This interactive tool helps you customize your MCP server by selecting",
		"which tools you want to enable. You'll be able to:",
		"",
	}

	for _, line := range description {
		centered := lipgloss.NewStyle().
			Width(70).
			Align(lipgloss.Center).
			Foreground(lipgloss.Color("#888888")).
			Render(line)
		s.WriteString(centered)
		s.WriteString("\n")
	}

	// Features
	features := []string{
		accentStyle.Render("  ✓") + " Browse all available GitHub tools",
		accentStyle.Render("  ✓") + " Search and filter by name or category",
		accentStyle.Render("  ✓") + " Select only the tools you need",
		accentStyle.Render("  ✓") + " Generate ready-to-use configuration",
	}

	for _, feature := range features {
		centered := lipgloss.NewStyle().
			Width(70).
			Align(lipgloss.Center).
			Render(feature)
		s.WriteString(centered)
		s.WriteString("\n")
	}

	s.WriteString("\n\n")

	// Call to action with a pulsing effect
	ctaMain := lipgloss.NewStyle().
		Width(70).
		Align(lipgloss.Center).
		Foreground(lipgloss.Color("#7D56F4")).
		Bold(true).
		Render("Press ENTER or SPACE to continue")
	s.WriteString(ctaMain)
	s.WriteString("\n")

	ctaQuit := lipgloss.NewStyle().
		Width(70).
		Align(lipgloss.Center).
		Foreground(lipgloss.Color("#888888")).
		Render("(or press 'q' to quit)")
	s.WriteString(ctaQuit)
	s.WriteString("\n")

	return s.String()
}

func (m configureModel) renderConfirmation() string {
	var s strings.Builder

	// Get selected tools
	var selectedTools []string
	totalTokens := 0
	for i, selected := range m.selected {
		if selected && i < len(m.filteredTools) {
			selectedTools = append(selectedTools, m.filteredTools[i].name)
			totalTokens += m.filteredTools[i].tokenCount
		}
	}

	// Sort for consistent output
	sort.Strings(selectedTools)

	s.WriteString("\n")
	s.WriteString(successStyle.Render("✅ Configuration Complete!"))
	s.WriteString("\n\n")
	
	// Add token summary
	if m.encoder != nil && totalTokens > 0 {
		s.WriteString(itemStyle.Render(fmt.Sprintf("📊 Total Estimated Tokens: ~%s", formatTokenCount(totalTokens))))
		s.WriteString("\n")
		s.WriteString(dimStyle.Render("    This is an approximation"))
		s.WriteString("\n\n")
	}
	
	s.WriteString(titleStyle.Render("Selected Tools:"))

	if len(selectedTools) == 0 {
		s.WriteString(dimStyle.Render("  (none - all tools will be enabled by default)"))
		s.WriteString("\n")
	} else {
		// Group by toolset
		toolsBySet := make(map[string][]string)
		for i, selected := range m.selected {
			if selected && i < len(m.filteredTools) {
				tool := m.filteredTools[i]
				toolsBySet[tool.toolsetName] = append(toolsBySet[tool.toolsetName], tool.name)
			}
		}

		// Get sorted toolset names
		var toolsetNames []string
		for name := range toolsBySet {
			toolsetNames = append(toolsetNames, name)
		}
		sort.Strings(toolsetNames)

		for _, tsName := range toolsetNames {
			s.WriteString("\n")
			s.WriteString(categoryStyle.Render("  " + tsName + ":"))
			s.WriteString("\n")
			for _, toolName := range toolsBySet[tsName] {
				s.WriteString(itemStyle.Render("    • " + toolName))
				s.WriteString("\n")
			}
		}
	}

	// Build command args - use package path instead of individual files
	cmdArgs := []string{
		"run",
		"./cmd/github-mcp-server",
		"stdio",
	}

	if len(selectedTools) > 0 {
		cmdArgs = append(cmdArgs, "--tools")
		cmdArgs = append(cmdArgs, strings.Join(selectedTools, ","))
	}

	s.WriteString("\n")
	s.WriteString(titleStyle.Render("Configuration for mcp.json:"))
	s.WriteString("\n\n")

	// Print JSON format
	s.WriteString(itemStyle.Render(`"args": [`))
	s.WriteString("\n")
	for i, arg := range cmdArgs {
		comma := ","
		if i == len(cmdArgs)-1 {
			comma = ""
		}
		s.WriteString(itemStyle.Render(fmt.Sprintf(`    "%s"%s`, arg, comma)))
		s.WriteString("\n")
	}
	s.WriteString(itemStyle.Render(`],`))
	s.WriteString("\n\n")

	return s.String()
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
	
	// Initialize tokenizer for token counting
	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		enc = nil // Continue without tokenizer if it fails
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
			toolInfo := toolInfo{
				name:        tool.Tool.Name,
				description: tool.Tool.Description,
				toolsetName: toolsetName,
				isReadOnly:  tool.Tool.Annotations.ReadOnlyHint != nil && *tool.Tool.Annotations.ReadOnlyHint,
			}
			
			// Estimate token count for this tool using the actual MCP tool
			if enc != nil {
				toolInfo.tokenCount = estimateToolTokens(enc, tool)
			}
			
			ts.tools = append(ts.tools, toolInfo)
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

// estimateToolTokens estimates the token count for a tool's MCP definition
// This serializes the complete tool definition (name, description, annotations, inputSchema)
// to approximate the token count that will be sent to the LLM in the tools/list response
func estimateToolTokens(enc tokenizer.Codec, mcpTool server.ServerTool) int {
	if enc == nil {
		return 0
	}

	// Serialize the full MCP tool definition to JSON
	// This is what gets sent to the LLM in the tools/list response
	jsonBytes, err := json.Marshal(mcpTool.Tool)
	if err != nil {
		return 0
	}

	// Encode and count tokens
	tokens, _, _ := enc.Encode(string(jsonBytes))
	return len(tokens)
}

// formatTokenCount formats token count with K suffix for thousands
func formatTokenCount(count int) string {
	if count >= 1000 {
		return fmt.Sprintf("%.1fK", float64(count)/1000)
	}
	return fmt.Sprintf("%d", count)
}


func runConfigure(cmd *cobra.Command, args []string) error {
	// Dynamically get available toolsets
	toolsets := getAvailableToolsets()

	// Create and run the Bubble Tea program
	p := tea.NewProgram(
		initialConfigureModel(toolsets),
		tea.WithAltScreen(),
	)

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("error running configuration: %w", err)
	}

	// Cast the final model and print confirmation if needed
	if m, ok := finalModel.(configureModel); ok {
		if m.confirmed {
			// Print the confirmation output after exiting alt screen
			fmt.Print(m.renderConfirmation())
		}
	}

	return nil
}