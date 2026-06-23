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

type StateFile struct {
	GeneratedAt string                       `json:"generated_at"`
	Timestamp   string                       `json:"timestamp"`
	CsvFilename string                       `json:"csv_filename"`
	SqlRows     map[string]map[string]string `json:"sql_rows"`
	SharedRefs  []string                     `json:"shared_refs"`
}

var sqlColumns = []string{
	"ref_id", "original_ref_id", "req_channel", "requester", "req_dtm", "ref_no",
	"payment_txn_ref", "tfr_dtm", "created_request_id", "amount", "denomination",
	"customer_note", "customer_ref_id",
	"from_main_account_id", "from_main_account_no", "from_transfer_account_no",
	"from_acct_no", "from_acct_id", "to_internal_acct_id",
	"from_trans_code", "from_address", "from_acct_status",
	"from_product_class", "from_product_group", "from_product_type", "from_acct_type",
	"from_branch_code", "from_account_display_name", "from_account_name_th", "from_account_name_en",
	"from_bank_code", "from_core_bank_channel", "from_cif_no", "from_cdi_token", "from_internal_acct_id",
	"to_main_account_id", "to_main_account_no", "to_transfer_account_no",
	"to_acct_no", "to_acct_id", "to_trans_code", "to_address", "to_acct_status",
	"to_product_class", "to_product_group", "to_product_type", "to_acct_type",
	"to_branch_code", "to_account_display_name", "to_account_name_th", "to_account_name_en",
	"to_bank_code", "to_core_bank_channel", "to_cif_no", "to_cdi_token",
	"fee_amount", "fee_internal_acct_id", "fee_trans_code", "fee_type",
	"status", "status_code", "status_desc", "debit_status", "credit_status",
	"service_type", "created_dtm", "updated_dtm", "transaction_type", "posting_type",
	"effective_date", "reversal_flag", "transfer_type", "input_terminal",
	"dcb_request", "dcb_response", "debit_ref_id", "credit_ref_id",
	"user_id", "trans_location", "from_comment", "to_comment",
	"source_of_payment", "erp_info_debit", "erp_info_credit", "info",
	"term_id", "term_type", "waive_flag", "verify_ref", "term_branch",
	"adjusted_amount", "override_amount", "is_pay_off",
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
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Printf("Error parsing config.json: %v\n", err)
		os.Exit(1)
	}
	return cfg
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
