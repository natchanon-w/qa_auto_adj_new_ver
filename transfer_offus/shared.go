package main

import (
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
)

type Config struct {
	Env           string `json:"-"` // active_env, for display
	Bucket        string `json:"bucket"`
	BasePath      string `json:"base_path"`
	AwsProfile    string `json:"aws_profile"`
	Region        string `json:"region"`         // AWS region of Bucket; passed as --region to the aws CLI. Empty = let the CLI/profile decide.
	EncryptionKey string `json:"encryption_key"` // base64 AES-GCM key = the env's savedb secret.env AES_GCM_KEY (inbound_actual_account PII)
}

type EnvConfig struct {
	Bucket        string `json:"bucket"`
	BasePath      string `json:"base_path"`
	AwsProfile    string `json:"aws_profile"`
	Region        string `json:"region"`
	EncryptionKey string `json:"encryption_key"`
}

type RawConfig struct {
	ActiveEnv    string               `json:"active_env"`
	Environments map[string]EnvConfig `json:"environments"`
}

// TypeState holds everything finalize needs for one of the 4 off-us file
// variants (inbound/outbound x promptpay/actual-account).
type TypeState struct {
	FilePrefix     string                       `json:"file_prefix"`
	ControlSlug    string                       `json:"control_slug"`
	Table          string                       `json:"table"`
	RefColumn      string                       `json:"ref_column"`
	StatusColumn   string                       `json:"status_column"`
	ResetStatus    string                       `json:"reset_status"` // value the update script resets to; "" (old state.json) = PROCESSING
	SqlColumns     []string                     `json:"sql_columns"`
	RawCsvFilename string                       `json:"raw_csv_filename"`
	CsvFilename    string                       `json:"csv_filename"`
	SqlRows        map[string]map[string]string `json:"sql_rows"`
	SharedRefs     []string                     `json:"shared_refs"`
}

type StateFile struct {
	GeneratedAt string               `json:"generated_at"`
	Timestamp   string               `json:"timestamp"`
	Types       map[string]TypeState `json:"types"`
}

func baseDir() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return dir
}

func readConfig() Config {
	path := filepath.Join(baseDir(), "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("Error reading config.json: %v\n", err)
		os.Exit(1)
	}
	var raw RawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		fmt.Printf("Error parsing config.json: %v\n", err)
		os.Exit(1)
	}
	if raw.ActiveEnv == "" {
		fmt.Println("config.json is missing \"active_env\"")
		os.Exit(1)
	}
	env, ok := raw.Environments[raw.ActiveEnv]
	if !ok {
		fmt.Printf("active_env %q not found under \"environments\" in config.json\n", raw.ActiveEnv)
		os.Exit(1)
	}
	return Config{
		Env:           raw.ActiveEnv,
		Bucket:        env.Bucket,
		BasePath:      env.BasePath,
		AwsProfile:    env.AwsProfile,
		Region:        env.Region,
		EncryptionKey: env.EncryptionKey,
	}
}

// extractDateFromBasePath scans every "/"-separated segment of base_path for
// one that parses as YYYY-MM-DD. Off-us base_path ends in ".../request/"
// (a real convention, not a date), so — unlike the single-segment tools —
// we can't just take the last path segment.
func extractDateFromBasePath(basePath string) time.Time {
	for _, part := range strings.Split(strings.Trim(basePath, "/"), "/") {
		if d, err := time.Parse("2006-01-02", part); err == nil {
			return d
		}
	}
	fmt.Printf("Cannot find a YYYY-MM-DD date segment in base_path %q\n", basePath)
	os.Exit(1)
	return time.Time{}
}

func resolveWorkDir(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	latestPath := filepath.Join(baseDir(), ".latest")
	data, err := os.ReadFile(latestPath)
	if err != nil {
		fmt.Println("No .latest file found. Run 'generate' first or pass a work dir explicitly.")
		os.Exit(1)
	}
	name := strings.TrimSpace(string(data))
	return filepath.Join(baseDir(), "work", name)
}

