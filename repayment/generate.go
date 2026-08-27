package main

import (
	"bufio"
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

// crossbankCsvHeader is csvHeader plus cbpay_status — the one column REPAY_CROSS_BANK files
// carry that REPAY_MANUAL_DDR files don't (payment-bulk-adj-repayment-processor
// app/batch/detail_parser.go: CbPayStatus resolves to "" for files without this column).
var crossbankCsvHeader = append(append([]string{}, csvHeader...), "cbpay_status")

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

// crossbankCases is the REPAY_CROSS_BANK decision matrix, confirmed by business 2026-08-13
// against 4 sample cases (RPCB_TXN_002, 003, 009, 010) — see
// payment-bulk-adj-repayment-processor app/decision/adapter_crossbank.go (crossbankRules).
// dpp_transfer_status uses PROCESSING (not PRCS) because that's the value the confirmation
// doc actually exercised; PRCS is accepted by the gate too but untested for crossbank.
// The last row (cbpay_status FAILED) isn't part of the confirmed matrix — it's a QA negative
// case: business confirmed any cbpay_status other than SUCCESS has no crossbank usecase and
// the item is ignored (no status update sent).
var crossbankCases = []map[string]string{
	{"auto_adj_transfer_status": "DPP", "cbpay_status": "SUCCESS", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PROCESSING", "dcb_payment_status": "NULL", "dlp_payment_status": "PENDING", "after_status": "COMPLETED"},
	{"auto_adj_transfer_status": "DPP", "cbpay_status": "SUCCESS", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PROCESSING", "dcb_payment_status": "SUCCESS", "dlp_payment_status": "COMPLETED", "after_status": "COMPLETED"},
	{"auto_adj_transfer_status": "DPP", "cbpay_status": "SUCCESS", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PROCESSING", "dcb_payment_status": "SUCCESS", "dlp_payment_status": "PENDING", "after_status": "COMPLETED"},
	{"auto_adj_transfer_status": "DPP", "cbpay_status": "SUCCESS", "dcb_transfer_status": "FAILED", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PROCESSING", "dcb_payment_status": "NULL", "dlp_payment_status": "PENDING", "after_status": "FAILED"},
	{"auto_adj_transfer_status": "DPP", "cbpay_status": "FAILED", "dcb_transfer_status": "SUCCESS", "dlp_transfer_status": "PENDING", "dpp_transfer_status": "PROCESSING", "dcb_payment_status": "NULL", "dlp_payment_status": "PENDING", "after_status": "No Update"},
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
		fmt.Println("Usage: go run . generate <records> [mode]")
		fmt.Printf("  mode : optional — %q, %q or %q. Overrides config.json's \"generate_mode\" for this run only.\n", ModeRepayment, ModeCrossbank, ModeBoth)
		os.Exit(1)
	}
	times, err := strconv.Atoi(args[0])
	if err != nil || times < 1 {
		fmt.Printf("Invalid records value: %q — must be a positive integer\n", args[0])
		os.Exit(1)
	}

	cfg := readConfig()

	if len(args) >= 2 {
		mode := args[1]
		if mode != ModeRepayment && mode != ModeCrossbank && mode != ModeBoth {
			fmt.Printf("Invalid mode %q — must be %q, %q or %q\n", mode, ModeRepayment, ModeCrossbank, ModeBoth)
			os.Exit(1)
		}
		fmt.Printf("generate_mode overridden on the command line: %s (config.json has %q)\n", mode, cfg.GenerateMode)
		cfg.GenerateMode = mode
	} else {
		cfg.GenerateMode = promptForMode(cfg.GenerateMode)
	}

	now := time.Now()
	timestamp := now.Format("20060102_150405")
	workDir := filepath.Join(baseDir(), "work", timestamp)
	os.MkdirAll(workDir, os.ModePerm)

	pathParts := strings.Split(strings.Trim(cfg.BasePath, "/"), "/")
	csvDateStr := pathParts[len(pathParts)-1]
	csvDate, err := time.Parse("2006-01-02", csvDateStr)
	if err != nil {
		fmt.Printf("Cannot parse date from base_path %q: %v\n", cfg.BasePath, err)
		os.Exit(1)
	}

	state := StateFile{
		GeneratedAt: now.Format(time.RFC3339),
		Timestamp:   timestamp,
		Mode:        cfg.GenerateMode,
		SqlRows:     make(map[string]map[string]string),
	}

	fmt.Printf("generate_mode: %s\n\n", cfg.GenerateMode)

	if cfg.GenerateMode == ModeRepayment || cfg.GenerateMode == ModeBoth {
		state.Repayment = generateDataset(datasetSpec{
			workDir:     workDir,
			rawFilename: "raw_repayment.csv",
			filePrefix:  "REPAY_MANUAL_DDR_RECONCILE_UNMATCHED",
			ctrlPrefix:  "repayment_manual_ddr_unmatched",
			label:       "repayment",
			header:      csvHeader,
			caseSet:     cases,
		}, times, now, csvDate, cfg, state.SqlRows)
	}
	if cfg.GenerateMode == ModeCrossbank || cfg.GenerateMode == ModeBoth {
		state.Crossbank = generateDataset(datasetSpec{
			workDir:     workDir,
			rawFilename: "raw_crossbank.csv",
			filePrefix:  "REPAY_CROSS_BANK_RECONCILE_UNMATCHED",
			ctrlPrefix:  "repayment_cross_bank_unmatched",
			label:       "crossbank-repayment",
			header:      crossbankCsvHeader,
			caseSet:     crossbankCases,
			isCrossbank: true,
		}, times, now, csvDate, cfg, state.SqlRows)
	}

	stateBytes, _ := json.MarshalIndent(state, "", "  ")
	os.WriteFile(filepath.Join(workDir, "state.json"), stateBytes, 0644)

	os.WriteFile(filepath.Join(baseDir(), ".latest"), []byte(timestamp), 0644)

	fmt.Printf("\nWork dir : %s\n", workDir)
	fmt.Printf("\nEdit the raw_*.csv file(s) if needed, then run:\n  go run . finalize\n")
}

// promptForMode is used when `generate` is run without a mode argument on the command line —
// it asks interactively instead of silently falling back to config.json's generate_mode, so
// picking the wrong dataset by habit (forgetting to check config.json first) isn't silent.
// defaultMode (config.json's current generate_mode) is offered as the blank-input answer.
func promptForMode(defaultMode string) string {
	options := []string{ModeRepayment, ModeCrossbank, ModeBoth}
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Select generate_mode:")
	for i, m := range options {
		suffix := ""
		if m == defaultMode {
			suffix = "  (config.json default)"
		}
		fmt.Printf("  %d) %s%s\n", i+1, m, suffix)
	}

	for {
		fmt.Printf("Enter choice [1-3] (blank = %s): ", defaultMode)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)

		if line == "" {
			fmt.Println()
			return defaultMode
		}
		if idx, err := strconv.Atoi(line); err == nil && idx >= 1 && idx <= len(options) {
			fmt.Println()
			return options[idx-1]
		}
		for _, m := range options {
			if strings.EqualFold(line, m) {
				fmt.Println()
				return m
			}
		}
		fmt.Printf("Invalid choice: %q — enter 1, 2, 3, or the mode name\n", line)
	}
}

// datasetSpec describes one dataset's generation inputs — repayment or crossbank-repayment.
type datasetSpec struct {
	workDir     string
	rawFilename string
	filePrefix  string // e.g. REPAY_CROSS_BANK_RECONCILE_UNMATCHED
	ctrlPrefix  string // e.g. repayment_cross_bank_unmatched
	label       string // for log output only
	header      []string
	caseSet     []map[string]string
	isCrossbank bool // true for the crossbank dataset — shapes the base repayment_transactions row
}

// generateDataset writes spec.rawFilename (an editable pipe-delimited CSV) inside workDir,
// records matching SQL rows into sqlRowsMap (shared across datasets — repayment and
// crossbank rows can coexist in one insert script since both target repayment_transactions),
// and returns the CsvDataset descriptor finalize.go needs to encrypt/upload this dataset.
func generateDataset(spec datasetSpec, times int, now time.Time, csvDate time.Time, cfg Config, sqlRowsMap map[string]map[string]string) *CsvDataset {
	csvFilename := fmt.Sprintf("%s_%s.csv", spec.filePrefix, csvDate.Format("20060102"))

	var csvRows [][]string
	for t := 0; t < times; t++ {
		caseItem := spec.caseSet[t%len(spec.caseSet)]
		csvRowMap, sqlVals, sharedRef := generateRowPair(caseItem, cfg, now, csvDate, spec.isCrossbank)

		var row []string
		for _, h := range spec.header {
			row = append(row, csvRowMap[h])
		}
		csvRows = append(csvRows, row)
		sqlRowsMap[sharedRef] = sqlVals
	}

	rawCsvPath := filepath.Join(spec.workDir, spec.rawFilename)
	f, err := os.Create(rawCsvPath)
	if err != nil {
		panic(err)
	}
	f.WriteString(strings.Join(spec.header, "|") + "\n")
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

	fmt.Printf("Generated %d %s record(s)\n", len(csvRows), spec.label)
	fmt.Printf("  Raw CSV : %s\n", rawCsvPath)

	return &CsvDataset{
		CsvFilename: csvFilename,
		RawFilename: spec.rawFilename,
		CtrlPrefix:  spec.ctrlPrefix,
		Header:      spec.header,
	}
}

// generateRowPair builds one record's CSV row map and matching SQL row (pre-formatted SQL
// fragments), sharing the same reconcile_ref_id and plaintext PII between the two so they
// stay consistent. caseItem supplies the reconcile-status combination under test (auto_adj /
// dlp / dpp / dcb transfer+payment statuses, plus cbpay_status for crossbank cases); none of
// it feeds the SQL row — that row is a plausible base repayment_transactions record the CSV's
// reconcile_ref_id points back to, independent of which decision-matrix case is exercised.
//
// When isCrossbank is true, the base row's shape fields (transfer_type, transaction_type,
// posting_type, input_terminal, from_trans_code/from_internal_acct_id, dcb_request/
// dcb_response) are overridden to match a real crossbank repayment_transactions record
// (prod sample repayment_transactions_202608241152.csv, transfer_type MANUAL_CROSSBANK —
// settles through the SETTLEMENT_PROMPTPAY internal account, so there's no physical "from"
// customer account and those columns go NULL instead of a synthesized SAV pocket). status /
// status_code / status_desc / debit_status / credit_status stay at the existing PROCESSING
// defaults regardless of isCrossbank, and ref_id still shares sharedRef with the CSV row's
// reconcile_ref_id — neither of those changes with the dataset.
func generateRowPair(caseItem map[string]string, cfg Config, now time.Time, csvDate time.Time, isCrossbank bool) (map[string]string, map[string]string, string) {
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
		// cbpay_status only exists in the crossbank header; harmless to set here for
		// every row (repayment header simply never looks up the key).
		"cbpay_status": "NULL",
	}
	if v, ok := caseItem["cbpay_status"]; ok {
		csvRowMap["cbpay_status"] = v
	}

	// Build SQL row — store pre-formatted SQL fragments
	recordDtm := csvDate.Format("2006-01-02") + fmt.Sprintf(" %02d:%02d:%02d.000000 +00:00", rand.Intn(24), rand.Intn(60), rand.Intn(60))
	cifNo := fmt.Sprintf("%015d", rand.Int63n(999999999999999))
	refNo := fmt.Sprintf("%012d", rand.Int63n(999999999999))
	paymentTxnRef := fmt.Sprintf("D07D2%s", now.Format("0102150405"))
	toAcctID := newUUIDv7()
	amountStr := "1850.00"

	var dcbReq, dcbResp string
	if isCrossbank {
		// Shaped like the real crossbank batch request/response (clientId,
		// batchRefId, transactions[] with a "custom" leg), success values only —
		// status defaults stay PROCESSING/0000, see doc comment above.
		dcbReq = fmt.Sprintf(`{"clientId": "DigitalPaymentProcessorClientID", "requestId": "%s", "batchRefId": "%s", "reversalFlag": false, "transactions": [{"custom": {"amount": %s, "toAccountId": "%s", "denomination": "THB", "internalAccountId": "SETTLEMENT_PROMPTPAY"}, "transactionCode": "MLRPIN", "transactionType": "REPAYMENT", "transactionClass": "L", "transactionRefId": "%s"}], "effectiveDate": "%s", "additionalInfo": {"refId": "%s", "refNo": "%s", "originalChannelRequestId": "%s"}, "transactionDatetime": "%sT%s+07:00"}`,
			sharedRef, paymentTxnRef, amountStr, toAcctID, paymentTxnRef, csvDate.Format("20060102"), sharedRef, refNo, sharedRef, now.Format("2006-01-02"), now.Format("15:04:05"))
		dcbResp = `{"code": "0000", "message": "Success"}`
	} else {
		dcbReq = fmt.Sprintf(`{"clientId": "DigitalPaymentProcessorClientID", "requestId": "%s", "effectiveDate": "%s", "transactionDatetime": "%sT%s+07:00"}`,
			sharedRef, now.Format("20060102"), now.Format("2006-01-02"), now.Format("15:04:05"))
		dcbResp = fmt.Sprintf(`{"code": "0000", "message": "Success", "data": {"createdRequestId": "%s", "createdDatetime": "%s"}}`,
			sharedRef, now.Format("2006-01-02T15:04:05.000+07:00"))
	}

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
		"ref_no":                    refNo,
		"payment_txn_ref":           paymentTxnRef,
		"tfr_dtm":                   recordDtm,
		"created_request_id":        sharedRef,
		"amount":                    amountStr,
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
		"to_acct_id":                toAcctID,
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

	if isCrossbank {
		// Crossbank repayments settle through SCB's internal PromptPay settlement
		// account (SETTLEMENT_PROMPTPAY), not a customer SAV pocket, so there's no
		// physical "from" account — these columns are NULL in the real record, not
		// a synthesized/encrypted one. status/status_code/status_desc/debit_status/
		// credit_status and ref_id are intentionally left untouched above.
		sqlValsRaw["from_main_account_no"] = nil
		sqlValsRaw["from_transfer_account_no"] = nil
		sqlValsRaw["from_acct_no"] = nil
		sqlValsRaw["from_acct_id"] = nil
		sqlValsRaw["from_trans_code"] = "MLRPSCN"
		sqlValsRaw["from_acct_status"] = nil
		sqlValsRaw["from_product_class"] = nil
		sqlValsRaw["from_product_group"] = nil
		sqlValsRaw["from_product_type"] = nil
		sqlValsRaw["from_acct_type"] = nil
		sqlValsRaw["from_branch_code"] = nil
		sqlValsRaw["from_account_display_name"] = nil
		sqlValsRaw["from_account_name_th"] = nil
		sqlValsRaw["from_account_name_en"] = nil
		sqlValsRaw["from_bank_code"] = nil
		sqlValsRaw["from_core_bank_channel"] = nil
		sqlValsRaw["from_internal_acct_id"] = "SETTLEMENT_PROMPTPAY"
		sqlValsRaw["customer_note"] = nil
		sqlValsRaw["transaction_type"] = "REPAYMENT"
		sqlValsRaw["posting_type"] = "CUSTOM"
		sqlValsRaw["transfer_type"] = "MANUAL_CROSSBANK"
		sqlValsRaw["input_terminal"] = "SCAN"
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

	if isCrossbank {
		// from_account_* are NULL in the real prod record, but repayment_transactions
		// enforces NOT NULL on these name columns — insert '' (same as the to_account_*
		// columns above) instead of the NULL we set in sqlValsRaw.
		sqlVals["from_account_display_name"] = "''"
		sqlVals["from_account_name_th"] = "''"
		sqlVals["from_account_name_en"] = "''"
	}

	return csvRowMap, sqlVals, sharedRef
}

func newUUIDv7() string {
	u, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return u.String()
}
