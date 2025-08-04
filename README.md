# kds - Kubernetes Decode Secret

[![Release kds](https://github.com/diskmanti/kds/actions/workflows/release.yml/badge.svg)](https://github.com/diskmanti/kds/actions/workflows/release.yml)


[![Go Report Card](https://goreportcard.com/badge/github.com/diskmanti/kds)](https://goreportcard.com/report/github.com/diskmanti/kds)

kds is a modern, beautiful, and fast command-line tool for viewing the data within Kubernetes Secrets. It provides a rich terminal user interface with fuzzy-finding to make searching for and decoding secrets effortless.

> Tired of the cumbersome kubectl get secret <name> -o yaml | grep ... | base64 -d workflow? kds is your new best friend.


# Installation

## From GitHub Releases (Recommended)

You can download the latest pre-built binary for your operating system from the Releases page.

```
curl -sL "https://github.com/diskmanti/kds/releases/download/${VERSION}/kds_${VERSION#v}_${OS}_${ARCH}.tar.gz" | tar -xz kds
```

### Move the binary to your PATH
sudo install -m 755 kds /usr/local/bin/kds


## From Source

If you have Go installed (version 1.21+):

```      
git clone https://github.com/diskmanti/kds.git
cd kds
go install .
```

## Keybindings:

    Type to start fuzzy-finding.

    ↑/↓ to navigate the list.

    Enter to select a secret and view its data.

    Ctrl+C / q / Esc to quit.

Non-Interactive (Direct) Mode

If you know the name of the secret, you can pass it as an argument to print its contents directly. This is useful for scripting.
Generated bash

      
## View a secret in the current context's namespace
kds my-secret-name

## View a secret in a specific namespace
kds my-secret-name --namespace my-app

## Use a non-default kubeconfig
kds my-secret --kubeconfig /path/to/other/config
      
$ kds --help
kds (Kubernetes Decode Secret) is a CLI tool with a rich terminal UI
for browsing, finding, and viewing Kubernetes secrets.

```
Usage:
  kds [secret-name] [flags]

Flags:
  -h, --help              help for kds
      --kubeconfig string   (optional) path to kubeconfig (default "/home/user/.kube/config")
  -n, --namespace string    namespace (overrides context)
      --version           version for kds
```