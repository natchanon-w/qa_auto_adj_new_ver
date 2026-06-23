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
	"ref_id", "payment_token", "retrieval_ref_no", "cust_ref_id", "bpp_retrieval_ref_no",
	"bpp_txn_ref", "customer_note", "from_acct_id", "status", "dcb_status", "status_cd", "status_desc",
	"biller_ref_type", "biller_ref_value", "biller_name_th", "biller_name_en", "to_display_name",
	"channel_agent_id", "customer_fee", "company_fee", "banking_agent_fee", "total_fee", "payment_fee",
	"amount", "ref1", "ref2", "ref3", "ref4", "account_type", "source_of_fund", "sof_type", "req_by",
	"req_dtm", "reverse_dtm", "bill_payment_workflow", "to_acct_no", "to_bank_cd", "proc_cd", "terminal_type",
	"category", "created_dtm", "updated_dtm", "payment_txn_ref", "from_acct_no", "pib_id", "dcb_created_request_id",
	"print1", "print2", "print3", "print4", "print5", "print6", "print7", "transaction_type", "transaction_date_time",
	"transaction_code", "internal_account_id", "transaction_class", "denomination", "reversal_flag", "tfr_dtm",
	"fee_internal_account_id", "fee_transaction_code", "fee_transaction_amount", "fee_type", "posting_type",
	"effective_date", "dlp_status", "state", "partner_ref_id", "from_main_account_no", "from_address",
	"to_main_account_no", "to_address", "is_force_success", "input_terminal", "from_bank_code",
	"from_account_display_name", "from_account_name_th", "from_account_name_en", "from_province_code",
	"term_type", "pan_id", "terminal_id", "transferee_fee", "transferer_fee", "sender_fee", "instruction_id",
	"type_of_sender", "type_of_receiver", "share_flag", "mer_cat_code", "settlement_date",
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
