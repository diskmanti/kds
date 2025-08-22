// kds is a standalone command-line tool for viewing data stored in Kubernetes Secrets
// using a beautiful and fast terminal user interface with fuzzy-finding.
// It provides a dual-pane layout with a searchable list of secrets on the left
// and a live, scrollable, word-wrapped view of the decrypted secret data on the right.
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// --- Build Information ---
// These variables are placeholders and will be injected by GoReleaser at build time
// using ldflags to provide versioning information.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// --- STYLES and CONSTANTS ---
var (
	primaryColor     = lipgloss.Color("#00BFFF")
	focusedColor     = lipgloss.Color("#AD58B4")
	errorColor       = lipgloss.Color("#FF4136")
	noteStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	titleStyle       = lipgloss.NewStyle().Foreground(primaryColor).Bold(true).MarginBottom(1)
	errorTitleStyle  = titleStyle.Copy().Foreground(errorColor)
	errorStyle       = lipgloss.NewStyle().Foreground(errorColor).Bold(true)
	paneBaseStyle    = lipgloss.NewStyle().Padding(1, 2).BorderStyle(lipgloss.RoundedBorder())
	leftPaneStyle    = paneBaseStyle.Copy().BorderForeground(primaryColor)
	focusedLeftPane  = leftPaneStyle.Copy().BorderForeground(focusedColor)
	rightPaneStyle   = paneBaseStyle.Copy().BorderForeground(primaryColor)
	focusedRightPane = rightPaneStyle.Copy().BorderForeground(focusedColor)
)

// pane identifies which of the two panes is currently focused.
type pane int

const (
	leftPane pane = iota
	rightPane
)

// --- BUBBLE TEA MODEL ---

// item represents a single Kubernetes secret in our list. It satisfies the
// list.Item interface, making it usable in the bubbles/list component.
type item struct{ name, namespace string }

func (i item) Title() string       { return i.name }
func (i item) Description() string { return fmt.Sprintf("Namespace: %s", i.namespace) }
func (i item) FilterValue() string { return i.name }

// itemSource is a slice of items that satisfies the fuzzy.Source interface,
// allowing our fuzzy-finder library to search through it.
type itemSource []item

func (s itemSource) String(i int) string { return s[i].name }
func (s itemSource) Len() int            { return len(s) }

// --- MESSAGES ---
// Bubble Tea applications communicate via messages.

// secretDataLoadedMsg is sent when a secret's data has been successfully fetched.
type secretDataLoadedMsg struct {
	secretName string
	data       map[string]string
}

// secretDataErrorMsg is sent when fetching a secret's data fails.
// This allows for graceful error handling within the UI.
type secretDataErrorMsg struct {
	secretName string
	err        error
}

// fatalErrorMsg is used for unrecoverable errors, which will display a final
// error screen before the application quits.
type fatalErrorMsg struct{ err error }

// model is the main state of our Bubble Tea application.
type model struct {
	// clientset is the Kubernetes API client.
	clientset *kubernetes.Clientset
	namespace string

	// Components
	list      list.Model
	textinput textinput.Model
	spinner   spinner.Model
	viewport  viewport.Model

	// State
	allItems        itemSource                  // Holds all secrets fetched from the API.
	highlightedItem item                        // The secret currently selected in the list.
	secretCache     map[string]map[string]string // Caches secret data to avoid repeated API calls.
	secretErrCache  map[string]error            // Caches errors for specific secrets.
	width, height   int
	focus           pane // Tracks which pane is active.
	loading         bool // True when fetching the initial list of secrets.
	loadingSecret   bool // True when fetching data for a single secret.
	ready           bool // True once the initial layout has been calculated.
	err             error
}

func NewModel(clientset *kubernetes.Clientset, namespace string) model {
	ti := textinput.New()
	ti.Placeholder = "Search for a secret..."
	ti.Focus()
	ti.Prompt = "🔎 "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(primaryColor)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(primaryColor)

	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Kubernetes Secrets"
	l.Styles.Title = noteStyle
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false) // We handle filtering manually with fuzzy matching.

	return model{
		clientset:      clientset,
		namespace:      namespace,
		textinput:      ti,
		spinner:        s,
		list:           l,
		loading:        true,
		focus:          leftPane, // Start focus on the left pane.
		secretCache:    make(map[string]map[string]string),
		secretErrCache: make(map[string]error),
	}
}

