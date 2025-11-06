package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/pbkdf2"
)

// Authenticator handles authentication against Calibre-Web database
type Authenticator struct {
	dbPath string
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(dbPath string) *Authenticator {
	return &Authenticator{
		dbPath: dbPath,
	}
}

// Authenticate validates Basic Auth credentials against the database
// Returns true if authentication is successful
func (a *Authenticator) Authenticate(username, password string) (bool, error) {
	// If no database path is configured, always authenticate
	if a.dbPath == "" {
		return true, nil
	}

	// Open database in read-only mode
	dbURI := fmt.Sprintf("file:%s?mode=ro&immutable=1", a.dbPath)
	db, err := sql.Open("sqlite3", dbURI)
	if err != nil {
		return false, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Query for user's password hash
	var passwordHash string
	err = db.QueryRow("SELECT password FROM user WHERE name = ?", username).Scan(&passwordHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("database query failed: %w", err)
	}

	// Verify password hash
	return a.checkPasswordHash(passwordHash, password)
}

// checkPasswordHash verifies a password against a Werkzeug-style hash
// Werkzeug format: pbkdf2:sha256:260000$salt$hash
func (a *Authenticator) checkPasswordHash(hashString, password string) (bool, error) {
	if hashString == "" {
		return false, nil
	}

	// Parse the hash string
	parts := strings.Split(hashString, ":")
	if len(parts) < 3 {
		return false, fmt.Errorf("invalid hash format")
	}

	method := parts[0]
	algorithm := parts[1]
	
	// Only support pbkdf2:sha256
	if method != "pbkdf2" || algorithm != "sha256" {
		return false, fmt.Errorf("unsupported hash method: %s:%s", method, algorithm)
	}

	// Parse iterations and salt/hash
	var iterations int
	var saltAndHash string
	
	if len(parts) == 3 {
		// Format: pbkdf2:sha256:iterations$salt$hash
		saltHashParts := strings.SplitN(parts[2], "$", 3)
		if len(saltHashParts) != 3 {
			return false, fmt.Errorf("invalid salt/hash format")
		}
		
		_, err := fmt.Sscanf(saltHashParts[0], "%d", &iterations)
		if err != nil {
			return false, fmt.Errorf("invalid iterations: %w", err)
		}
		
		saltAndHash = saltHashParts[1] + "$" + saltHashParts[2]
	} else if len(parts) == 4 {
		// Format: pbkdf2:sha256:iterations:salt$hash
		_, err := fmt.Sscanf(parts[2], "%d", &iterations)
		if err != nil {
			return false, fmt.Errorf("invalid iterations: %w", err)
		}
		saltAndHash = parts[3]
	} else {
		return false, fmt.Errorf("invalid hash format")
	}

	// Split salt and hash
	saltHashParts := strings.SplitN(saltAndHash, "$", 2)
	if len(saltHashParts) != 2 {
		return false, fmt.Errorf("invalid salt/hash separation")
	}

	salt := saltHashParts[0]
	storedHash := saltHashParts[1]

	// Decode base64 salt (Werkzeug uses standard base64 encoding)
	saltBytes, err := base64.StdEncoding.DecodeString(salt)
	if err != nil {
		// Try URL-safe encoding
		saltBytes, err = base64.URLEncoding.DecodeString(salt)
		if err != nil {
			return false, fmt.Errorf("failed to decode salt: %w", err)
		}
	}

	// Decode base64 hash
	storedHashBytes, err := base64.StdEncoding.DecodeString(storedHash)
	if err != nil {
		// Try URL-safe encoding
		storedHashBytes, err = base64.URLEncoding.DecodeString(storedHash)
		if err != nil {
			return false, fmt.Errorf("failed to decode hash: %w", err)
		}
	}

	// Compute PBKDF2 hash
	computedHash := pbkdf2.Key([]byte(password), saltBytes, iterations, len(storedHashBytes), sha256.New)

	// Constant-time comparison
	if subtle.ConstantTimeCompare(computedHash, storedHashBytes) == 1 {
		return true, nil
	}

	return false, nil
}
