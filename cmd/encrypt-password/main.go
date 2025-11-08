package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	"elasticetl/pkg/utils"

	"golang.org/x/term"
)

func main() {
	var key = flag.String("key", "", "Encryption key (32 characters). If not provided, default key will be used.")
	var password = flag.String("password", "", "Password to encrypt. If not provided, will prompt securely.")
	var help = flag.Bool("help", false, "Show help message")

	flag.Parse()

	if *help {
		showHelp()
		return
	}

	// Get password
	var passwordToEncrypt string
	if *password != "" {
		passwordToEncrypt = *password
	} else {
		var err error
		passwordToEncrypt, err = getPasswordSecurely()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading password: %v\n", err)
			os.Exit(1)
		}
	}

	// Get encryption key
	encryptionKey := *key
	if encryptionKey == "" {
		fmt.Println("No encryption key provided, using default key.")
		fmt.Println("For production use, provide a custom 32-character key with -key flag.")
		encryptionKey = "elasticetl-default-key-32-chars"
	} else if len(encryptionKey) != 32 {
		fmt.Printf("Warning: Key length is %d characters. AES-256 requires exactly 32 characters.\n", len(encryptionKey))
		if len(encryptionKey) < 32 {
			fmt.Println("Key will be padded with default key.")
		} else {
			fmt.Println("Key will be truncated to 32 characters.")
		}
	}

	// Encrypt password
	encrypted, err := utils.EncryptAES(passwordToEncrypt, encryptionKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encrypting password: %v\n", err)
		os.Exit(1)
	}

	// Display results
	fmt.Println("\n=== Encryption Results ===")
	fmt.Printf("Encrypted password: %s\n", encrypted)
	fmt.Println("\n=== Configuration Examples ===")

	fmt.Println("\n1. ENCRYPTED type:")
	fmt.Println("auth_basic:")
	fmt.Println("  user: \"your_username\"")
	fmt.Printf("  password: \"%s\"\n", encrypted)
	fmt.Println("  password_type: \"ENCRYPTED\"")
	if *key != "" {
		fmt.Printf("  passkey: \"%s\"\n", encryptionKey)
	}

	fmt.Println("\n2. ENCRYPTED_BASE64 type (same as above, already base64 encoded):")
	fmt.Println("auth_basic:")
	fmt.Println("  user: \"your_username\"")
	fmt.Printf("  password: \"%s\"\n", encrypted)
	fmt.Println("  password_type: \"ENCRYPTED_BASE64\"")
	if *key != "" {
		fmt.Printf("  passkey: \"%s\"\n", encryptionKey)
	}

	fmt.Println("\n=== Security Notes ===")
	fmt.Println("- Store the encryption key securely, separate from your configuration")
	fmt.Println("- Use environment variables or secure key management for production")
	fmt.Println("- The encrypted password can be safely stored in configuration files")
	if *key == "" {
		fmt.Println("- Consider using a custom encryption key for better security")
	}
}

func getPasswordSecurely() (string, error) {
	fmt.Print("Enter password to encrypt: ")

	// Read password without echoing to terminal
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}

	fmt.Println() // Print newline after password input

	password := string(bytePassword)
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("password cannot be empty")
	}

	return password, nil
}

func showHelp() {
	fmt.Println("ElasticETL Password Encryption Utility")
	fmt.Println("=====================================")
	fmt.Println()
	fmt.Println("This utility encrypts passwords for use with ElasticETL's basic authentication feature.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  encrypt-password [flags]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -key string")
	fmt.Println("        Encryption key (32 characters). If not provided, default key will be used.")
	fmt.Println("  -password string")
	fmt.Println("        Password to encrypt. If not provided, will prompt securely.")
	fmt.Println("  -help")
	fmt.Println("        Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Encrypt password with secure prompt and default key")
	fmt.Println("  encrypt-password")
	fmt.Println()
	fmt.Println("  # Encrypt password with custom key")
	fmt.Println("  encrypt-password -key \"my-secret-encryption-key-32-chars\"")
	fmt.Println()
	fmt.Println("  # Encrypt specific password with custom key")
	fmt.Println("  encrypt-password -password \"mypassword\" -key \"my-secret-key-32-characters-long\"")
	fmt.Println()
	fmt.Println("Security Notes:")
	fmt.Println("  - Use the secure prompt (no -password flag) to avoid password in shell history")
	fmt.Println("  - Store encryption keys securely, separate from configuration files")
	fmt.Println("  - Use custom encryption keys for production environments")
	fmt.Println("  - The encrypted output can be safely stored in configuration files")
}
