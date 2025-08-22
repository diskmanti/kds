# kds - Kubernetes Decode Secret

[![Go Report Card](https://goreportcard.com/badge/github.com/diskmanti/kds)](https://goreportcard.com/report/github.com/diskmanti/kds)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/github/release/diskmanti/kds.svg)](https://github.com/diskmanti/kds/releases/latest)

A blazing fast, interactive TUI for viewing Kubernetes Secrets.

`kds` provides a beautiful and efficient terminal user interface to instantly browse, search, and view decrypted data from your Kubernetes secrets, replacing the tedious `kubectl get secret ... -o yaml` workflow.

![kds screenshot](https://user-images.githubusercontent.com/12345/123456789-abcdef.gif)
*(Note: This is a placeholder for a GIF. You would need to record one and upload it to GitHub.)*

---

## Features

-   **Live Decryption Pane**: A dual-pane layout shows your secrets on the left and the decrypted data for the selected secret on the right.
-   **Fuzzy Finding**: Instantly search and filter secrets by typing, just like `fzf`.
-   **Performance First**: Secret data is cached in memory for snappy navigation between previously viewed secrets.
-   **Graceful Error Handling**: A failure to fetch one secret won't crash the UI. Errors are displayed inline, allowing you to continue browsing.
-   **Responsive TUI**:
    -   **Independent Scrolling**: Scroll long secret values in the right pane without affecting the secret list.
    -   **Word Wrapping**: Long, single-line secret values are automatically wrapped to fit the pane.
    -   **Pane Navigation**: Easily switch focus between the secret list and the data view with `Tab`.
-   **Standard CLI Fallback**: Use `kds <secret-name>` for a non-interactive, direct print of a secret's decrypted data.
-   **Context-Aware**: Automatically uses the namespace from your current `kubeconfig` context, which can be overridden with a flag.

## Installation

#### Homebrew (macOS & Linux)

This is the recommended method for macOS and Linux users.

```sh
brew tap diskmanti/tap
brew install kds
```

#### GO 

```bash
go install github.com/diskmanti/kds@latest
```

#### Binaries

Pre-compiled binaries for various architectures are available on the GitHub Releases page.

1. Download the appropriate binary for your system (kds_..._amd64.tar.gz, kds_..._arm64.tar.gz, etc.).

1. Extract the archive.

1. Move the kds binary to a directory in your $PATH, like /usr/local/bin.

```bash
# Example for Linux amd64
wget https://github.com/diskmanti/kds/releases/download/v1.0.0/kds_1.0.0_linux_amd64.tar.gz
tar -xvf kds_1.0.0_linux_amd64.tar.gz
sudo mv kds /usr/local/bin/
```


## Usage

Simply run kds to launch the interactive terminal UI.

```bash
kds
```

#### Flags

- -n, --namespace <namespace>: Specify a namespace to view secrets from. If not provided, kds will use the namespace from your current kubeconfig context.
- --kubeconfig <path>: Use a specific kubeconfig file

#### TUI Controls

```
Key(s)	Action
↑ / ↓ / k / j	Navigate the secret list or scroll the data view

Tab	Switch focus between the secret list and data view

q / esc / Ctrl+C	Quit the application

(any other key)	Type to fuzzy find secrets

```

#### Non-Interactive Mode

To view a single secret and exit immediately, pass its name as an argument.

```bash
# View the 'my-api-key' secret in the current namespace
kds my-api-key

# View a secret in a specific namespace
kds my-db-credentials -n production
```

## Building from Source

1. If you'd like to build kds from source, you'll need Go 1.18 or later.

    ```bash
    git clone https://github.com/diskmanti/kds.git
    cd kds
    ```

2. Build the binary. For a proper build with version information, use ldflags to inject the values:

    ```bash
    IGNORE_WHEN_COPYING_START
    IGNORE_WHEN_COPYING_END

        
    go build -ldflags "-s -w \
    -X 'main.version=$(git describe --tags --abbrev=0)' \
    -X 'main.commit=$(git rev-parse HEAD)' \
    -X 'main.date=$(date -u +'%Y-%m-%dT%H:%M:%SZ')'" \
    -o kds .
    ```

3. Run your local build:

```bash
./kds
```

## Acknowledgments

This tool stands on the shoulders of giants. A huge thank you to the creators and maintainers of these incredible libraries:

- Charm: For their amazing ecosystem of TUI libraries, including Bubble Tea, Lipgloss, and many more.

- Cobra: For making modern CLI applications in Go a breeze.

- client-go: The official Go client for the Kubernetes API.

- fuzzy: For the excellent fuzzy-finding algorithm.

  

  
  