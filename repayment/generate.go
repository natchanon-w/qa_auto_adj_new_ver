package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var csvHeader = []string{
	"reconcile_status", "unmatch_reason", "oper_trigger", "auto_adj_transfer_status", "auto_adj_payment_status",
	"reconcile_ref_id", "dlp_check_duplicate_key", "dpp_check_duplicate_key", "dcb_check_duplicate_key",
	"dlp_effective_date", "dlp_event_dtm", "dlp_from_pocket_no", "dlp_to_loan_acct_no", "dlp_to_loan_acct_id",
	"dlp_transfer_status", "dlp_payment_status", "dlp_txn_amt", "dlp_principal", "dlp_interest",
	"dlp_penalty_interest", "dlp_collection_fee", "dlp_initiated_by",
	"dpp_effective_date", "dpp_event_dtm", "dpp_from_acct_no", "dpp_from_pocket_no", "dpp_from_acct_id",
	"dpp_to_loan_acct_no", "dpp_to_loan_acct_id", "dpp_transfer_status", "dpp_txn_amt", "dpp_txn_fee",
	"dcb_effective_date", "dcb_event_dtm", "dcb_to_loan_acct_id", "dcb_transfer_status", "dcb_payment_status",
	"dcb_txn_amt", "dcb_principal", "dcb_interest", "dcb_penalty_interest", "dcb_collection_fee",
}

var cases = []map[string]string{
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "NULL", "dlp_payment_status": "PENDING", "after_status": "COMPLETED"},
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "SUCCESS", "dlp_payment_status": "COMPLETED", "after_status": "COMPLETED"},
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "SUCCESS", "dlp_payment_status": "PENDING", "after_status": "COMPLETED"},
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "FAILED", "dlp_payment_status": "FAILED", "after_status": "COMPLETED"},
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "FAILED", "dlp_payment_status": "PENDING", "after_status": "COMPLETED"},
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "FAILED", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "NULL", "dlp_payment_status": "PENDING", "after_status": "FAILED"},
	{"auto_adj_transfer_status": "DLP", "dcb_transfer_status": "FAILED", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "NULL", "dlp_payment_status": "COMPLETED", "after_status": "No Update"},
	{"auto_adj_transfer_status": "DPP", "dcb_transfer_status": "FAILED", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PRCS", "dcb_payment_status": "SUCCESS", "dlp_payment_status": "PENDING", "after_status": "No Update"},
}

// sqlFormat converts interface{} values to SQL-safe strings stored in state.json.
// finalize.go emits these verbatim (no additional quoting).
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
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.2f", v)
	}
	return fmt.Sprintf("'%v'", val)
}

