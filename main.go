package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/crypto/ssh"
)

type SSHKeyPair struct {
	Name           string `json:"name"`
	PrivateKeyPath string `json:"private_key_path"`
	PublicKeyPath  string `json:"public_key_path"`
}

type SSHConfig struct {
	Keys       []SSHKeyPair `json:"keys"`
	ActiveKey  int          `json:"active_key"`
	ConfigPath string       `json:"-"` // 不序列化
	SSHConfig  string       `json:"-"` // 不序列化
}

func main() {
	generate := flag.String("g", "", "Generate a new SSH key pair with given name (RSA)")
	importPriv := flag.String("i", "", "Import private key path")
	importName := flag.String("n", "", "Name for imported key pair (default: filename)")
	list := flag.Bool("l", false, "List all SSH key pairs")
	switchKey := flag.Int("s", -1, "Switch to SSH key pair by index")
	flag.Parse()

	config := &SSHConfig{
		ConfigPath: filepath.Join(os.Getenv("HOME"), ".gss"),
		SSHConfig:  filepath.Join(os.Getenv("HOME"), ".ssh", "config"),
	}

	if err := os.MkdirAll(config.ConfigPath, 0700); err != nil {
		fmt.Printf("Failed to create config directory: %v\n", err)
		os.Exit(1)
	}

	config.loadConfig()

	switch {
	case *generate != "":
		config.generateKeyPair(*generate)
	case *importPriv != "":
		name := *importName
		if name == "" {
			name = filepath.Base(*importPriv)
		}
		config.importKeyPair(*importPriv, name)
	case *list:
		config.listKeyPairs()
	case *switchKey >= 0:
		config.switchKeyPair(*switchKey)
	default:
		flag.Usage()
		os.Exit(1)
	}

	config.saveConfig()
}

func (c *SSHConfig) loadConfig() {
	configFile := filepath.Join(c.ConfigPath, "config.json")
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			c.Keys = []SSHKeyPair{}
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

	if err := ioutil.WriteFile(configFile, data, 0600); err != nil {
		fmt.Printf("Failed to write config file: %v\n", err)
		os.Exit(1)
	}
}

