-----

# SSH and Git Key Switcher (GSS)

GSS (Git/SSH Switcher) is a command-line tool written in Go that simplifies managing multiple SSH key pairs and their corresponding Git configurations. It allows you to generate, import, list, switch, and delete SSH key pairs, automatically updating your `~/.ssh/config` and Git settings (`user.name`, `user.email`, `core.sshCommand`) for seamless transitions between different identities.

-----

## ✨ Features

  * **Generate SSH Key Pairs**: Quickly create new RSA SSH key pairs.
  * **Import Existing Keys**: Add your existing SSH private and public keys to GSS for management.
  * **List Key Pairs**: View all managed SSH key pairs with their details, including active status.
  * **Switch Key Pairs**: Effortlessly switch your active SSH key by updating `~/.ssh/config`. This also updates your Git user name, email, and SSH command to match the selected key's configuration. Supports both global and local Git scope.
  * **Delete Key Pair Entries**: Remove SSH key pair entries from GSS configuration. (Note: This **does not** delete the actual key files.)
  * **Persistent Configuration**: GSS stores your key pair configurations in `~/.gss/config.json`, ensuring your settings are saved across sessions.

-----

## 🚀 Getting Started

### Prerequisites

  * Go (Golang) environment (to build from source)
  * Git installed on your system

### Installation

To install GSS, you can build it from source:

1.  **Clone the repository**:

    ```bash
    git clone https://github.com:winnerxue/gss.git
    cd gss
    ```

    (Replace `https://github.com:winnerxue/gss.git` with the actual link to your repository.)

2.  **Build the executable**:

    ```bash
    go build -o gss .
    ```

3.  **Move the executable to your PATH**:

    ```bash
    sudo mv gss /usr/local/bin/
    ```

    This makes the `gss` command available globally.

-----

## 📖 Usage

GSS uses a subcommand-based interface.

```bash
gss <command> [options]
```

### Commands

  * `generate`, `gen`: Generate a new SSH key pair.
  * `import`, `i`: Import an existing SSH key pair.
  * `list`, `ls`: List all managed SSH key pairs.
  * `switch`, `s`: Switch to a specific SSH key pair.
  * `delete`, `del`: Delete an SSH key pair entry from the configuration.

-----

### Command Details

#### `gss generate | gen -g <name>`

Generates a new RSA SSH key pair with the specified name. The keys will be stored in `~/.gss/`.

  * `-g <name>`: **Required**. The name for the new key pair (e.g., `work_key`, `personal_key`).

**Example:**

```bash
gss gen -g my_new_key
```

#### `gss import | i -i <private_key_path> -p <public_key_path> -n <name> -git-email <email> -git-name <git_user_name> [-c <ssh_config_path>]`

Imports an existing SSH key pair into GSS. You must provide paths to both the private and public keys, a name for the entry, and associated Git user details.

  * `-i <private_key_path>`: **Required**. Path to your existing private SSH key.
  * `-p <public_key_path>`: **Required**. Path to your existing public SSH key.
  * `-n <name>`: **Required**. A unique name to identify this key pair in GSS (e.g., `github_personal`).
  * `-git-email <email>`: **Required**. The Git user email associated with this key.
  * `-git-name <git_user_name>`: **Required**. The Git user name associated with this key.
  * `-c <ssh_config_path>`: **Optional**. Path to an additional SSH config file specific to this key (e.g., `~/.ssh/config_github`). This content will be appended to your main `~/.ssh/config` when this key is active.

**Example:**

```bash
gss import -i ~/.ssh/id_rsa_github -p ~/.ssh/id_rsa_github.pub -n github_work -git-email "work@example.com" -git-name "Work User" -c ~/.ssh/config_github
```

#### `gss list | ls`

Lists all SSH key pairs currently managed by GSS, along with their associated paths and whether they are currently active.

**Example:**

```bash
gss ls
```

#### `gss switch | s [-i <index>] [-s <scope>]`

Switches the active SSH key pair.

  * `-i <index>`: **Optional**. The numerical index of the key pair to switch to (as shown by `gss list`). If not provided, GSS will enter an interactive mode, prompting you to select from the list.
  * `-s <scope>`: **Optional**. Specifies the scope for Git configuration updates. Can be `global` (default) or `local`.
      * `global`: Updates the `~/.gitconfig` file.
      * `local`: Updates the `.git/config` file in the current repository. If using `local` scope, ensure you are in a Git repository.

When switching, GSS:

1.  Updates `~/.ssh/config` to use the selected key's private key.
2.  Sets `core.sshCommand` in your Git configuration to point to the GSS-managed `~/.ssh/config`.
3.  Updates `user.name` and `user.email` in your Git configuration based on the imported key's settings.

**Examples:**

```bash
gss s -i 0             # Switch to the key at index 0 (global Git scope)
gss s                  # Interactive mode to select key
gss s -i 1 -s local    # Switch to key at index 1 and update local Git config
```

#### `gss delete | del [-i <index>] [-f]`

Deletes an SSH key pair entry from the GSS configuration. **This command only removes the entry from GSS's management; it does NOT delete the actual private and public key files from your system.**

  * `-i <index>`: **Optional**. The numerical index of the key pair entry to delete. If not provided, GSS will enter an interactive mode, prompting you to select from the list.
  * `-f`: **Optional**. Force deletion without confirmation prompt.

**Examples:**

```bash
gss del -i 2           # Delete the key entry at index 2
gss del                # Interactive mode to select key for deletion
gss del -i 0 -f        # Force delete the key entry at index 0 without confirmation
```

-----

## ⚙️ Configuration

GSS stores its configuration in a JSON file located at `~/.gss/config.json`. You typically won't need to edit this file directly, as all operations are managed via the `gss` command-line tool.

-----

## 🤝 Contributing

Contributions are welcome\! If you find a bug or have a feature request, please open an issue.

-----