// Init is the first command run when the Bubble Tea program starts.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, fetchSecrets(m.clientset, m.namespace))
}

// --- COMMANDS ---
// Commands are functions that perform I/O and return a message.

// fetchSecrets fetches the list of all secrets in a namespace.
func fetchSecrets(clientset *kubernetes.Clientset, namespace string) tea.Cmd {
	return func() tea.Msg {
		secrets, err := clientset.CoreV1().Secrets(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fatalErrorMsg{err} // This is a critical failure.
		}
		if len(secrets.Items) == 0 {
			return fatalErrorMsg{fmt.Errorf("no secrets found in namespace '%s'", namespace)}
		}
		items := make(itemSource, len(secrets.Items))
		for i, secret := range secrets.Items {
			items[i] = item{name: secret.Name, namespace: secret.Namespace}
		}
		return items
	}
}

// fetchSecretData fetches and decodes the data for a single secret.
func fetchSecretData(clientset *kubernetes.Clientset, secretName, namespace string) tea.Cmd {
	return func() tea.Msg {
		secret, err := clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			// This is a non-fatal error; we can still browse other secrets.
			return secretDataErrorMsg{secretName: secretName, err: err}
		}
		data := make(map[string]string)
		for key, value := range secret.Data {
			decodedValue, err := base64.StdEncoding.DecodeString(string(value))
			if err != nil {
				data[key] = string(value) + " " + noteStyle.Render("(raw, base64 decoding failed)")
			} else {
				data[key] = string(decodedValue)
			}
		}
		return secretDataLoadedMsg{secretName: secretName, data: data}
	}
}

