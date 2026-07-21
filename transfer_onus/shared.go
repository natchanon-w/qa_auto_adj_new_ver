package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
)

type Config struct {
	Bucket     string `json:"bucket"`
	BasePath   string `json:"base_path"`
	AwsProfile string `json:"aws_profile"`
}

type EnvConfig struct {
	Bucket     string `json:"bucket"`
	BasePath   string `json:"base_path"`
	AwsProfile string `json:"aws_profile"`
}

type RawConfig struct {
	ActiveEnv    string               `json:"active_env"`
	Environments map[string]EnvConfig `json:"environments"`
}

type StateFile struct {
	GeneratedAt string                       `json:"generated_at"`
	Timestamp   string                       `json:"timestamp"`
	CsvFilename string                       `json:"csv_filename"`
	SqlRows     map[string]map[string]string `json:"sql_rows"`
	SharedRefs  []string                     `json:"shared_refs"`
}

// sqlColumns is the column order for source table "internal_transfer_transactions"
// (schema provided by the user), excluding the bigserial "seq_id" primary key
// which the DB assigns on insert.
var sqlColumns = []string{
	"req_channel", "requester", "ref_id", "req_dtm", "retrieval_ref_no", "pib_id",
	"created_request_id", "amount", "payment_txn_ref", "customer_note",
	"from_acct_no", "from_acct_id", "from_trans_code", "from_internal_acct_id",
	"from_acct_status", "from_acct_class", "from_acct_group", "from_acct_type",
	"to_acct_no", "to_acct_id", "to_trans_code", "to_internal_acct_id",
	"to_acct_status", "to_acct_class", "to_acct_group", "to_acct_type",
	"status", "status_code", "status_desc", "payment_fee", "service_type",
	"created_dtm", "updated_dtm",
	"from_account_display_name", "from_account_name_th", "from_account_name_en",
	"to_account_display_name", "to_account_name_th", "to_account_name_en",
	"denomination", "tfr_dtm", "transaction_type", "resp_status_code", "resp_status_desc",
	"posting_type", "effective_date", "from_bank_code", "to_bank_code",
	"from_product_group", "from_product_type", "to_product_group", "to_product_type",
	"from_branch_code", "from_core_bank", "to_branch_code", "to_core_bank",
	"to_proxy_type", "to_proxy_value", "customer_ref_id",
	"from_main_account_no", "to_main_account_no",
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
		Bucket:     env.Bucket,
		BasePath:   env.BasePath,
		AwsProfile: env.AwsProfile,
	}
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
