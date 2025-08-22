// kds is a standalone command-line tool for viewing data stored in Kubernetes Secrets
// using a beautiful and fast terminal user interface with fuzzy-finding.
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
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1" // Import for the interface
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// --- Build Information ---
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
	errorTitleStyle  = titleStyle.Foreground(errorColor)
	errorStyle       = lipgloss.NewStyle().Foreground(errorColor).Bold(true)
	paneBaseStyle    = lipgloss.NewStyle().Padding(1, 2).BorderStyle(lipgloss.RoundedBorder())
	leftPaneStyle    = paneBaseStyle.BorderForeground(primaryColor)
	focusedLeftPane  = leftPaneStyle.BorderForeground(focusedColor)
	rightPaneStyle   = paneBaseStyle.BorderForeground(primaryColor)
	focusedRightPane = rightPaneStyle.BorderForeground(focusedColor)
)

type pane int

const (
	leftPane pane = iota
	rightPane
)

// --- NEW INTERFACE ---
// k8sClient defines the interface for the Kubernetes client.
// This allows us to use a real or fake clientset, which is essential for testing.
// It only includes the methods our application actually needs.
type k8sClient interface {
	CoreV1() corev1client.CoreV1Interface
}

// --- BUBBLE TEA MODEL ---
type item struct{ name, namespace string }

func (i item) Title() string       { return i.name }
func (i item) Description() string { return fmt.Sprintf("Namespace: %s", i.namespace) }
func (i item) FilterValue() string { return i.name }

type itemSource []item

func (s itemSource) String(i int) string { return s[i].name }
func (s itemSource) Len() int            { return len(s) }

type secretDataLoadedMsg struct {
	secretName string
	data       map[string]string
}
type secretDataErrorMsg struct {
	secretName string
	err        error
}
type fatalErrorMsg struct{ err error }

type model struct {
	clientset k8sClient // MODIFIED: Use the interface, not the concrete struct.
	namespace string
	list      list.Model
	textinput textinput.Model
	spinner   spinner.Model
	viewport  viewport.Model
	allItems        itemSource
	highlightedItem item
	secretCache     map[string]map[string]string
	secretErrCache  map[string]error
	width, height   int
	focus           pane
	loading         bool
	loadingSecret   bool
	ready           bool
	err             error
}

func NewModel(clientset k8sClient, namespace string) model { // MODIFIED: Use the interface
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
	l.SetFilteringEnabled(false)
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

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, fetchSecrets(m.clientset, m.namespace))
}

// --- COMMANDS ---
func fetchSecrets(clientset k8sClient, namespace string) tea.Cmd { // MODIFIED: Use the interface
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

func fetchSecretData(clientset k8sClient, secretName, namespace string) tea.Cmd { // MODIFIED: Use the interface
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
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var listCmd, inputCmd, vpCmd tea.Cmd

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
	case itemSource:
		m.loading = false
		m.allItems = msg
		listItems := make([]list.Item, len(m.allItems))
		for i, it := range m.allItems {
			listItems[i] = it
		}
		cmds = append(cmds, m.list.SetItems(listItems))
		if len(m.list.Items()) > 0 {
			m.highlightedItem = m.list.SelectedItem().(item)
			m.loadingSecret = true
			cmds = append(cmds, fetchSecretData(m.clientset, m.highlightedItem.name, m.highlightedItem.namespace))
		}
		return m, tea.Batch(cmds...)
	case secretDataLoadedMsg:
		if m.highlightedItem.name == msg.secretName {
			m.loadingSecret = false
			m.secretCache[msg.secretName] = msg.data
			delete(m.secretErrCache, msg.secretName)
			m.viewport.SetContent(m.formatSecretData(msg.data))
			m.viewport.GotoTop()
		}
		return m, nil
	case secretDataErrorMsg:
		if m.highlightedItem.name == msg.secretName {
			m.loadingSecret = false
			m.secretErrCache[msg.secretName] = msg.err
		}
		return m, nil
	case fatalErrorMsg:
		m.err = msg.err
		m.loading = false
		m.loadingSecret = false
		return m, tea.Quit
	}

	if m.loading {
		return m, tea.Batch(cmds...)
	}

	if m.focus == leftPane {
		m.textinput, inputCmd = m.textinput.Update(msg)
		cmds = append(cmds, inputCmd)
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
		if selected, ok := m.list.SelectedItem().(item); ok && m.highlightedItem.name != selected.name {
			m.highlightedItem = selected
			if _, found := m.secretCache[selected.name]; !found {
				m.loadingSecret = true
				cmds = append(cmds, fetchSecretData(m.clientset, selected.name, selected.namespace))
			}
		}
	} else {
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
	}

	return m, tea.Batch(cmds...)
}

// --- VIEW ---
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
		Long:  `kds is a CLI tool for browsing, finding, and viewing Kubernetes secrets.`,
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

func viewSecretDataDirectly(clientset k8sClient, secretName, namespace string) error { // MODIFIED: Use the interface
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