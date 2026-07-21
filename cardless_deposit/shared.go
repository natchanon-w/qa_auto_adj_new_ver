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

// sqlColumns mirrors payment-bulk-adj-cardless-deposit-processor/local-scripts/generate_test_data.go —
// column order for the source table "cardless_deposit_transactions".
var sqlColumns = []string{
	"id", "req_ref_id", "req_dtm", "req_channel", "requester", "pan_marking", "trace_id", "mti", "inteface",
	"processing_code", "card_ref", "terminal_id", "acquirer_id", "card_acceptor_identification_code",
	"location", "merchant_cat_code", "rrn", "transaction_status", "cust_ref_id", "to_account_no",
	"to_account_name_th", "to_account_name_en", "to_account_branch_code", "to_account_cost_center",
	"to_product_group", "to_product_type", "to_pocket_no", "to_bank_code", "to_account_type",
	"to_account_class", "to_account_group", "to_core_bank", "region", "request_id", "client_id",
	"payment_transaction_ref", "transaction_type", "posting_type", "transaction_amount", "denomination",
	"transaction_datetime", "effective_date", "to_account_id", "to_address", "to_transaction_code",
	"from_internal_account_id", "to_transaction_class", "fee_internal_account_id", "fee_transaction_code",
	"fee_transaction_amount", "fee_type", "b24_fee", "access_fee", "system_fee", "acq_fee", "iss_fee",
	"to_fee", "pib_id", "transaction_created_datetime", "transaction_created_request_id",
	"transaction_value_datetime", "to_account_ledger_balance", "to_account_available_balance",
	"dcb_resp_code", "dcb_resp_msg", "created_dtm", "updated_dtm", "operation_transaction_ref",
	"location_cost_center", "traceparent", "term_id", "local_transaction_time", "local_transaction_date",
	"transmission_date_time", "cdi_token",
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
