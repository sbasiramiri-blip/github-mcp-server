package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// Styles for the wizard UI
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

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Bold(true)
)

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

type wizardModel struct {
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
}

func initialWizardModel(toolsets []toolsetInfo) wizardModel {
	// Flatten all tools
	var allTools []toolInfo
	for _, ts := range toolsets {
		for _, tool := range ts.tools {
			allTools = append(allTools, tool)
		}
	}

	return wizardModel{
		toolsets:      toolsets,
		allTools:      allTools,
		filteredTools: allTools,
		selected:      make(map[int]bool),
		width:         80,
		height:        24,
	}
}

func (m wizardModel) Init() tea.Cmd {
	return nil
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
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

func (m *wizardModel) applyFilter() {
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

func (m *wizardModel) adjustViewport() {
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

func (m wizardModel) View() string {
	if m.quitting && !m.confirmed {
		return dimStyle.Render("\nConfiguration cancelled.\n")
	}

	if m.confirmed {
		return m.renderConfirmation()
	}

	var s strings.Builder

	// Header
	s.WriteString(titleStyle.Render("🧙 GitHub MCP Server Configuration Wizard"))
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

	selectedCount := 0
	for _, selected := range m.selected {
		if selected {
			selectedCount++
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
			category := dimStyle.Render(fmt.Sprintf("[%s]", tool.toolsetName))
			line += category

			// Add description (truncated if needed)
			desc := getFirstSentence(tool.description)
			if len(desc) > 60 {
				desc = desc[:57] + "..."
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
	s.WriteString(helpStyle.Render(fmt.Sprintf("Selected: %d tools", selectedCount)))
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

func (m wizardModel) renderConfirmation() string {
	var s strings.Builder

	// Get selected tools
	var selectedTools []string
	for i, selected := range m.selected {
		if selected && i < len(m.filteredTools) {
			selectedTools = append(selectedTools, m.filteredTools[i].name)
		}
	}

	// Sort for consistent output
	sort.Strings(selectedTools)

	s.WriteString("\n")
	s.WriteString(successStyle.Render("✅ Configuration Complete!"))
	s.WriteString("\n\n")
	s.WriteString(titleStyle.Render("Selected Tools:"))
	s.WriteString("\n")

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

	// Build command args
	cmdArgs := []string{
		"run",
		"cmd/github-mcp-server/main.go",
		"cmd/github-mcp-server/wizard.go",
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
	s.WriteString(dimStyle.Render(`"args": [`))
	s.WriteString("\n")
	for i, arg := range cmdArgs {
		comma := ","
		if i == len(cmdArgs)-1 {
			comma = ""
		}
		s.WriteString(dimStyle.Render(fmt.Sprintf(`    "%s"%s`, arg, comma)))
		s.WriteString("\n")
	}
	s.WriteString(dimStyle.Render(`],`))
	s.WriteString("\n\n")

	s.WriteString(titleStyle.Render("Or run directly with:"))
	s.WriteString("\n")
	s.WriteString(successStyle.Render(fmt.Sprintf("go %s", strings.Join(cmdArgs, " "))))
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
	// Dynamically get available toolsets
	toolsets := getAvailableToolsets()

	// Create and run the Bubble Tea program
	p := tea.NewProgram(
		initialWizardModel(toolsets),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running wizard: %w", err)
	}

	return nil
}