// kds is a standalone command-line tool for viewing data stored in Kubernetes Secrets
// using a beautiful and fast terminal user interface with fuzzy-finding.
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// --- Build Information ---
// These variables are placeholders and will be injected by GoReleaser at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// --- STYLES ---
// These styles are used throughout the application for a consistent look and feel.
var (
	primaryColor = lipgloss.Color("#00BFFF")
	errorColor   = lipgloss.Color("#FF4136")

	// noteStyle is used for supplementary information, like help text.
	noteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	// titleStyle is used for prominent headers.
	titleStyle = lipgloss.NewStyle().Foreground(primaryColor).Bold(true).Margin(0, 0, 1, 0)

	// errorStyle is used for displaying error messages.
	errorStyle = lipgloss.NewStyle().Foreground(errorColor).Bold(true)
)

// --- BUBBLE TEA MODEL ---

// item represents a single Kubernetes secret in our list. It satisfies the
// list.Item interface.
type item struct {
	name, namespace string
}

// Title is required by the list.Item interface and returns the primary text to display.
func (i item) Title() string { return i.name }

// Description is required by the list.Item interface and returns the secondary text.
func (i item) Description() string { return fmt.Sprintf("Namespace: %s", i.namespace) }

// FilterValue is required by the list.Item interface and returns the string
// that the list's default filter will use. In our case, it's the same as the title.
func (i item) FilterValue() string { return i.name }

// itemSource is a slice of items that satisfies the fuzzy.Source interface,
// allowing our fuzzy-finder library to search through it.
type itemSource []item

// String returns the string representation of an element at a given index.
// This is required by the fuzzy.Source interface.
func (s itemSource) String(i int) string { return s[i].name }

// Len returns the number of items in the source.
// This is required by the fuzzy.Source interface.
func (s itemSource) Len() int { return len(s) }

// model is the main state of our Bubble Tea application.
type model struct {
	textinput textinput.Model // The input field for fuzzy searching.
	list      list.Model      // The list component to display secrets.
	spinner   spinner.Model   // A spinner to show while loading.
	clientset *kubernetes.Clientset

	namespace string
	allItems  itemSource // Holds all secrets fetched from the API.
	loading   bool       // True when fetching secrets, false otherwise.

	selectedSecret string // The name of the secret chosen by the user.
	err            error  // Any error that occurs during operation.
}

// NewModel creates and initializes a new TUI model.
func NewModel(clientset *kubernetes.Clientset, namespace string) model {
	ti := textinput.New()
	ti.Placeholder = "Search for a secret..."
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 50
	ti.Prompt = "🔎 "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(primaryColor)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(primaryColor)

	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Secrets"
	l.Styles.Title = noteStyle
	l.SetShowHelp(false) // We manage help text manually in the View.

	return model{
		textinput: ti,
		spinner:   s,
		list:      l,
		clientset: clientset,
		namespace: namespace,
		loading:   true,
	}
}

// Init is the first command run when the Bubble Tea program starts.
func (m model) Init() tea.Cmd {
	// We start the spinner and fetch the secrets from the Kubernetes API concurrently.
	return tea.Batch(m.spinner.Tick, fetchSecrets(m.clientset, m.namespace))
}

// --- MESSAGES & COMMANDS ---

// secretsLoadedMsg is a message sent when the Kubernetes secrets have been successfully fetched.
type secretsLoadedMsg struct{ items itemSource }

// errorMsg is a message sent when an error occurs during an asynchronous command.
type errorMsg struct{ err error }

// fetchSecrets is a Bubble Tea command that fetches secrets from the Kubernetes API
// and sends a secretsLoadedMsg or errorMsg.
func fetchSecrets(clientset *kubernetes.Clientset, namespace string) tea.Cmd {
	return func() tea.Msg {
		secrets, err := clientset.CoreV1().Secrets(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return errorMsg{err}
		}
		if len(secrets.Items) == 0 {
			return errorMsg{fmt.Errorf("no secrets found in namespace '%s'", namespace)}
		}
		items := make(itemSource, len(secrets.Items))
		for i, secret := range secrets.Items {
			items[i] = item{name: secret.Name, namespace: secret.Namespace}
		}
		return secretsLoadedMsg{items}
	}
}

// --- UPDATE ---

