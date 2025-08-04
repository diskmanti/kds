kds - Kubernetes Decode Secret

![alt text](https://img.shields.io/github/v/release/diskmanti/kds?style=for-the-badge)


![alt text](https://img.shields.io/github/actions/workflow/status/diskmanti/kds/release.yml?branch=main&style=for-the-badge)


![alt text](https://goreportcard.com/badge/github.com/diskmanti/kds?style=for-the-badge)


![alt text](https://img.shields.io/badge/License-MIT-blue.svg?style=for-the-badge)

kds is a modern, beautiful, and fast command-line tool for viewing the data within Kubernetes Secrets. It provides a rich terminal user interface with fuzzy-finding to make searching for and decoding secrets effortless.

Tired of the cumbersome kubectl get secret <name> -o yaml | grep ... | base64 -d workflow? kds is your new best friend.
Demo

![alt text](https://raw.githubusercontent.com/diskmanti/kds/main/demo.gif)

(To create a GIF like this, you can use tools like vhs or asciinema.)
Features

    Interactive TUI: A beautiful terminal interface powered by Charmbracelet's Bubble Tea.

    Fuzzy Finding: Instantly find the secret you're looking for, even with thousands of secrets in a namespace.

    Smart Decoding: Automatically handles both Base64-encoded and raw string data stored in secrets.

    Standalone & Portable: A single, dependency-free binary for Linux, macOS, and Windows.

    Non-Interactive Mode: Pipe-friendly for use in scripts and automation.

    Standard Kubeconfig Discovery: Works out-of-the-box by respecting your KUBECONFIG environment variable and ~/.kube/config file, just like kubectl.

Installation
From GitHub Releases (Recommended)

You can download the latest pre-built binary for your operating system from the Releases page.

For macOS and Linux (using Homebrew):
Generated bash

      
brew tap diskmanti/kds
brew install kds

    

IGNORE_WHEN_COPYING_START
Use code with caution. Bash
IGNORE_WHEN_COPYING_END

(Note: This requires setting up a Homebrew tap, which GoReleaser can automate with its Pro version or with some additional configuration.)

For macOS and Linux (manual):
Generated bash

      
# Replace v0.1.0 with the desired version
VERSION="v0.1.0"
# Adjust for your OS (linux/darwin) and architecture (amd64/arm64)
ARCH="amd64"
OS="linux"

curl -sL "https://github.com/diskmanti/kds/releases/download/${VERSION}/kds_${VERSION#v}_${OS}_${ARCH}.tar.gz" | tar -xz kds

# Move the binary to your PATH
sudo install -m 755 kds /usr/local/bin/kds

    

IGNORE_WHEN_COPYING_START
Use code with caution. Bash
IGNORE_WHEN_COPYING_END

For Windows:

    Download the .zip file for your architecture from the Releases page.

    Unzip the archive.

    Place the kds.exe file in a directory that is included in your system's PATH.

From Source

If you have Go installed (version 1.21+):
Generated bash

      
git clone https://github.com/diskmanti/kds.git
cd kds
go install .

    

IGNORE_WHEN_COPYING_START
Use code with caution. Bash
IGNORE_WHEN_COPYING_END
Usage
Interactive Mode

Simply run kds without any arguments to launch the interactive fuzzy-finder.
Generated bash

      
kds

    

IGNORE_WHEN_COPYING_START
Use code with caution. Bash
IGNORE_WHEN_COPYING_END

To search within a specific namespace:
Generated bash

      
kds -n production

    

IGNORE_WHEN_COPYING_START
Use code with caution. Bash
IGNORE_WHEN_COPYING_END

Keybindings:

    Type to start fuzzy-finding.

    ↑/↓ to navigate the list.

    Enter to select a secret and view its data.

    Ctrl+C / q / Esc to quit.

Non-Interactive (Direct) Mode

If you know the name of the secret, you can pass it as an argument to print its contents directly. This is useful for scripting.
Generated bash

      
# View a secret in the current context's namespace
kds my-secret-name

# View a secret in a specific namespace
kds my-secret-name --namespace my-app

# Use a non-default kubeconfig
kds my-secret --kubeconfig /path/to/other/config

    

IGNORE_WHEN_COPYING_START
Use code with caution. Bash
IGNORE_WHEN_COPYING_END
Flags
Generated text

      
$ kds --help
kds (Kubernetes Decode Secret) is a CLI tool with a rich terminal UI
for browsing, finding, and viewing Kubernetes secrets.

Usage:
  kds [secret-name] [flags]

Flags:
  -h, --help              help for kds
      --kubeconfig string   (optional) path to kubeconfig (default "/home/user/.kube/config")
  -n, --namespace string    namespace (overrides context)
      --version           version for kds

    

IGNORE_WHEN_COPYING_START
Use code with caution. Text
IGNORE_WHEN_COPYING_END
Building From Source

If you want to contribute or build the project yourself:

    Clone the repository:
    Generated bash

      
git clone https://github.com/diskmanti/kds.git
cd kds

    

IGNORE_WHEN_COPYING_START
Use code with caution. Bash
IGNORE_WHEN_COPYING_END

Ensure you have Go 1.21+ installed.

Tidy the modules:
Generated bash

      
go mod tidy

    

IGNORE_WHEN_COPYING_START
Use code with caution. Bash
IGNORE_WHEN_COPYING_END

Build the binary:
Generated bash

      
go build -o kds main.go

    

IGNORE_WHEN_COPYING_START

    Use code with caution. Bash
    IGNORE_WHEN_COPYING_END

Contributing

Contributions are welcome! If you have a feature request, bug report, or pull request, please feel free to open an issue or PR.
Acknowledgements

This tool is built on the shoulders of giants and wouldn't be possible without the amazing work by the teams behind:

    Charmbracelet (Bubble Tea, Lip Gloss, Bubbles)

    Kubernetes (client-go)

    Cobra

    GoReleaser

License

This project is licensed under the MIT License.