// --- UPDATE ---
// Update is the core message handler for the Bubble Tea application.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var listCmd, inputCmd, vpCmd tea.Cmd

	// The spinner should tick whenever we are in a loading state.
	if m.loading || m.loadingSecret {
		var spinCmd tea.Cmd
		m.spinner, spinCmd = m.spinner.Update(msg)
		cmds = append(cmds, spinCmd)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		helpHeight := lipgloss.Height(m.viewHelp())
		mainContentHeight := m.height - helpHeight
		leftPaneWidth := m.width / 2
		rightPaneWidth := m.width - leftPaneWidth
		textInputHeight := lipgloss.Height(m.textinput.View())
		listHeight := mainContentHeight - textInputHeight - paneBaseStyle.GetVerticalPadding()
		m.list.SetSize(leftPaneWidth-paneBaseStyle.GetHorizontalPadding(), listHeight)
		m.viewport.Width = rightPaneWidth - rightPaneStyle.GetHorizontalPadding()
		m.viewport.Height = mainContentHeight - rightPaneStyle.GetVerticalPadding()
		if !m.ready {
			m.ready = true
		} else {
			// Re-wrap content on resize if we have content.
			if content, ok := m.secretCache[m.highlightedItem.name]; ok {
				m.viewport.SetContent(m.formatSecretData(content))
			}
		}
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "tab":
			if m.focus == leftPane {
				m.focus = rightPane
				m.textinput.Blur()
			} else {
				m.focus = leftPane
				m.textinput.Focus()
			}
			return m, nil
		}

	// Received the initial list of secrets.
	case itemSource:
		m.loading = false
		m.allItems = msg
		listItems := make([]list.Item, len(m.allItems))
		for i, it := range m.allItems {
			listItems[i] = it
		}
		cmds = append(cmds, m.list.SetItems(listItems))
		// Automatically trigger a fetch for the first item.
		if len(m.list.Items()) > 0 {
			m.highlightedItem = m.list.SelectedItem().(item)
			m.loadingSecret = true
			cmds = append(cmds, fetchSecretData(m.clientset, m.highlightedItem.name, m.highlightedItem.namespace))
		}
		return m, tea.Batch(cmds...)

	// Received data for a single secret.
	case secretDataLoadedMsg:
		if m.highlightedItem.name == msg.secretName {
			m.loadingSecret = false
			m.secretCache[msg.secretName] = msg.data // Cache the data.
			delete(m.secretErrCache, msg.secretName) // Clear any previous errors.
			m.viewport.SetContent(m.formatSecretData(msg.data))
			m.viewport.GotoTop()
		}
		return m, nil

	// A non-fatal error occurred while fetching one secret's data.
	case secretDataErrorMsg:
		if m.highlightedItem.name == msg.secretName {
			m.loadingSecret = false
			m.secretErrCache[msg.secretName] = msg.err // Cache the error.
		}
		return m, nil

	// A fatal, unrecoverable error occurred.
	case fatalErrorMsg:
		m.err = msg.err
		m.loading = false
		m.loadingSecret = false
		return m, tea.Quit // Quit the program.
	}

	if m.loading {
		return m, tea.Batch(cmds...)
	}

	// --- FOCUS-BASED INPUT HANDLING ---
	if m.focus == leftPane {
		m.textinput, inputCmd = m.textinput.Update(msg)
		cmds = append(cmds, inputCmd)

		// Filter the list based on search input.
		pattern := m.textinput.Value()
		var newItems []list.Item
		if pattern == "" {
			newItems = make([]list.Item, len(m.allItems))
			for i, it := range m.allItems {
				newItems[i] = it
			}
		} else {
			matches := fuzzy.FindFrom(pattern, m.allItems)
			newItems = make([]list.Item, len(matches))
			for i, match := range matches {
				newItems[i] = m.allItems[match.Index]
			}
		}
		cmds = append(cmds, m.list.SetItems(newItems))

		m.list, listCmd = m.list.Update(msg)
		cmds = append(cmds, listCmd)

		// If the highlighted item changes, fetch its data (from cache or API).
		if selected, ok := m.list.SelectedItem().(item); ok && m.highlightedItem.name != selected.name {
			m.highlightedItem = selected
			// Check cache first.
			if _, found := m.secretCache[selected.name]; !found {
				m.loadingSecret = true
				cmds = append(cmds, fetchSecretData(m.clientset, selected.name, selected.namespace))
			}
		}
	} else { // Right Pane is focused, so handle viewport scrolling.
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
	}

	return m, tea.Batch(cmds...)
}

// --- VIEW ---
// The View function is responsible for rendering the UI.

// formatSecretData formats the key-value data into a word-wrapped string for the viewport.
func (m *model) formatSecretData(data map[string]string) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.highlightedItem.name))
	for key, value := range data {
		b.WriteString(fmt.Sprintf("%s: %s\n", key, value))
	}
	return wordwrap.String(b.String(), m.viewport.Width)
}

func (m *model) viewHelp() string {
	return noteStyle.Render("  ↑/↓: navigate | tab: switch pane | q: quit")
}

func (m *model) viewLeftPane() string {
	return lipgloss.JoinVertical(lipgloss.Left, m.textinput.View(), m.list.View())
}