// Update is the core message handler for the Bubble Tea application. It's called
// whenever a message is received.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Adjust component sizes when the terminal window is resized.
		m.list.SetSize(msg.Width, msg.Height-4) // Account for textinput and help text.
		return m, nil

	case tea.KeyMsg:
		// Handle key presses.
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "enter":
			if !m.loading && len(m.list.Items()) > 0 {
				selectedItem, ok := m.list.SelectedItem().(item)
				if ok {
					m.selectedSecret = selectedItem.name // Store the user's choice.
					return m, tea.Quit                   // Immediately quit the TUI.
				}
			}
		}

	// --- Custom Messages ---
	case secretsLoadedMsg:
		m.loading = false
		m.allItems = msg.items
		// Initially, display all items in the list.
		listItems := make([]list.Item, len(m.allItems))
		for i, it := range m.allItems {
			listItems[i] = it
		}
		m.list.SetItems(listItems)
		return m, nil

	case errorMsg:
		m.err = msg.err
		return m, nil
	}

	// If we're loading, we only need to update the spinner.
	if m.loading {
		var spinCmd tea.Cmd
		m.spinner, spinCmd = m.spinner.Update(msg)
		return m, spinCmd
	}

	// --- Handle Fuzzy Finding ---
	// First, update the text input model.
	var inputCmd tea.Cmd
	m.textinput, inputCmd = m.textinput.Update(msg)
	cmds = append(cmds, inputCmd)

	pattern := m.textinput.Value()
	var newListItems []list.Item

	if pattern == "" {
		// If search is empty, show all items.
		newListItems = make([]list.Item, len(m.allItems))
		for i, it := range m.allItems {
			newListItems[i] = it
		}
	} else {
		// Otherwise, perform the fuzzy search.
		matches := fuzzy.FindFrom(pattern, m.allItems)
		newListItems = make([]list.Item, len(matches))
		for i, match := range matches {
			newListItems[i] = m.allItems[match.Index]
		}
	}

	// Update the list with the new filtered/ranked items and pass along any commands.
	var listCmd tea.Cmd
	m.list.SetItems(newListItems)
	m.list, listCmd = m.list.Update(msg)
	cmds = append(cmds, listCmd)

	return m, tea.Batch(cmds...)
}

// --- VIEW ---

// View renders the UI based on the current state of the model.
func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  %s: %v\n\n  Press 'q' to quit.", errorStyle.Render("Error"), m.err)
	}
	if m.loading {
		return fmt.Sprintf("\n  %s Searching for secrets in namespace '%s'...\n\n", m.spinner.View(), m.namespace)
	}
	// The main view combines the search input, the list, and help text.
	help := noteStyle.Render("  ↑/↓: navigate | enter: select | q: quit")
	return fmt.Sprintf("\n%s\n\n%s\n%s", m.textinput.View(), m.list.View(), help)
}

// --- MAIN & COBRA ---

// main is the entry point for the entire application.
func main() {
	var namespace string
	var kubeconfig string
	var clientset *kubernetes.Clientset

	// rootCmd is the main command for the kds application.
	rootCmd := &cobra.Command{
		Use:   "kds [secret-name]",
		Short: "A tool with fuzzy-finding to view Kubernetes secrets.",
		Long: `kds (Kubernetes Decode Secret) is a CLI tool with a rich terminal UI
for browsing, finding, and viewing Kubernetes secrets.`,
		// PreRunE is run by Cobra before the main RunE. It's used here for setup.
		PreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
			if err != nil {
				return fmt.Errorf("failed to build kubeconfig: %w", err)
			}
			clientset, err = kubernetes.NewForConfig(restConfig)
			if err != nil {
				return fmt.Errorf("failed to create kubernetes clientset: %w", err)
			}
			if namespace == "" {
				// If namespace flag is not set, determine it from the current kubeconfig context.
				loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
				if kubeconfig != "" {
					loadingRules.ExplicitPath = kubeconfig
				}
				apiConfig, err := loadingRules.Load()
				if err != nil {
					return fmt.Errorf("failed to load api config: %w", err)
				}
				clientConfig := clientcmd.NewNonInteractiveClientConfig(*apiConfig, apiConfig.CurrentContext, &clientcmd.ConfigOverrides{}, nil)
				ns, _, err := clientConfig.Namespace()
				if err != nil {
					return fmt.Errorf("failed to get namespace from context: %w", err)
				}
				namespace = ns
			}
			return nil
		},
		// RunE is the main execution logic for the command.
		RunE: func(cmd *cobra.Command, args []string) error {
			// If a secret name is provided as an argument, run non-interactively and exit.
			if len(args) > 0 {
				return viewSecretDataDirectly(clientset, args[0], namespace)
			}

			// Otherwise, start the TUI to let the user choose a secret.
			tuiProgram := tea.NewProgram(NewModel(clientset, namespace), tea.WithAltScreen())
			finalModel, err := tuiProgram.Run()
			if err != nil {
				return fmt.Errorf("TUI failed to run: %w", err)
			}

			// After the TUI quits, check the final model to see what was selected.
			m, ok := finalModel.(model)
			if !ok {
				return fmt.Errorf("internal error: could not cast final model")
			}

			// If a secret was selected, print its data.
			if m.selectedSecret != "" {
				fmt.Println() // Add a newline for cleaner output.
				return viewSecretDataDirectly(clientset, m.selectedSecret, namespace)
			}

			return nil // User quit without choosing.
		},
	}

	// versionCmd defines the 'kds version' command.
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version, commit, and build date of kds",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kds Version: %s\n", version)
			fmt.Printf("Commit: %s\n", commit)
			fmt.Printf("Built at: %s\n", date)
		},
	}

	// Add the version command to the root command.
	rootCmd.AddCommand(versionCmd)

	// Setup Cobra flags.
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

// viewSecretDataDirectly handles the non-interactive output. It fetches a single
// secret and prints its data to standard output.
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
}