func cmdGenerate(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: go run . generate <records>")
		os.Exit(1)
	}
	times, err := strconv.Atoi(args[0])
	if err != nil || times < 1 {
		fmt.Printf("Invalid records value: %q — must be a positive integer\n", args[0])
		os.Exit(1)
	}

	now := time.Now()
	timestamp := now.Format("20060102_150405")
	workDir := filepath.Join(baseDir(), "work", timestamp)
	os.MkdirAll(workDir, os.ModePerm)

	cfg := readConfig()
	pathParts := strings.Split(strings.Trim(cfg.BasePath, "/"), "/")
	csvDateStr := pathParts[len(pathParts)-1]
	csvDate, err := time.Parse("2006-01-02", csvDateStr)
	if err != nil {
		fmt.Printf("Cannot parse date from base_path %q: %v\n", cfg.BasePath, err)
		os.Exit(1)
	}
	csvFilename := fmt.Sprintf("REPAYMENT_MANUAL_DDR_RECONCILE_UNMATCHED_%s.csv", csvDate.Format("20060102"))

	var csvRows [][]string
	sqlRowsMap := make(map[string]map[string]string)
	var sharedRefs []string

	for t := 0; t < times; t++ {
		caseItem := cases[t%len(cases)]
		sharedRef := newUUIDv7()

		// Plaintext PII, shared between the CSV row and the SQL row so the two stay
		// consistent. The SQL row stores the encrypted form (see encryptPII below) —
		// savedb-consumer decrypts these columns on read, so ciphertext here must be
		// produced with the same AES-256-GCM scheme it uses, not a static placeholder.
		fromAcctNoPlain := fmt.Sprintf("%d", 100000000000+rand.Int63n(900000000000))
		toLoanAcctNoPlain := "1234567890"
		fromDisplayNamePlain := "QA AUTOMATION TEST"
		fromNameThPlain := "ทดสอบ ระบบคิวเอ"
		fromNameEnPlain := "QA AUTOMATION TEST"

		csvRowMap := map[string]string{
			"reconcile_status":         "DLP Unmatch",
			"unmatch_reason":           "No Record DLP",
			"oper_trigger":             "Y",
			"auto_adj_transfer_status": fmt.Sprintf(`["%s"]`, caseItem["auto_adj_transfer_status"]),
			"auto_adj_payment_status":  `["DPP","DCB"]`,
			"reconcile_ref_id":         sharedRef,
			"dlp_check_duplicate_key":  "NULL",
			"dpp_check_duplicate_key":  sharedRef,
			"dcb_check_duplicate_key":  sharedRef,
			"dlp_effective_date":       "NULL",
			"dlp_event_dtm":            "NULL",
			"dlp_from_pocket_no":       "NULL",
			"dlp_to_loan_acct_no":      "NULL",
			"dlp_to_loan_acct_id":      "NULL",
			"dlp_transfer_status":      caseItem["dlp_transfer_status"],
			"dlp_payment_status":       caseItem["dlp_payment_status"],
			"dlp_txn_amt":              "NULL",
			"dlp_principal":            "NULL",
			"dlp_interest":             "NULL",
			"dlp_penalty_interest":     "NULL",
			"dlp_collection_fee":       "NULL",
			"dlp_initiated_by":         "NULL",
			"dpp_effective_date":       now.Format("2006-01-02"),
			"dpp_event_dtm":            now.Format("2006-01-02 15:04:05"),
			"dpp_from_acct_no":         fromAcctNoPlain,
			"dpp_from_pocket_no":       fmt.Sprintf("%d", 100000000000+rand.Int63n(900000000000)),
			"dpp_from_acct_id":         newUUIDv7(),
			"dpp_to_loan_acct_no":      toLoanAcctNoPlain,
			"dpp_to_loan_acct_id":      newUUIDv7(),
			"dpp_transfer_status":      caseItem["dpp_transfer_status"],
			"dpp_txn_amt":              "1850.00",
			"dpp_txn_fee":              "10.00",
			"dcb_effective_date":       now.Format("2006-01-02"),
			"dcb_event_dtm":            now.Format("2006-01-02 15:04:05"),
			"dcb_to_loan_acct_id":      newUUIDv7(),
			"dcb_transfer_status":      caseItem["dcb_transfer_status"],
			"dcb_payment_status":       caseItem["dcb_payment_status"],
			"dcb_txn_amt":              "1850.00",
			"dcb_principal":            "1500.00",
			"dcb_interest":             "200.00",
			"dcb_penalty_interest":     "100.00",
			"dcb_collection_fee":       "50.00",
		}
		var row []string
		for _, h := range csvHeader {
			row = append(row, csvRowMap[h])
		}
		csvRows = append(csvRows, row)

		// Build SQL row — store pre-formatted SQL fragments
		recordDtm := csvDate.Format("2006-01-02") + fmt.Sprintf(" %02d:%02d:%02d.000000 +00:00", rand.Intn(24), rand.Intn(60), rand.Intn(60))
		cifNo := fmt.Sprintf("%015d", rand.Int63n(999999999999999))
		dcbReq := fmt.Sprintf(`{"clientId": "DigitalPaymentProcessorClientID", "requestId": "%s", "effectiveDate": "%s", "transactionDatetime": "%sT%s+07:00"}`,
			sharedRef, now.Format("20060102"), now.Format("2006-01-02"), now.Format("15:04:05"))
		dcbResp := fmt.Sprintf(`{"code": "0000", "message": "Success", "data": {"createdRequestId": "%s", "createdDatetime": "%s"}}`,
			sharedRef, now.Format("2006-01-02T15:04:05.000+07:00"))

		// Encrypt the whitelisted PII columns with the same AES-256-GCM scheme
		// savedb-consumer uses (repository.DBColumnWhitelist), so the values it reads
		// back and decrypts are genuine ciphertext for the plaintext above — not a
		// static placeholder blob that happens to look like base64.
		fromMainAcctNoEnc, err := encryptAESGCM(fromAcctNoPlain, cfg.EncryptionKey)
		if err != nil {
			fmt.Printf("Failed to encrypt from_main_account_no: %v\n", err)
			os.Exit(1)
		}
		fromTransferAcctNoEnc, err := encryptAESGCM(fromAcctNoPlain, cfg.EncryptionKey)
		if err != nil {
			fmt.Printf("Failed to encrypt from_transfer_account_no: %v\n", err)
			os.Exit(1)
		}
		fromAcctNoEnc, err := encryptAESGCM(fromAcctNoPlain, cfg.EncryptionKey)
		if err != nil {
			fmt.Printf("Failed to encrypt from_acct_no: %v\n", err)
			os.Exit(1)
		}
		fromDisplayNameEnc, err := encryptAESGCM(fromDisplayNamePlain, cfg.EncryptionKey)
		if err != nil {
			fmt.Printf("Failed to encrypt from_account_display_name: %v\n", err)
			os.Exit(1)
		}
		fromNameThEnc, err := encryptAESGCM(fromNameThPlain, cfg.EncryptionKey)
		if err != nil {
			fmt.Printf("Failed to encrypt from_account_name_th: %v\n", err)
			os.Exit(1)
		}
		fromNameEnEnc, err := encryptAESGCM(fromNameEnPlain, cfg.EncryptionKey)
		if err != nil {
			fmt.Printf("Failed to encrypt from_account_name_en: %v\n", err)
			os.Exit(1)
		}
		toAcctNoEnc, err := encryptAESGCM(toLoanAcctNoPlain, cfg.EncryptionKey)
		if err != nil {
			fmt.Printf("Failed to encrypt to_acct_no: %v\n", err)
			os.Exit(1)
		}

		sqlValsRaw := map[string]interface{}{
			"ref_id":                    sharedRef,
			"original_ref_id":           "",
			"req_channel":               "VB",
			"requester":                 "DLP",
			"req_dtm":                   recordDtm,
			"ref_no":                    fmt.Sprintf("%012d", rand.Int63n(999999999999)),
			"payment_txn_ref":           fmt.Sprintf("D07D2%s", now.Format("0102150405")),
			"tfr_dtm":                   recordDtm,
			"created_request_id":        sharedRef,
			"amount":                    "1850.00",
			"denomination":              "THB",
			"customer_note":             "PP Initial Repayment",
			"customer_ref_id":           cifNo,
			"from_main_account_id":      nil,
			"from_main_account_no":      fromMainAcctNoEnc,
			"from_transfer_account_no":  fromTransferAcctNoEnc,
			"from_acct_no":              fromAcctNoEnc,
			"from_acct_id":              newUUIDv7(),
			"to_internal_acct_id":       "FUND_TRANSFER",
			"from_trans_code":           "MLRPMIN",
			"from_address":              nil,
			"from_acct_status":          "0",
			"from_product_class":        "D",
			"from_product_group":        "SAV",
			"from_product_type":         "SA01",
			"from_acct_type":            "POCKET",
			"from_branch_code":          "46",
			"from_account_display_name": fromDisplayNameEnc,
			"from_account_name_th":      fromNameThEnc,
			"from_account_name_en":      fromNameEnEnc,
			"from_bank_code":            "088",
			"from_core_bank_channel":    "DCB",
			"from_cif_no":               cifNo,
			"from_cdi_token":            nil,
			"from_internal_acct_id":     "INTERNAL",
			"to_main_account_id":        nil,
			"to_main_account_no":        nil,
			"to_transfer_account_no":    nil,
			"to_acct_no":                toAcctNoEnc,
			"to_acct_id":                newUUIDv7(),
			"to_trans_code":             "MLRPIN",
			"to_address":                nil,
			"to_acct_status":            nil,
			"to_product_class":          "L",
			"to_product_group":          "LENDING",
			"to_product_type":           "LOAN",
			"to_acct_type":              "LOAN",
			"to_branch_code":            nil,
			"to_account_display_name":   "",
			"to_account_name_th":        "",
			"to_account_name_en":        "",
			"to_bank_code":              "088",
			"to_core_bank_channel":      "DCB",
			"to_cif_no":                 cifNo,
			"to_cdi_token":              cifNo,
			"fee_amount":                0.00,
			"fee_internal_acct_id":      nil,
			"fee_trans_code":            nil,
			"fee_type":                  nil,
			"status":                    "PROCESSING",
			"status_code":               "0000",
			"status_desc":               "Processing",
			"debit_status":              nil,
			"credit_status":             nil,
			"service_type":              "Repayment",
			"created_dtm":               recordDtm,
			"updated_dtm":               recordDtm,
			"transaction_type":          "REPAYMENT_TDR_INITIATE",
			"posting_type":              "OUTBOUND_CUSTOM",
			"effective_date":            csvDate.Format("2006-01-02"),
			"reversal_flag":             "N",
			"transfer_type":             "MANUAL_INITIAL",
			"input_terminal":            "KEYIN",
			"dcb_request":               dcbReq,
			"dcb_response":              dcbResp,
			"debit_ref_id":              nil,
			"credit_ref_id":             nil,
			"user_id":                   nil,
			"trans_location":            nil,
			"from_comment":              nil,
			"to_comment":                nil,
			"source_of_payment":         nil,
			"erp_info_debit":            nil,
			"erp_info_credit":           nil,
			"info":                      nil,
			"term_id":                   nil,
			"term_type":                 nil,
			"waive_flag":                nil,
			"verify_ref":                nil,
			"term_branch":               nil,
			"adjusted_amount":           nil,
			"override_amount":           nil,
			"is_pay_off":                false,
		}

		// Pre-format all values as SQL fragments
		sqlVals := make(map[string]string)
		for _, col := range sqlColumns {
			sqlVals[col] = sqlFormat(sqlValsRaw[col])
		}
		// these NOT NULL columns default to '' — must not be NULL
		sqlVals["original_ref_id"] = "''"
		sqlVals["to_account_display_name"] = "''"
		sqlVals["to_account_name_th"] = "''"
		sqlVals["to_account_name_en"] = "''"
		sqlRowsMap[sharedRef] = sqlVals
		sharedRefs = append(sharedRefs, sharedRef)
	}

	rawCsvPath := filepath.Join(workDir, "raw.csv")
	f, err := os.Create(rawCsvPath)
	if err != nil {
		panic(err)
	}
	f.WriteString(strings.Join(csvHeader, "|") + "\n")
	for _, row := range csvRows {
		cleaned := make([]string, len(row))
		for i, v := range row {
			if v == "NULL" {
				cleaned[i] = ""
			} else {
				cleaned[i] = v
			}
		}
		f.WriteString(strings.Join(cleaned, "|") + "\n")
	}
	f.Close()

	state := StateFile{
		GeneratedAt: now.Format(time.RFC3339),
		Timestamp:   timestamp,
		CsvFilename: csvFilename,
		SqlRows:     sqlRowsMap,
		SharedRefs:  sharedRefs,
	}
	stateBytes, _ := json.MarshalIndent(state, "", "  ")
	os.WriteFile(filepath.Join(workDir, "state.json"), stateBytes, 0644)

	os.WriteFile(filepath.Join(baseDir(), ".latest"), []byte(timestamp), 0644)

	fmt.Printf("Generated %d record(s)\n", len(csvRows))
	fmt.Printf("Work dir : %s\n", workDir)
	fmt.Printf("Raw CSV  : %s\n", rawCsvPath)
	fmt.Printf("\nEdit raw.csv if needed, then run:\n  go run . finalize\n")
}

func newUUIDv7() string {
	u, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return u.String()
}