func (c *SSHConfig) generateKeyPair(name string) {
	if name == "" {
		name = "id_rsa"
	}

	privPath, pubPath := getUniqueFilePaths(c.ConfigPath, name)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fmt.Printf("Failed to generate private key: %v\n", err)
		os.Exit(1)
	}

	privFile, err := os.OpenFile(privPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
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

	pubFile, err := os.OpenFile(pubPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Printf("Failed to create public key file: %v\n", err)
		os.Exit(1)
	}
	defer pubFile.Close()

	if _, err := pubFile.Write(pubBytes); err != nil {
		fmt.Printf("Failed to write public key: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated: %s (%s, %s)\n", name, privPath, pubPath)
	c.Keys = append(c.Keys, SSHKeyPair{
		Name:           name,
		PrivateKeyPath: privPath,
		PublicKeyPath:  pubPath,
	})
}

func (c *SSHConfig) importKeyPair(privPath, name string) {
	if _, err := os.Stat(privPath); os.IsNotExist(err) {
		fmt.Printf("Private key not found: %s\n", privPath)
		os.Exit(1)
	}

	// 读取私钥
	privData, err := ioutil.ReadFile(privPath)
	if err != nil {
		fmt.Printf("Failed to read private key: %v\n", err)
		os.Exit(1)
	}

	// 解析 PEM
	privBlock, _ := pem.Decode(privData)
	if privBlock == nil {
		fmt.Println("Invalid private key format")
		os.Exit(1)
	}

	// 探测密钥类型并解析
	var pubKey ssh.PublicKey
	switch privBlock.Type {
	case "RSA PRIVATE KEY":
		privateKey, err := x509.ParsePKCS1PrivateKey(privBlock.Bytes)
		if err != nil {
			fmt.Printf("Failed to parse RSA private key: %v\n", err)
			os.Exit(1)
		}
		pubKey, err = ssh.NewPublicKey(&privateKey.PublicKey)
		if err != nil {
			fmt.Printf("Failed to generate RSA public key: %v\n", err)
			os.Exit(1)
		}
	case "EC PRIVATE KEY":
		privateKey, err := x509.ParseECPrivateKey(privBlock.Bytes)
		if err != nil {
			fmt.Printf("Failed to parse ECDSA private key: %v\n", err)
			os.Exit(1)
		}
		pubKey, err = ssh.NewPublicKey(&privateKey.PublicKey)
		if err != nil {
			fmt.Printf("Failed to generate ECDSA public key: %v\n", err)
			os.Exit(1)
		}
	case "ED25519 PRIVATE KEY", "OPENSSH PRIVATE KEY":
		privateKey, err := ssh.ParseRawPrivateKey(privData)
		if err != nil {
			fmt.Printf("Failed to parse private key: %v\n", err)
			os.Exit(1)
		}
		switch key := privateKey.(type) {
		case *rsa.PrivateKey:
			pubKey, err = ssh.NewPublicKey(&key.PublicKey)
			if err != nil {
				fmt.Printf("Failed to generate RSA public key: %v\n", err)
				os.Exit(1)
			}
		case *ecdsa.PrivateKey:
			pubKey, err = ssh.NewPublicKey(&key.PublicKey)
			if err != nil {
				fmt.Printf("Failed to generate ECDSA public key: %v\n", err)
				os.Exit(1)
			}
		case ed25519.PrivateKey:
			pubKey, err = ssh.NewPublicKey(key.Public())
			if err != nil {
				fmt.Printf("Failed to generate Ed25519 public key: %v\n", err)
				os.Exit(1)
			}
		default:
			fmt.Printf("Unsupported private key type: %T\n", privateKey)
			os.Exit(1)
		}
	default:
		fmt.Printf("Unsupported private key type: %s\n", privBlock.Type)
		os.Exit(1)
	}

	// 生成公钥
	pubBytes := ssh.MarshalAuthorizedKey(pubKey)

	// 获取唯一的文件名
	newPrivPath, newPubPath := getUniqueFilePaths(c.ConfigPath, name)

	// 保存私钥和公钥
	if err := copyFile(privPath, newPrivPath, 0600); err != nil {
		fmt.Printf("Failed to copy private key: %v\n", err)
		os.Exit(1)
	}

	pubFile, err := os.OpenFile(newPubPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Printf("Failed to create public key file: %v\n", err)
		os.Exit(1)
	}
	defer pubFile.Close()

	if _, err := pubFile.Write(pubBytes); err != nil {
		fmt.Printf("Failed to write public key: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Imported: %s (%s, %s)\n", name, newPrivPath, newPubPath)
	c.Keys = append(c.Keys, SSHKeyPair{
		Name:           name,
		PrivateKeyPath: newPrivPath,
		PublicKeyPath:  newPubPath,
	})
}

func (c *SSHConfig) listKeyPairs() {
	if len(c.Keys) == 0 {
		fmt.Println("No key pairs found.")
		return
	}
	for i, key := range c.Keys {
		active := ""
		if i == c.ActiveKey {
			active = " (active)"
		}
		fmt.Printf("%d: %s (%s, %s)%s\n", i, key.Name, key.PrivateKeyPath, key.PublicKeyPath, active)
	}
}

func (c *SSHConfig) switchKeyPair(index int) {
	if index >= len(c.Keys) {
		fmt.Printf("Invalid index: %d\n", index)
		os.Exit(1)
	}

	c.ActiveKey = index
	key := c.Keys[index]

	configContent := fmt.Sprintf(`
Host *
	IdentityFile %s
`, key.PrivateKeyPath)

	if err := os.MkdirAll(filepath.Dir(c.SSHConfig), 0700); err != nil {
		fmt.Printf("Failed to create SSH config directory: %v\n", err)
		os.Exit(1)
	}

	if err := ioutil.WriteFile(c.SSHConfig, []byte(configContent), 0600); err != nil {
		fmt.Printf("Failed to update SSH config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Switched to: %s (%s)\n", key.Name, key.PrivateKeyPath)
}

func copyFile(src, dst string, perm os.FileMode) error {
	data, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(dst, data, perm)
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