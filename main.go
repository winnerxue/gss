package main

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

type SSHKeyPairConfig struct {
	Name           string                 `json:"name"`
	PrivateKeyPath string                 `json:"private_key_path"`
	PublicKeyPath  string                 `json:"public_key_path"`
	SSHConfig      string                 `json:"ssh_config"` // Path to SSH config file
	GitConfig      map[string]interface{} `json:"git_config"` // Git-specific configuration
}

type SSHConfig struct {
	Keys       []SSHKeyPairConfig `json:"keys"`
	ActiveKey  int                `json:"active_key"`
	ConfigPath string             `json:"-"` // Not serialized
	SSHConfig  string             `json:"-"` // Path to ~/.ssh/config
}

func getHomeDir() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("USERPROFILE")
	}
	return os.Getenv("HOME")
}

func main() {
	config := &SSHConfig{
		ConfigPath: filepath.Join(getHomeDir(), ".gss"),
		SSHConfig:  filepath.Join(getHomeDir(), ".ssh", "config"),
	}

	if err := os.MkdirAll(config.ConfigPath, 0700); err != nil {
		fmt.Printf("Failed to create config directory: %v\n", err)
		os.Exit(1)
	}

	config.loadConfig()
	defer config.saveConfig()

	// Check for a subcommand
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	// Directly handle top-level commands
	switch os.Args[1] {
	case "generate", "gen":
		generateCmd(config)
	case "import", "i":
		importCmd(config)
	case "list", "ls":
		listCmd(config)
	case "switch", "s":
		switchCmd(config)
	case "delete", "del":
		deleteCmd(config)
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Printf("Usage: %s <command> [options]\n", os.Args[0])
	fmt.Println("Commands:")
	fmt.Println("  generate, gen   Generate a new SSH key pair")
	fmt.Println("  import, i       Import an existing SSH key pair")
	fmt.Println("  list, ls        List all SSH key pairs")
	fmt.Println("  switch, s       Switch to an SSH key pair by index, or choose interactively")
	fmt.Println("  delete, del     Delete an SSH key pair entry from config by index, or choose interactively")
	fmt.Println("\nUse '<command> -h' for more information on a command.")
}

// Command implementations
func generateCmd(config *SSHConfig) {
	generateCmd := flag.NewFlagSet("generate", flag.ExitOnError)
	name := generateCmd.String("g", "", "Generate a new SSH key pair with given name (RSA)")
	generateCmd.Parse(os.Args[2:])

	if *name == "" {
		generateCmd.Usage()
		os.Exit(1)
	}

	config.generateKeyPair(*name)
}

func importCmd(config *SSHConfig) {
	importCmd := flag.NewFlagSet("import", flag.ExitOnError)
	privPath := importCmd.String("i", "", "Import private key path")
	pubPath := importCmd.String("p", "", "Import public key path")
	name := importCmd.String("n", "", "Name for imported key pair")
	sshConfigPath := importCmd.String("c", "", "Path to SSH config file for imported key pair")
	email := importCmd.String("git-email", "", "Git user email")
	nameGit := importCmd.String("git-name", "", "Git user name")
	importCmd.Parse(os.Args[2:])

	if *privPath == "" || *pubPath == "" || *email == "" || *nameGit == "" || *name == "" {
		importCmd.Usage()
		os.Exit(1)
	}
	// Convert to absolute paths
	absPrivPath, err := filepath.Abs(*privPath)
	if err != nil {
		fmt.Printf("Failed to convert private key path to absolute: %v\n", err)
		os.Exit(1)
	}
	absPubPath, err := filepath.Abs(*pubPath)
	if err != nil {
		fmt.Printf("Failed to convert public key path to absolute: %v\n", err)
		os.Exit(1)
	}
	var absSSHConfigPath string
	if *sshConfigPath != "" {
		absSSHConfigPath, err = filepath.Abs(*sshConfigPath)
		if err != nil {
			fmt.Printf("Failed to convert SSH config path to absolute: %v\n", err)
			os.Exit(1)
		}
	}

	// Initialize git_config
	gitConfig := make(map[string]interface{})
	if *email != "" {
		gitConfig["user.email"] = *email
	}
	if *nameGit != "" {
		gitConfig["user.name"] = *nameGit
	}

	config.importKeyPair(absPrivPath, absPubPath, *name, absSSHConfigPath, gitConfig)
}

func listCmd(config *SSHConfig) {
	listCmd := flag.NewFlagSet("list", flag.ExitOnError)
	listCmd.Parse(os.Args[2:])
	config.listKeyPairs()
}

func switchCmd(config *SSHConfig) {
	switchCmd := flag.NewFlagSet("switch", flag.ExitOnError)
	// We use an explicit bool flag to check if -i was provided,
	// because `flag.Int` returns a default value if not provided.
	indexProvided := false
	indexVal := switchCmd.Int("i", -1, "Index of SSH key pair to switch to")
	switchCmd.Visit(func(f *flag.Flag) {
		if f.Name == "i" {
			indexProvided = true
		}
	})

	scope := switchCmd.String("s", "global", "Git configuration scope (global or local). Default is global.")
	switchCmd.Parse(os.Args[2:])

	var chosenIndex int
	if indexProvided {
		chosenIndex = *indexVal
	} else {
		// No -i provided, enter interactive mode
		if len(config.Keys) == 0 {
			fmt.Println("No key pairs found to switch to. Generate or import one first.")
			os.Exit(1)
		}

		fmt.Println("\n--- Available SSH Key Pairs for Switching ---")
		config.listKeyPairs() // Show the list
		fmt.Print("Enter the index of the key pair to switch to: ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		parsedIndex, err := strconv.Atoi(input)
		if err != nil {
			fmt.Printf("Invalid input. Please enter a number.\n")
			os.Exit(1)
		}
		chosenIndex = parsedIndex
	}

	if chosenIndex < 0 || chosenIndex >= len(config.Keys) {
		fmt.Printf("Invalid index: %d (available: 0 to %d)\n", chosenIndex, len(config.Keys)-1)
		os.Exit(1)
	}

	config.ActiveKey = chosenIndex
	key := config.Keys[chosenIndex]

	// Adjust private key permissions for Linux/Unix
	privKeyPath := key.PrivateKeyPath
	if _, err := os.Stat(privKeyPath); os.IsNotExist(err) {
		fmt.Printf("Private key not found at: %s\n", privKeyPath)
		os.Exit(1)
	}

	if runtime.GOOS != "windows" { // Linux/Unix
		fmt.Printf("Adjusting permissions for private key (Linux/Unix): %s\n", privKeyPath)
		if err := os.Chmod(privKeyPath, 0600); err != nil {
			fmt.Printf("Failed to set private key permissions to 0600: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Private key permissions set to 0600 on Linux/Unix.")
	}

	// SSH configuration
	if err := os.MkdirAll(filepath.Dir(config.SSHConfig), 0700); err != nil {
		fmt.Printf("Failed to create SSH config directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Using IdentityFile: %s\n", key.PrivateKeyPath)

	// Predefined configuration with dynamic IdentityFile
	predefinedConfig := fmt.Sprintf("IdentityFile %s", key.PrivateKeyPath)

	var configContent string
	if key.SSHConfig != "" {
		absSSHConfigPath, err := filepath.Abs(key.SSHConfig)
		if err != nil {
			fmt.Printf("Failed to convert SSH config path to absolute: %v\n", err)
			os.Exit(1)
		}
		if _, err := os.Stat(absSSHConfigPath); os.IsNotExist(err) {
			fmt.Printf("SSH config file not found: %s\n", absSSHConfigPath)
			os.Exit(1)
		}
		configData, err := os.ReadFile(absSSHConfigPath)
		if err != nil {
			fmt.Printf("Failed to read SSH config file: %v\n", err)
			os.Exit(1)
		}
		configContent = predefinedConfig + "\n" + string(configData)
	} else {
		configContent = predefinedConfig
	}

	if err := os.WriteFile(config.SSHConfig, []byte(configContent), 0600); err != nil {
		fmt.Printf("Failed to update SSH config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("SSH key switched to: %s (%s)\n", key.Name, key.PrivateKeyPath)

	// Git configuration
	var gitConfigFile string
	switch strings.ToLower(*scope) {
	case "local":
		gitConfigFile = filepath.Join(".", ".git", "config")
		if _, err := os.Stat(gitConfigFile); os.IsNotExist(err) {
			fmt.Printf("Local Git config not found at %s. Ensure you are in a Git repository if using local scope.\n", gitConfigFile)
		}
	case "global":
		gitConfigFile = filepath.Join(getHomeDir(), ".gitconfig")
	default:
		fmt.Printf("Invalid scope: %s. Using 'global' Git configuration.\n", *scope)
		gitConfigFile = filepath.Join(getHomeDir(), ".gitconfig")
	}

	if key.GitConfig == nil {
		fmt.Println("No Git configuration defined for this key pair. Skipping Git config update.")
	} else {
		// Ensure core.sshCommand is set to use the GSS-managed SSH config
		key.GitConfig["core.sshCommand"] = fmt.Sprintf("ssh -F %s", strings.ReplaceAll(config.SSHConfig, "\\", "/"))

		// Apply git_config to the specified config file
		for gitKey, gitValue := range key.GitConfig {
			switch v := gitValue.(type) {
			case string:
				cmdArgs := []string{"config"}
				if *scope == "global" {
					cmdArgs = append(cmdArgs, "--global")
				} else { // "local" scope
					cmdArgs = append(cmdArgs, "--file", gitConfigFile)
				}
				cmdArgs = append(cmdArgs, gitKey, v)

				cmd := exec.Command("git", cmdArgs...)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					fmt.Printf("Failed to set Git config '%s' in %s scope: %v\n", gitKey, *scope, err)
				} else {
					fmt.Printf("Git config '%s' set to '%s' in %s scope.\n", gitKey, v, *scope)
				}
			default:
				fmt.Printf("Unsupported Git config value type for '%s': %T. Skipping.\n", gitKey, gitValue)
			}
		}
		fmt.Printf("Git configuration updated for key pair: %s in %s scope.\n", key.Name, *scope)
	}

	fmt.Printf("Successfully switched to key pair: %s (Index: %d).\n", key.Name, chosenIndex)

	// Windows icacls instructions moved to the end
	if runtime.GOOS == "windows" {
		currentUser := os.Getenv("USERNAME")
		if currentUser == "" {
			currentUser = "YOUR_USERNAME" // Placeholder if username not found
		}

		fmt.Printf("\n--- Important: Manual Permissions Adjustment Required (Windows) ---\n")
		fmt.Printf("Please run the following commands in an **Administrator Command Prompt (CMD)**:\n\n")

		fmt.Printf(":: For your Private Key: %s\n", privKeyPath)
		fmt.Printf("icacls \"%s\" /reset\n", privKeyPath)
		fmt.Printf("icacls \"%s\" /grant:r \"%s\":F\n", privKeyPath, currentUser)
		fmt.Printf("icacls \"%s\" /inheritance:r\n\n", privKeyPath)

		sshConfigFilePath := config.SSHConfig
		fmt.Printf(":: For your SSH Config File: %s\n", sshConfigFilePath)
		fmt.Printf("icacls \"%s\" /reset\n", sshConfigFilePath)
		fmt.Printf("icacls \"%s\" /grant:r \"%s\":F\n", sshConfigFilePath, currentUser)
		fmt.Printf("icacls \"%s\" /inheritance:r\n\n", sshConfigFilePath)

		fmt.Printf("After running these commands, press Enter to continue...\n")
		bufio.NewReader(os.Stdin).ReadBytes('\n') // Wait for user to press Enter
	}
}

func deleteCmd(config *SSHConfig) {
	deleteCmd := flag.NewFlagSet("delete", flag.ExitOnError)
	indexProvided := false
	indexVal := deleteCmd.Int("i", -1, "Index of SSH key pair to delete from config")
	force := deleteCmd.Bool("f", false, "Force deletion without confirmation")
	deleteCmd.Visit(func(f *flag.Flag) {
		if f.Name == "i" {
			indexProvided = true
		}
	})

	deleteCmd.Parse(os.Args[2:])

	if len(config.Keys) == 0 {
		fmt.Println("No key pairs found to delete. Generate or import one first.")
		os.Exit(1)
	}

	var chosenIndex int
	if indexProvided {
		chosenIndex = *indexVal
	} else {
		// Interactive mode
		fmt.Println("\n--- Available SSH Key Pairs for Deletion ---")
		config.listKeyPairs()
		fmt.Print("Enter the index of the key pair to delete: ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		parsedIndex, err := strconv.Atoi(input)
		if err != nil {
			fmt.Printf("Invalid input. Please enter a number.\n")
			os.Exit(1)
		}
		chosenIndex = parsedIndex
	}

	if chosenIndex < 0 || chosenIndex >= len(config.Keys) {
		fmt.Printf("Invalid index: %d (available: 0 to %d)\n", chosenIndex, len(config.Keys)-1)
		os.Exit(1)
	}

	key := config.Keys[chosenIndex]
	fmt.Printf("\nSelected key pair for deletion from config:\n")
	fmt.Printf("  Name: %s\n", key.Name)
	fmt.Printf("  Private Key: %s\n", key.PrivateKeyPath)
	fmt.Printf("  Public Key: %s\n", key.PublicKeyPath)
	if key.SSHConfig != "" {
		fmt.Printf("  SSH Config: %s\n", key.SSHConfig)
	}

	if !*force {
		fmt.Print("\nAre you sure you want to delete this key pair entry from config? (Files will not be deleted) (y/N): ")
		reader := bufio.NewReader(os.Stdin)
		confirmation, _ := reader.ReadString('\n')
		confirmation = strings.TrimSpace(strings.ToLower(confirmation))
		if confirmation != "y" && confirmation != "yes" {
			fmt.Println("Deletion cancelled.")
			os.Exit(0)
		}
	}

	// Remove from config
	config.Keys = append(config.Keys[:chosenIndex], config.Keys[chosenIndex+1:]...)

	// Adjust ActiveKey if necessary
	if config.ActiveKey == chosenIndex {
		config.ActiveKey = -1
		if len(config.Keys) > 0 {
			config.ActiveKey = 0
			fmt.Printf("Active key was deleted. Switched to key pair: %s (Index: 0)\n", config.Keys[0].Name)
		} else {
			fmt.Println("No key pairs remaining. Active key unset.")
			// Clear SSH config if no keys remain
			if err := os.WriteFile(config.SSHConfig, []byte(""), 0600); err != nil {
				fmt.Printf("Failed to clear SSH config: %v\n", err)
			}
		}
	} else if config.ActiveKey > chosenIndex {
		config.ActiveKey--
	}

	fmt.Printf("Successfully deleted key pair entry: %s (Index: %d) from config\n", key.Name, chosenIndex)
}

func (c *SSHConfig) loadConfig() {
	configFile := filepath.Join(c.ConfigPath, "config.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			c.Keys = []SSHKeyPairConfig{}
			return
		}
		fmt.Printf("Failed to read config file: %v\n", err)
		os.Exit(1)
	}

	if err := json.Unmarshal(data, c); err != nil {
		fmt.Printf("Failed to parse config file: %v\n", err)
		os.Exit(1)
	}
}

func (c *SSHConfig) saveConfig() {
	configFile := filepath.Join(c.ConfigPath, "config.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		fmt.Printf("Failed to marshal config: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(configFile, data, 0600); err != nil {
		fmt.Printf("Failed to write config file: %v\n", err)
		os.Exit(1)
	}
}

func (c *SSHConfig) generateKeyPair(name string) {
	if name == "" {
		name = "id_rsa"
	}

	privPath, pubPath := getUniqueFilePaths(c.ConfigPath, name)

	// Convert to absolute paths
	absPrivPath, err := filepath.Abs(privPath)
	if err != nil {
		fmt.Printf("Failed to convert private key path to absolute: %v\n", err)
		os.Exit(1)
	}
	absPubPath, err := filepath.Abs(pubPath)
	if err != nil {
		fmt.Printf("Failed to convert public key path to absolute: %v\n", err)
		os.Exit(1)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fmt.Printf("Failed to generate private key: %v\n", err)
		os.Exit(1)
	}

	privFile, err := os.OpenFile(absPrivPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		fmt.Printf("Failed to create private key file: %v\n", err)
		os.Exit(1)
	}
	defer privFile.Close()

	privPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	if err := pem.Encode(privFile, privPEM); err != nil {
		fmt.Printf("Failed to write private key: %v\n", err)
		os.Exit(1)
	}

	pubKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		fmt.Printf("Failed to generate public key: %v\n", err)
		os.Exit(1)
	}
	pubBytes := ssh.MarshalAuthorizedKey(pubKey)

	pubFile, err := os.OpenFile(absPubPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Printf("Failed to create public key file: %v\n", err)
		os.Exit(1)
	}
	defer pubFile.Close()

	if _, err := pubFile.Write(pubBytes); err != nil {
		fmt.Printf("Failed to write public key: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated: %s (%s, %s)\n", name, absPrivPath, absPubPath)
	c.Keys = append(c.Keys, SSHKeyPairConfig{
		Name:           name,
		PrivateKeyPath: absPrivPath,
		PublicKeyPath:  absPubPath,
		SSHConfig:      "",
		GitConfig:      make(map[string]interface{}),
	})
}

func (c *SSHConfig) importKeyPair(privPath, pubPath, name, sshConfigPath string, gitConfig map[string]interface{}) {
	if _, err := os.Stat(privPath); os.IsNotExist(err) {
		fmt.Printf("Private key not found: %s\n", privPath)
		os.Exit(1)
	}
	if _, err := os.Stat(pubPath); os.IsNotExist(err) {
		fmt.Printf("Public key not found: %s\n", pubPath)
		os.Exit(1)
	}

	// Read private key to validate
	privData, err := os.ReadFile(privPath)
	if err != nil {
		fmt.Printf("Failed to read private key: %v\n", err)
		os.Exit(1)
	}

	// Parse PEM
	privBlock, _ := pem.Decode(privData)
	if privBlock == nil {
		fmt.Println("Invalid private key format")
		os.Exit(1)
	}

	// Validate SSH config path if provided
	var absSSHConfigPath string
	if sshConfigPath != "" {
		if _, err := os.Stat(sshConfigPath); os.IsNotExist(err) {
			fmt.Printf("SSH config file not found: %s\n", sshConfigPath)
			os.Exit(1)
		}
		absSSHConfigPath = sshConfigPath
	}

	fmt.Printf("Imported: %s (%s, %s, %s)\n", name, privPath, pubPath, absSSHConfigPath)
	c.Keys = append(c.Keys, SSHKeyPairConfig{
		Name:           name,
		PrivateKeyPath: privPath,
		PublicKeyPath:  pubPath,
		SSHConfig:      absSSHConfigPath,
		GitConfig:      gitConfig,
	})
}

func (c *SSHConfig) listKeyPairs() {
	if len(c.Keys) == 0 {
		fmt.Println("No key pairs found. Generate or import one first.")
		return
	}

	fmt.Println("--- SSH Key Pairs ---")
	for i, key := range c.Keys {
		activeStatus := " "
		if i == c.ActiveKey {
			activeStatus = "✅"
		}

		sshConfigPath := key.SSHConfig
		if sshConfigPath == "" {
			sshConfigPath = "N/A"
		}

		fmt.Printf("\n%s Index: %d\n", activeStatus, i)
		fmt.Printf("  Name: %s\n", key.Name)
		fmt.Printf("  Private Key: %s\n", key.PrivateKeyPath)
		fmt.Printf("  Public Key: %s\n", key.PublicKeyPath)
		fmt.Printf("  SSH Config: %s\n", sshConfigPath)

		if len(key.GitConfig) > 0 {
			fmt.Println("  Git Config:")
			for gitKey, gitValue := range key.GitConfig {
				fmt.Printf("    - %s: %v\n", gitKey, gitValue)
			}
		}
		fmt.Println("--------------------")
	}
}

func copyFile(src, dst string, perm os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func getUniqueFilePaths(configPath, baseName string) (privPath, pubPath string) {
	privPath = filepath.Join(configPath, baseName+".key")
	pubPath = privPath + ".pub"

	for i := 1; ; i++ {
		if _, err := os.Stat(privPath); os.IsNotExist(err) {
			break
		}
		privPath = filepath.Join(configPath, baseName+"_"+strconv.Itoa(i)+".key")
		pubPath = privPath + ".pub"
	}

	return privPath, pubPath
}
