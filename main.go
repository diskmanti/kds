// kds is a standalone command-line tool for viewing data stored in Kubernetes Secrets.
// It uses a beautiful and fast terminal user interface (TUI) with fuzzy-finding,
// built with the Charm Bubble Tea framework.
//
// The application provides a dual-pane layout with a searchable list of secrets on
// the left and a live, scrollable, word-wrapped view of the decrypted secret data
// on the right. It is designed to be a more efficient and user-friendly alternative
// to the standard `kubectl get secret ...` workflow.
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
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// --- Build Information ---

// These variables are placeholders and will be injected by the GoReleaser build process
// using ldflags. This provides versioning information in the final binary.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// --- STYLES and CONSTANTS ---

// This block defines the color palette and styling for the entire TUI,
// ensuring a consistent and professional look and feel.
var (
	primaryColor     = lipgloss.Color("#00BFFF")
	focusedColor     = lipgloss.Color("#AD58B4")
	errorColor       = lipgloss.Color("#FF4136")
	noteStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	titleStyle       = lipgloss.NewStyle().Foreground(primaryColor).Bold(true).MarginBottom(1)
	errorTitleStyle  = titleStyle.Foreground(errorColor)
	errorStyle       = lipgloss.NewStyle().Foreground(errorColor).Bold(true)
	paneBaseStyle    = lipgloss.NewStyle().Padding(1, 2).BorderStyle(lipgloss.RoundedBorder())
	leftPaneStyle    = paneBaseStyle.BorderForeground(primaryColor)
	focusedLeftPane  = leftPaneStyle.BorderForeground(focusedColor)
	rightPaneStyle   = paneBaseStyle.BorderForeground(primaryColor)
	focusedRightPane = rightPaneStyle.BorderForeground(focusedColor)
)

// pane identifies which of the two UI panes is currently focused by the user.
type pane int

const (
	leftPane pane = iota
	rightPane
)

// k8sClient defines the interface for the Kubernetes client.
// This allows us to use a real clientset in production and a fake clientset
// during testing, which is a crucial practice for writing testable code.
// It only includes the methods our application actually needs from the client.
type k8sClient interface {
	CoreV1() corev1client.CoreV1Interface
}

// --- BUBBLE TEA MODEL ---

// item represents a single Kubernetes secret in our list.
// It satisfies the `bubbles/list.Item` interface, making it usable in the list component.
type item struct {
	name      string
	namespace string
}

// Title returns the primary text to display in the list.
func (i item) Title() string { return i.name }

// Description returns the secondary text to display in the list.
func (i item) Description() string { return fmt.Sprintf("Namespace: %s", i.namespace) }

// FilterValue is the string that the list's fuzzy-finder will use for matching.
func (i item) FilterValue() string { return i.name }

// itemSource is a slice of items that satisfies the `fuzzy.Source` interface,
// allowing our fuzzy-finder library to search through it.
type itemSource []item

// String returns the string representation of an element at a given index for the fuzzy finder.
func (s itemSource) String(i int) string { return s[i].name }

// Len returns the number of items in the source for the fuzzy finder.
func (s itemSource) Len() int { return len(s) }

// --- MESSAGES ---
// Bubble Tea applications are event-driven and communicate via messages.

// secretDataLoadedMsg is sent when a secret's data has been successfully fetched.
type secretDataLoadedMsg struct {
	secretName string
	data       map[string]string
}

// secretDataErrorMsg is sent when fetching a specific secret's data fails.
// This is a non-fatal error, allowing the UI to continue running.
type secretDataErrorMsg struct {
	secretName string
	err        error
}

// fatalErrorMsg is used for unrecoverable errors (e.g., cannot connect to Kubernetes),
// which will cause the application to display an error and quit.
type fatalErrorMsg struct{ err error }

// model is the main state of our Bubble Tea application.
// It holds all the UI components and the application's current state.
type model struct {
	// clientset is the Kubernetes API client (can be real or fake).
	clientset k8sClient
	// namespace is the Kubernetes namespace we are currently viewing.
	namespace string

	// --- Components ---
	list      list.Model
	textinput textinput.Model
	spinner   spinner.Model
	viewport  viewport.Model // For the scrollable right-hand pane.

	// --- State ---
	allItems        itemSource                   // Holds all secrets fetched from the API.
	highlightedItem item                         // The secret currently selected in the list.
	secretCache     map[string]map[string]string // Caches secret data to avoid repeated API calls.
	secretErrCache  map[string]error             // Caches errors for specific secrets to show in the UI.
	width, height   int                          // Current terminal dimensions.
	focus           pane                         // Tracks which pane is active (left or right).
	loading         bool                         // True when fetching the initial list of secrets.
	loadingSecret   bool                         // True when fetching data for a single secret.
	ready           bool                         // True once the initial layout has been calculated.
	err             error                        // Stores any fatal error that occurs.
}