func (m *model) viewRightPane() string {
	// If an error exists for this secret, display it.
	if err, found := m.secretErrCache[m.highlightedItem.name]; found {
		var b strings.Builder
		b.WriteString(errorTitleStyle.Render("Error"))
		b.WriteString(fmt.Sprintf("Failed to fetch secret '%s':\n\n", m.highlightedItem.name))
		b.WriteString(errorStyle.Render(err.Error()))
		return wordwrap.String(b.String(), m.viewport.Width)
	}
	// If data is in the cache, display it.
	if data, found := m.secretCache[m.highlightedItem.name]; found {
		m.viewport.SetContent(m.formatSecretData(data))
		return m.viewport.View()
	}
	// If we are currently fetching data, show the spinner.
	if m.loadingSecret {
		return fmt.Sprintf("\n%s Loading secret data...", m.spinner.View())
	}
	// Default message.
	return noteStyle.Render("Select a secret to view its data.")
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n%s: %v\n\n", errorStyle.Render("Fatal Error"), m.err)
	}
	if !m.ready {
		return "Initializing..."
	}
	if m.loading {
		return fmt.Sprintf("\n  %s Searching for secrets in namespace '%s'...\n\n", m.spinner.View(), m.namespace)
	}

	var currentLeftPaneStyle, currentRightPaneStyle lipgloss.Style
	if m.focus == leftPane {
		currentLeftPaneStyle = focusedLeftPane
		currentRightPaneStyle = rightPaneStyle
	} else {
		currentLeftPaneStyle = leftPaneStyle
		currentRightPaneStyle = focusedRightPane
	}

	helpHeight := lipgloss.Height(m.viewHelp())
	mainContentHeight := m.height - helpHeight
	leftPaneWidth := m.width / 2
	rightPaneWidth := m.width - leftPaneWidth

	mainPanes := lipgloss.JoinHorizontal(lipgloss.Top,
		currentLeftPaneStyle.Width(leftPaneWidth).Height(mainContentHeight).Render(m.viewLeftPane()),
		currentRightPaneStyle.Width(rightPaneWidth).Height(mainContentHeight).Render(m.viewRightPane()),
	)

	return lipgloss.JoinVertical(lipgloss.Left, mainPanes, m.viewHelp())
}

// --- MAIN & COBRA ---
func main() {
	var namespace, kubeconfig string
	rootCmd := &cobra.Command{
		Use:   "kds [secret-name]",
		Short: "A tool with fuzzy-finding to view Kubernetes secrets.",
		Long:  `kds is a CLI tool with a rich terminal UI for browsing, finding, and viewing Kubernetes secrets with live decryption.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
			if err != nil {
				return fmt.Errorf("failed to build kubeconfig: %w", err)
			}
			clientset, err := kubernetes.NewForConfig(restConfig)
			if err != nil {
				return fmt.Errorf("failed to create kubernetes clientset: %w", err)
			}
			if namespace == "" {
				ns, err := getNamespaceFromKubeconfig(kubeconfig)
				if err != nil {
					return err
				}
				namespace = ns
			}
			if len(args) > 0 {
				return viewSecretDataDirectly(clientset, args[0], namespace)
			}
			p := tea.NewProgram(NewModel(clientset, namespace), tea.WithAltScreen(), tea.WithMouseAllMotion())
			if _, err := p.Run(); err != nil {
				// Don't wrap the error here, as Bubble Tea already provides a good message.
				return err
			}
			return nil
		},
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version, commit, and build date of kds",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kds Version: %s\n", version)
			fmt.Printf("Commit: %s\n", commit)
			fmt.Printf("Built at: %s\n", date)
		},
	}
	rootCmd.AddCommand(versionCmd)

	if home := homedir.HomeDir(); home != "" {
		rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) path to kubeconfig")
	} else {
		rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig")
	}
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "namespace (overrides context)")
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func getNamespaceFromKubeconfig(kubeconfigPath string) (string, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}
	apiConfig, err := loadingRules.Load()
	if err != nil {
		return "", fmt.Errorf("failed to load api config: %w", err)
	}
	clientConfig := clientcmd.NewNonInteractiveClientConfig(*apiConfig, apiConfig.CurrentContext, &clientcmd.ConfigOverrides{}, nil)
	ns, _, err := clientConfig.Namespace()
	if err != nil {
		return "", fmt.Errorf("failed to get namespace from context: %w", err)
	}
	return ns, nil
}

func viewSecretDataDirectly(clientset *kubernetes.Clientset, secretName, namespace string) error {
	secret, err := clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get secret '%s': %w", secretName, err)
	}
	fmt.Println(titleStyle.Render(fmt.Sprintf("Data for secret '%s' in namespace '%s'", secretName, namespace)))
	for key, value := range secret.Data {
		decodedValue, err := base64.StdEncoding.DecodeString(string(value))
		if err != nil {
			fmt.Printf("  %s: %s %s\n", key, string(value), noteStyle.Render("(raw value)"))
		} else {
			fmt.Printf("  %s: %s\n", key, string(decodedValue))
		}
	}
	return nil
}// A test comment