func sha256File(path string) string {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		panic(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func encryptFile(inputFile, outputFile, pubKeyPath string) error {
	pgp := crypto.PGP()
	pubKeyBytes, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}
	pubKey, err := crypto.NewKeyFromArmored(string(pubKeyBytes))
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}
	encHandle, err := pgp.Encryption().Recipient(pubKey).New()
	if err != nil {
		return fmt.Errorf("failed to create encryption handle: %w", err)
	}
	inBytes, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}
	pgpMessage, err := encHandle.Encrypt(inBytes)
	if err != nil {
		return fmt.Errorf("failed to encrypt: %w", err)
	}
	out, err := pgpMessage.ArmorBytes()
	if err != nil {
		return fmt.Errorf("failed to armor: %w", err)
	}
	return os.WriteFile(outputFile, out, 0644)
}

func decryptToBytes(encryptedPath, privKeyPath string) ([]byte, error) {
	privKeyBytes, err := os.ReadFile(privKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}
	privKey, err := crypto.NewKeyFromArmored(string(privKeyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	pgp := crypto.PGP()
	decHandle, err := pgp.Decryption().DecryptionKey(privKey).New()
	if err != nil {
		return nil, fmt.Errorf("failed to create decryption handle: %w", err)
	}
	armoredBytes, err := os.ReadFile(encryptedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read encrypted file: %w", err)
	}
	result, err := decHandle.Decrypt(armoredBytes, crypto.Armor)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}
	return result.Bytes(), nil
}

// newPIIEncrypter returns the function that encrypts PII columns the way the
// online inbound path stores them (payment-lib-j-common-service
// AESEncryptionService.encryptAESGCM): Base64(IV 12 bytes || ciphertext+tag),
// AES-GCM with a Base64 key, blank stays blank. The key is the active env's
// config.json "encryption_key" (= that env's savedb secret.env AES_GCM_KEY);
// env var AES_GCM_KEY overrides it. Non-PRD keys only.
// plain=true returns values unchanged, for an env with aes.gcm.enable=false.
func newPIIEncrypter(plain bool, cfg Config) func(string) string {
	if plain {
		return func(s string) string { return s }
	}
	keyB64 := strings.TrimSpace(os.Getenv("AES_GCM_KEY"))
	if keyB64 == "" {
		keyB64 = strings.TrimSpace(cfg.EncryptionKey)
	}
	if keyB64 == "" {
		fmt.Printf("No AES-GCM key for env %q.\n", cfg.Env)
		fmt.Println("  inbound_actual_account rows must be AES-GCM encrypted like online (aes.gcm.enable=true on SIT/UAT),")
		fmt.Println("  otherwise savedb cannot decrypt them and auto-adjustment sends no TM / CLOG message.")
		fmt.Println("  Set \"encryption_key\" for this env in config.json (the env's savedb secret.env AES_GCM_KEY),")
		fmt.Println("  or export AES_GCM_KEY, or pass --plain for an env with gcm off.")
		os.Exit(1)
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		fmt.Printf("AES_GCM_KEY is not valid Base64: %v\n", err)
		os.Exit(1)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Printf("AES_GCM_KEY is not a valid AES key: %v\n", err)
		os.Exit(1)
	}
	gcm, err := cipher.NewGCM(block) // 12-byte nonce, 16-byte tag = Java GCM_IV_LENGTH 12 / GCM_TAG_LENGTH 128
	if err != nil {
		panic(err)
	}
	return func(s string) string {
		if strings.TrimSpace(s) == "" {
			return ""
		}
		iv := make([]byte, gcm.NonceSize())
		if _, err := cryptorand.Read(iv); err != nil {
			panic(err)
		}
		return base64.StdEncoding.EncodeToString(gcm.Seal(iv, iv, []byte(s), nil))
	}
}

func sqlFormat(val interface{}) string {
	if val == nil {
		return "NULL"
	}
	switch v := val.(type) {
	case string:
		if v == "NULL" || v == "" {
			return "NULL"
		}
		return fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.2f", v)
	}
	return fmt.Sprintf("'%v'", val)
}
