# Basic Authentication Guide

This guide explains how to use the new basic authentication support in ElasticETL, which provides flexible password handling with multiple security options including encryption and environment variable support.

## Overview

ElasticETL now supports basic authentication through the `auth_basic` configuration in the extract section. This feature provides:

- **Multiple Password Types**: Support for plain text, base64 encoded, encrypted, and environment variable-based passwords
- **Flexible Encryption**: AES-256-GCM encryption with custom or default keys
- **Backward Compatibility**: Existing `auth_headers` configurations continue to work unchanged
- **Priority System**: `auth_headers` takes precedence over `auth_basic` when both are configured

## Configuration

### Basic Configuration Structure

```yaml
extract:
  auth_basic:
    user: "username"
    password: "password_value"
    password_type: "PASSWORD_TYPE"  # Optional, defaults to PLAIN_TEXT
    passkey: "encryption_key"       # Optional, for encrypted types
```

### Supported Password Types

#### 1. PLAIN_TEXT (Default)
Use the password as-is without any processing.

```yaml
auth_basic:
  user: "elastic"
  password: "mypassword"
  password_type: "PLAIN_TEXT"  # Optional, this is the default
```

#### 2. PLAIN_TEXT_BASE64
Password is base64 encoded and will be decoded before use.

```yaml
auth_basic:
  user: "elastic"
  password: "bXlwYXNzd29yZA=="  # Base64 encoded "mypassword"
  password_type: "PLAIN_TEXT_BASE64"
```

#### 3. ENCRYPTED
Password is encrypted using AES-256-GCM and will be decrypted before use.

```yaml
auth_basic:
  user: "elastic"
  password: "encrypted_password_string"
  password_type: "ENCRYPTED"
  passkey: "my-secret-encryption-key-32-chars"  # Optional, uses default if empty
```

#### 4. ENCRYPTED_BASE64
Password is base64 encoded and encrypted, will be decoded then decrypted.

```yaml
auth_basic:
  user: "elastic"
  password: "base64_encoded_encrypted_password"
  password_type: "ENCRYPTED_BASE64"
  passkey: "my-secret-encryption-key-32-chars"
```

#### 5. ENV_VAR
Password is stored in an environment variable.

```yaml
auth_basic:
  user: "elastic"
  password: "ELASTIC_PASSWORD"  # Environment variable name
  password_type: "ENV_VAR"
```

#### 6. ENV_VAR_BASE64
Password is stored in an environment variable and is base64 encoded.

```yaml
auth_basic:
  user: "elastic"
  password: "ELASTIC_PASSWORD_B64"  # Environment variable with base64 encoded password
  password_type: "ENV_VAR_BASE64"
```

#### 7. ENV_VAR_ENCRYPTED
Password is stored in an environment variable and is encrypted.

```yaml
auth_basic:
  user: "elastic"
  password: "ELASTIC_PASSWORD_ENC"  # Environment variable with encrypted password
  password_type: "ENV_VAR_ENCRYPTED"
  passkey: "my-secret-encryption-key-32-chars"
```

#### 8. ENV_VAR_ENCRYPTED_BASE64
Password is stored in an environment variable, base64 encoded, and encrypted.

```yaml
auth_basic:
  user: "elastic"
  password: "ELASTIC_PASSWORD_ENC_B64"  # Environment variable with base64 encoded encrypted password
  password_type: "ENV_VAR_ENCRYPTED_BASE64"
  passkey: "my-secret-encryption-key-32-chars"
```

## Authentication Priority

When both authentication methods are configured, the priority is:

1. **auth_headers** (if provided and not empty) - takes precedence
2. **auth_basic** (if auth_headers is not available) - fallback option

```yaml
extract:
  auth_headers:
    - "Bearer token123"  # This will be used
  auth_basic:
    user: "elastic"      # This will be ignored
    password: "pass"
```

## Encryption Details

### Default Encryption Key

If no `passkey` is provided for encrypted password types, ElasticETL uses a default key:
```
elasticetl-default-key-32-chars
```

**Security Note**: For production use, always provide your own encryption key via the `passkey` field.

### Key Requirements

- Encryption keys must be exactly 32 characters for AES-256
- If a shorter key is provided, it will be padded with the default key
- If a longer key is provided, it will be truncated to 32 characters

### Encryption Algorithm

- **Algorithm**: AES-256-GCM
- **Mode**: Galois/Counter Mode (provides both encryption and authentication)
- **Key Size**: 256 bits (32 bytes)
- **Output Format**: Base64 encoded ciphertext with embedded nonce

## Example Configurations

### Simple Basic Authentication

```yaml
pipelines:
  - name: "simple-auth"
    extract:
      urls:
        - "http://localhost:9200/_search"
      cluster_names:
        - "local"
      auth_basic:
        user: "elastic"
        password: "changeme"
      # ... rest of configuration
```

### Environment Variable Authentication

```yaml
pipelines:
  - name: "env-auth"
    extract:
      urls:
        - "https://api.example.com/data"
      cluster_names:
        - "production"
      auth_basic:
        user: "api_user"
        password: "API_PASSWORD"  # Set via: export API_PASSWORD="secret123"
        password_type: "ENV_VAR"
      # ... rest of configuration
```

### Encrypted Password Authentication

```yaml
pipelines:
  - name: "encrypted-auth"
    extract:
      urls:
        - "https://secure-api.example.com/data"
      cluster_names:
        - "secure"
      auth_basic:
        user: "secure_user"
        password: "U2FsdGVkX1+vupppZksvRf5pq5g5XjFRIipRkwB0K1Y="
        password_type: "ENCRYPTED_BASE64"
        passkey: "my-production-encryption-key-32c"
      # ... rest of configuration
```