// NewModel is the constructor for our TUI model. It initializes all the components
// and sets the initial state of the application.
func NewModel(clientset k8sClient, namespace string) model {
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
	l.SetFilteringEnabled(false) // We handle filtering manually with our fuzzy matcher.

	return model{
		clientset:      clientset,
		namespace:      namespace,
		textinput:      ti,
		spinner:        s,
		list:           l,
		loading:        true,
		focus:          leftPane,
		secretCache:    make(map[string]map[string]string),
		secretErrCache: make(map[string]error),
	}
}

// Init is the first command run when the Bubble Tea program starts.
// It kicks off the initial I/O, like fetching the list of secrets.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, fetchSecrets(m.clientset, m.namespace))
}

// --- COMMANDS ---
// Commands are functions that perform I/O (like API calls) and return a message.

// fetchSecrets is a command that fetches the list of all secrets in a namespace.
// It returns an itemSource message on success or a fatalErrorMsg on failure.
func fetchSecrets(clientset k8sClient, namespace string) tea.Cmd {
	return func() tea.Msg {
		secrets, err := clientset.CoreV1().Secrets(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fatalErrorMsg{err}
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

// fetchSecretData is a command that fetches and decodes the data for a single secret.
// It returns a secretDataLoadedMsg on success or a secretDataErrorMsg on failure.
func fetchSecretData(clientset k8sClient, secretName, namespace string) tea.Cmd {
	return func() tea.Msg {
		secret, err := clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
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

// Update is the main message handler for the TUI. It acts as a dispatcher,
// routing messages to more specific handler functions to keep cognitive complexity low.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	// The spinner should tick whenever we are in a loading state.
	if m.loading || m.loadingSecret {
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Delegate message handling to a dedicated function.
	m, cmd = m.handleMessages(msg)
	cmds = append(cmds, cmd)

	// If we are not loading, handle user input based on which pane is focused.
	if !m.loading {
		m, cmd = m.handleFocusedPaneInput(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// handleMessages handles all incoming Bubble Tea messages by type.
func (m model) handleMessages(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	case itemSource:
		return m.handleSecretsLoaded(msg)
	case secretDataLoadedMsg:
		return m.handleSecretDataLoaded(msg)
	case secretDataErrorMsg:
		return m.handleSecretDataError(msg)
	case fatalErrorMsg:
		m.err = msg.err
		return m, tea.Quit
	default:
		return m, nil
	}
}

// handleWindowSize updates the layout when the terminal is resized.
func (m model) handleWindowSize(msg tea.WindowSizeMsg) (model, tea.Cmd) {
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
	} else if content, ok := m.secretCache[m.highlightedItem.name]; ok {
		m.viewport.SetContent(m.formatSecretData(content))
	}
	return m, nil
}

// handleKeyMsg handles all global keyboard input.
func (m model) handleKeyMsg(msg tea.KeyMsg) (model, tea.Cmd) {
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
	}
	return m, nil
}

// handleSecretsLoaded handles the message received after the initial list of secrets is fetched.
func (m model) handleSecretsLoaded(msg itemSource) (model, tea.Cmd) {
	m.loading = false
	m.allItems = msg
	listItems := make([]list.Item, len(m.allItems))
	for i, it := range m.allItems {
		listItems[i] = it
	}
	cmd := m.list.SetItems(listItems)

	if len(m.list.Items()) > 0 {
		if selected, ok := m.list.SelectedItem().(item); ok {
			m.highlightedItem = selected
			m.loadingSecret = true
			return m, tea.Batch(cmd, fetchSecretData(m.clientset, m.highlightedItem.name, m.highlightedItem.namespace))
		}
	}
	return m, cmd
}

// handleSecretDataLoaded handles the message received after a single secret's data is fetched.
func (m model) handleSecretDataLoaded(msg secretDataLoadedMsg) (model, tea.Cmd) {
	if m.highlightedItem.name == msg.secretName {
		m.loadingSecret = false
		m.secretCache[msg.secretName] = msg.data
		delete(m.secretErrCache, msg.secretName)
		m.viewport.SetContent(m.formatSecretData(msg.data))
		m.viewport.GotoTop()
	}
	return m, nil
}

// handleSecretDataError handles errors from fetching a single secret's data.
func (m model) handleSecretDataError(msg secretDataErrorMsg) (model, tea.Cmd) {
	if m.highlightedItem.name == msg.secretName {
		m.loadingSecret = false
		m.secretErrCache[msg.secretName] = msg.err
	}
	return m, nil
}

// handleFocusedPaneInput routes updates to the correct component based on which pane has focus.
func (m model) handleFocusedPaneInput(msg tea.Msg) (model, tea.Cmd) {
	// This function should only handle keyboard input, not other message types.
	if _, ok := msg.(tea.KeyMsg); !ok {
		return m, nil
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd

	if m.focus == leftPane {
		m.textinput, cmd = m.textinput.Update(msg)
		cmds = append(cmds, cmd)

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

		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)

		// Check if the highlighted item changed, and if so, fetch its data.
		if selected, ok := m.list.SelectedItem().(item); ok && m.highlightedItem.name != selected.name {
			m.highlightedItem = selected
			// Only fetch from the API if the data is not already in our cache.
			if _, found := m.secretCache[selected.name]; !found {
				m.loadingSecret = true
				cmds = append(cmds, fetchSecretData(m.clientset, selected.name, selected.namespace))
			}
		}
	} else { // Right Pane is focused
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// --- VIEW ---
// The View functions are responsible for rendering the UI based on the model's state.

// formatSecretData formats the key-value data into a word-wrapped string for the viewport.
func (m *model) formatSecretData(data map[string]string) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.highlightedItem.name))
	for key, value := range data {
		b.WriteString(fmt.Sprintf("%s: %s\n", key, value))
	}
	return wordwrap.String(b.String(), m.viewport.Width)
}

// viewHelp renders the help text at the bottom of the screen.
func (m *model) viewHelp() string {
	return noteStyle.Render("  ↑/↓: navigate | tab: switch pane | q: quit")
}

// viewLeftPane renders the content for the left-hand pane (search bar and list).
func (m *model) viewLeftPane() string {
	return lipgloss.JoinVertical(lipgloss.Left, m.textinput.View(), m.list.View())
}

// viewRightPane renders the content for the right-hand pane (secret data or status).
func (m *model) viewRightPane() string {
	if err, found := m.secretErrCache[m.highlightedItem.name]; found {
		var b strings.Builder
		b.WriteString(errorTitleStyle.Render("Error"))
		b.WriteString(fmt.Sprintf("Failed to fetch secret '%s':\n\n", m.highlightedItem.name))
		b.WriteString(errorStyle.Render(err.Error()))
		return wordwrap.String(b.String(), m.viewport.Width)
	}
	if data, found := m.secretCache[m.highlightedItem.name]; found {
		m.viewport.SetContent(m.formatSecretData(data))
		return m.viewport.View()
	}
	if m.loadingSecret {
		return fmt.Sprintf("\n%s Loading secret data...", m.spinner.View())
	}
	return noteStyle.Render("Select a secret to view its data.")
}

// View is the main render function for the entire TUI.
func (m model) View() string {
	// If a fatal error has occurred, show only the error message.
	if m.err != nil {
		return fmt.Sprintf("\n%s: %v\n\n", errorStyle.Render("Fatal Error"), m.err)
	}
	// Do not render anything until the initial layout is calculated.
	if !m.ready {
		return "Initializing..."
	}
	// Show a loading message while fetching the initial secret list.
	if m.loading {
		return fmt.Sprintf("\n  %s Searching for secrets in namespace '%s'...\n\n", m.spinner.View(), m.namespace)
	}

	// Determine which pane style to use based on focus.
	var currentLeftPaneStyle, currentRightPaneStyle lipgloss.Style
	if m.focus == leftPane {
		currentLeftPaneStyle = focusedLeftPane
		currentRightPaneStyle = rightPaneStyle
	} else {
		currentLeftPaneStyle = leftPaneStyle
		currentRightPaneStyle = focusedRightPane
	}

	// Calculate dimensions and join the panes together.
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
// This is the entry point for the application.

func main() {
	var namespace, kubeconfig string

	// rootCmd is the main command for the kds application, configured using Cobra.
	rootCmd := &cobra.Command{
		Use:   "kds [secret-name]",
		Short: "A tool with fuzzy-finding to view Kubernetes secrets.",
		Long:  `kds is a CLI tool for browsing, finding, and viewing Kubernetes secrets.`,
		RunE: func(_ *cobra.Command, args []string) error {
			// Set up Kubernetes client configuration.
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

			// If a secret name is provided as an argument, run in non-interactive mode.
			if len(args) > 0 {
				return viewSecretDataDirectly(clientset, args[0], namespace)
			}

			// Otherwise, start the interactive TUI.
			p := tea.NewProgram(NewModel(clientset, namespace), tea.WithAltScreen(), tea.WithMouseAllMotion())
			if _, err := p.Run(); err != nil {
				return err
			}
			return nil
		},
	}

	// versionCmd defines the 'kds version' command.
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version, commit, and build date of kds",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("kds Version: %s\n", version)
			fmt.Printf("Commit: %s\n", commit)
			fmt.Printf("Built at: %s\n", date)
		},
	}
	rootCmd.AddCommand(versionCmd)

	// Setup Cobra flags for command-line arguments.
	if home := homedir.HomeDir(); home != "" {
		rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) path to kubeconfig")
	} else {
		rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig")
	}
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "namespace (overrides context)")

	// Execute the root command.
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// getNamespaceFromKubeconfig parses the kubeconfig file to determine the active namespace.
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

// viewSecretDataDirectly handles the non-interactive output. It fetches a single
// secret and prints its data to standard output.
func viewSecretDataDirectly(clientset k8sClient, secretName, namespace string) error {
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
}