## Security Best Practices

### 1. Use Environment Variables for Sensitive Data

```yaml
# Good: Password stored in environment variable
auth_basic:
  user: "elastic"
  password: "ELASTIC_PASSWORD"
  password_type: "ENV_VAR"
```

```bash
# Set environment variable securely
export ELASTIC_PASSWORD="your-secure-password"
```

### 2. Use Encryption for Stored Passwords

```yaml
# Good: Encrypted password in configuration
auth_basic:
  user: "elastic"
  password: "encrypted_password_here"
  password_type: "ENCRYPTED"
  passkey: "your-unique-32-character-key-here"
```

### 3. Avoid Plain Text in Configuration Files

```yaml
# Avoid: Plain text password in config file
auth_basic:
  user: "elastic"
  password: "plaintext-password"  # Not recommended for production
```

### 4. Use Strong Encryption Keys

- Generate random 32-character keys
- Store encryption keys separately from configuration files
- Consider using key management systems for production

## Password Encryption Utility

You can encrypt passwords using the built-in utility functions. Here's an example of how to encrypt a password:

```go
package main

import (
    "fmt"
    "elasticetl/pkg/utils"
)

func main() {
    password := "mysecretpassword"
    key := "my-production-encryption-key-32c"
    
    encrypted, err := utils.EncryptAES(password, key)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Encrypted password: %s\n", encrypted)
}
```

## Troubleshooting

### Common Issues

#### 1. Environment Variable Not Found

**Error**: `environment variable API_PASSWORD is not set or empty`

**Solution**: Ensure the environment variable is set before running ElasticETL:
```bash
export API_PASSWORD="your-password"
./elasticetl -config config.yaml
```

#### 2. Decryption Failed

**Error**: `failed to decrypt: cipher: message authentication failed`

**Solutions**:
- Verify the encryption key matches the one used for encryption
- Ensure the encrypted password string is complete and not truncated
- Check that the password was encrypted with the same algorithm (AES-256-GCM)

#### 3. Base64 Decode Error

**Error**: `failed to decode base64: illegal base64 data`

**Solutions**:
- Verify the base64 string is properly formatted
- Ensure no extra whitespace or line breaks in the password string
- Check that the string was encoded with standard base64 encoding

#### 4. Authentication Failed

**Error**: `HTTP 401: Unauthorized`

**Solutions**:
- Verify the username and password are correct
- Check that the API endpoint supports basic authentication
- Ensure the password processing is working correctly by testing with `PLAIN_TEXT` type first

### Debug Steps

1. **Test with Plain Text**: Start with `password_type: "PLAIN_TEXT"` to verify credentials work
2. **Check Environment Variables**: Use `printenv | grep PASSWORD` to verify environment variables are set
3. **Validate Encryption**: Test encryption/decryption separately before using in configuration
4. **Enable Debug Logging**: Set logging level to debug to see detailed authentication information

## Migration from auth_headers

If you're currently using `auth_headers` for basic authentication, you can migrate to `auth_basic`:

### Before (auth_headers)
```yaml
extract:
  auth_headers:
    - "Basic ZWxhc3RpYzpjaGFuZ2VtZQ=="  # Base64 encoded "elastic:changeme"
```

### After (auth_basic)
```yaml
extract:
  auth_basic:
    user: "elastic"
    password: "changeme"
    password_type: "PLAIN_TEXT"
```

### Benefits of Migration

- **Clearer Configuration**: Separate username and password fields
- **Enhanced Security**: Multiple password protection options
- **Better Maintainability**: No need to manually encode credentials
- **Flexibility**: Easy switching between different password types

## API Reference

### Configuration Fields

```yaml
auth_basic:
  user: string          # Required: Username for authentication
  password: string      # Required: Password or reference (env var name, encrypted value, etc.)
  password_type: string # Optional: Password processing type (default: PLAIN_TEXT)
  passkey: string       # Optional: Encryption key for encrypted password types
```

### Password Types

| Type | Description | Requires Passkey |
|------|-------------|------------------|
| `PLAIN_TEXT` | Use password as-is | No |
| `PLAIN_TEXT_BASE64` | Base64 decode password | No |
| `ENCRYPTED` | Decrypt password using AES | Optional |
| `ENCRYPTED_BASE64` | Base64 decode then decrypt | Optional |
| `ENV_VAR` | Get password from environment variable | No |
| `ENV_VAR_BASE64` | Get from env var and base64 decode | No |
| `ENV_VAR_ENCRYPTED` | Get from env var and decrypt | Optional |
| `ENV_VAR_ENCRYPTED_BASE64` | Get from env var, decode, and decrypt | Optional |

### Error Handling

All authentication errors are wrapped with descriptive messages:

- Configuration errors: `auth_basic configuration is nil`
- Password processing errors: `failed to process password: <details>`
- Environment variable errors: `environment variable <name> is not set or empty`
- Encryption errors: `failed to decrypt: <details>`
- Base64 errors: `failed to decode base64: <details>`

## Security Considerations

1. **Key Management**: Store encryption keys securely, separate from configuration files
2. **Environment Variables**: Use secure methods to set environment variables in production
3. **File Permissions**: Restrict access to configuration files containing sensitive data
4. **Logging**: Ensure passwords are not logged in debug output
5. **Network Security**: Use HTTPS for API endpoints when possible
6. **Regular Rotation**: Implement password rotation policies for production systems

## Performance Impact

The basic authentication feature has minimal performance impact:

- **Password Processing**: Done once per pipeline initialization
- **Memory Usage**: Processed passwords are not stored permanently
- **CPU Overhead**: Encryption/decryption operations are fast for single passwords
- **Network Impact**: No additional network requests for authentication processing
